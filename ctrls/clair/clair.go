package clair

import (
	"context"
	"embed"
	"fmt"

	"gopkg.in/yaml.v2"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ktypes "sigs.k8s.io/kustomize/api/types"

	"github.com/ricardomaraschini/freighter/infra/mctrl"
	"github.com/ricardomaraschini/freighter/infra/resource"
)

//go:embed kustomize/*
var kfiles embed.FS

// New returns a new Clair controller. This controller attempts to mantain a clair instance online
// through a deployment. Provides mctrl.ScaleDownOverlay overlay (brings the number of clair pods
// down to zero). Clair default configuration is based in static/default-clair-config.yaml file.
func New(cli client.Client, opts ...Option) *Clair {
	cl := &Clair{
		KustCtrl:   mctrl.NewKustCtrl(cli, kfiles),
		namespace:  "default",
		namePrefix: "undefined",
		client:     cli,
	}

	cl.KMutators = append(cl.KMutators, cl.mutateKustomization)

	for _, opt := range opts {
		opt(cl)
	}
	return cl
}

// Clair controls a clair deployment. Deploys clair server and keeps track of its status.
type Clair struct {
	*mctrl.KustCtrl

	client     client.Client
	namespace  string
	namePrefix string
}

// mutateKustomization makes sure we append a prefix to all created objects. It also attempts
// to parse the static default configuration (static/default-clair-config.yaml) and build a
// valid clair configuration based in the received advertised data. Config is then placed in a
// secret that is mounted in clair pods.
func (c *Clair) mutateKustomization(
	ctx context.Context, kust *ktypes.Kustomization, ads mctrl.Ads,
) error {
	config, err := c.buildClairConfig(ads)
	if err != nil {
		return fmt.Errorf("unable to build clair config: %w", err)
	}

	cfg, err := yaml.Marshal(config)
	if err != nil {
		return fmt.Errorf("error marshaling clair config: %w", err)
	}

	kust.NamePrefix = fmt.Sprintf("%s-", c.namePrefix)
	kust.SecretGenerator = []ktypes.SecretArgs{
		{
			GeneratorArgs: ktypes.GeneratorArgs{
				Name: "clair-config",
				KvPairSources: ktypes.KvPairSources{
					LiteralSources: []string{
						fmt.Sprintf("config.yaml=%s", cfg),
					},
				},
			},
		},
	}
	return nil
}

// buildClairConfig attempts to construct a valid clair config. Verifies all mandatory data is
// present in received Ads. The following advertised info is mandatory: "dbhost", "dbport",
// "dbname", "dbrootuser", "dbrootpass". TODO(rmarasch): For sake of simplicity leverages root
// user and pass but this should be changed in the future.
func (c *Clair) buildClairConfig(ads mctrl.Ads) (*Config, error) {
	needed := []string{"dbhost", "dbport", "dbname", "dbrootuser", "dbrootpass"}
	if err := ads.Contains(needed...); err != nil {
		return nil, fmt.Errorf("missing advertised data: %w", err)
	}

	config, err := EmptyConfig()
	if err != nil {
		return nil, fmt.Errorf("unable to process default config: %w", err)
	}

	// make sure clair's config uses the right database according to advertised
	// info. Sets all agents to use the same database leveraging root user.
	connstr := fmt.Sprintf(
		"host=%s port=%s dbname=%s user=%s password=%s sslmode=disable",
		ads.Get("dbhost"),
		ads.Get("dbport"),
		ads.Get("dbname"),
		ads.Get("dbrootuser"),
		ads.Get("dbrootpass"),
	)
	config.Indexer.ConnString = connstr
	config.Matcher.ConnString = connstr
	config.Notifier.ConnString = connstr

	return config, nil
}

// Advertise returns data this component advertises. This component advertises only the clair
// address. TODO(rmarasch): there is more info that needs to be advertised, not clear yet what.
func (c *Clair) Advertise(ctx context.Context) (mctrl.Ads, error) {
	var ads mctrl.Ads
	if c.Overlay() == mctrl.ScaleDownOverlay {
		return ads, nil
	}
	addr := fmt.Sprintf("%s-clair.%s", c.namePrefix, c.namespace)
	ads.Put("clair-addr", addr)
	return ads, nil
}

// Status return the status for this component at the current overlay.
func (c *Clair) Status(ctx context.Context) (*mctrl.Status, error) {
	if c.Overlay() == mctrl.NotAppliedOverlay {
		return nil, fmt.Errorf("no overlay applied to the controller")
	}

	nsn := types.NamespacedName{
		Namespace: c.namespace,
		Name:      fmt.Sprintf("%s-clair", c.namePrefix),
	}

	var dep appsv1.Deployment
	if err := c.client.Get(ctx, nsn, &dep); err != nil {
		return nil, fmt.Errorf("error getting deployment: %w", err)
	}
	spec := dep.Spec
	stat := dep.Status

	var replicas int32
	if c.Overlay() != mctrl.ScaleDownOverlay && spec.Replicas != nil {
		replicas = *spec.Replicas
	}

	var conds []metav1.Condition
	for _, cond := range stat.Conditions {
		mv1cond, err := resource.ToCondition(cond)
		if err != nil {
			return nil, fmt.Errorf("error converting condition: %w", err)
		}
		conds = append(conds, mv1cond)
	}

	if stat.AvailableReplicas != replicas || stat.UpdatedReplicas != replicas {
		return &mctrl.Status{
			Ready:      false,
			Message:    "deployment not fully available yet",
			Conditions: conds,
		}, nil
	}

	return &mctrl.Status{
		Ready:      true,
		Message:    "deployment ready",
		Conditions: conds,
	}, nil
}
