package postgres

import (
	"context"
	"embed"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ktypes "sigs.k8s.io/kustomize/api/types"

	"github.com/google/uuid"

	"github.com/ricardomaraschini/freighter/infra/mctrl"
	"github.com/ricardomaraschini/freighter/infra/resource"
)

//go:embed kustomize/*
var kfiles embed.FS

// New returns a new Postgres controller. This creates a postgresq deployment, a pvc, a service
// and a service account. If you want to have more than one postgres instance in the same
// namespace you have to configure this to use different name prefixes, see WithNamePrefix option.
func New(cli client.Client, opts ...Option) *Postgres {
	pg := &Postgres{
		KustCtrl:   mctrl.NewKustCtrl(cli, kfiles),
		namespace:  "default",
		namePrefix: "undefined",
		client:     cli,
	}

	pg.KMutators = append(pg.KMutators, pg.mutateKustomization)

	for _, opt := range opts {
		opt(pg)
	}
	return pg
}

// Postgres controls a postgres deployment. This controller creates a default user and database
// but advertises the admin uri as well. If user is not happy with the default user and database
// they should use the admin uri and configure whatever they feel like (the goal here is to keep
// things as simple as possible). Default user is called 'user' and default database is called
// 'database', passwords are randomly generated when users first apply one of the overlays.
type Postgres struct {
	*mctrl.KustCtrl

	client     client.Client
	ownerRef   *metav1.OwnerReference
	namespace  string
	namePrefix string
}

// mutateKustomization makes sure we append a prefix to created objects and that we also populate
// a secret with the necessary database secret data. Passwords are kept in two different secrets,
// one if for this controller consumption and the other is a Generated Secret, the latter is then
// mounted in the postgresq deployment.
func (p *Postgres) mutateKustomization(
	ctx context.Context, kust *ktypes.Kustomization, ad mctrl.Ads,
) error {
	pass, rootpass, err := p.ensurePsqlSecretData(ctx)
	if err != nil {
		return fmt.Errorf("error ensuring pgsql secret data: %w", err)
	}

	sctcontent := []string{
		"database-username=user",
		"database-name=database",
		fmt.Sprintf("database-password=%s", pass),
		fmt.Sprintf("database-root-password=%s", rootpass),
	}

	kust.NamePrefix = fmt.Sprintf("%s-", p.namePrefix)
	kust.SecretGenerator = []ktypes.SecretArgs{
		{
			GeneratorArgs: ktypes.GeneratorArgs{
				Name: "postgres-config-secret",
				KvPairSources: ktypes.KvPairSources{
					LiteralSources: sctcontent,
				},
			},
		},
	}
	return nil
}

// Advertise advertises postgres address (service name), port, user, passowrd and database
// name. Advertises postgres' admin user and password as well.
func (p *Postgres) Advertise(ctx context.Context) (mctrl.Ads, error) {
	var ad mctrl.Ads

	// if scaling down or not deployed advertises nothing.
	if p.Overlay() == mctrl.ScaleDownOverlay || p.Overlay() == mctrl.NotAppliedOverlay {
		return ad, nil
	}

	pass, rootpass, err := p.ensurePsqlSecretData(ctx)
	if err != nil {
		return ad, fmt.Errorf("error reading pgsql secret data: %w", err)
	}

	ad.Put("dbhost", fmt.Sprintf("%s-database.%s.svc", p.namePrefix, p.namespace))
	ad.Put("dbport", "5432")
	ad.Put("dbuser", "user")
	ad.Put("dbpass", pass)
	ad.Put("dbname", "database")
	ad.Put("dbrootuser", "postgres")
	ad.Put("dbrootpass", rootpass)
	return ad, nil
}

// ensurePsqlSecretData makes sure we have created a secret to store pgsql access data. We have
// to keep this secret around so we don't keep regenerating passwords every time we Apply some
// different overlay. Returns the user and root passwords as strings after storing them in the
// kubernetes secret. If the secret already exists this function only reads its values.
func (p *Postgres) ensurePsqlSecretData(ctx context.Context) (string, string, error) {
	nsn := types.NamespacedName{
		Namespace: p.namespace,
		Name:      fmt.Sprintf("%s-pgsql-access-data", p.namePrefix),
	}

	var sct corev1.Secret
	err := p.client.Get(ctx, nsn, &sct)
	if err == nil {
		return string(sct.Data["pass"]), string(sct.Data["rootpass"]), nil
	} else if !errors.IsNotFound(err) {
		return "", "", fmt.Errorf("error reading pgsql access data: %w", err)
	}

	// generates new random password and root password.
	data := map[string]string{
		"pass":     uuid.New().String(),
		"rootpass": uuid.New().String(),
	}

	sct.Name = nsn.Name
	sct.Namespace = nsn.Namespace
	sct.StringData = data
	if p.ownerRef != nil {
		sct.SetOwnerReferences([]metav1.OwnerReference{*p.ownerRef})
	}

	if err := p.client.Create(ctx, &sct); err != nil {
		return "", "", fmt.Errorf("error creating pgsql secret data: %w", err)
	}
	return data["pass"], data["rootpass"], nil
}

// Status return the status for this component at the current overlay. Inspects the postgres
// deployment and sees if the number of available replicas is equal to the number of requested
// replicas. Returns postgres conditions as controller conditions.
func (p *Postgres) Status(ctx context.Context) (*mctrl.Status, error) {
	if p.Overlay() == mctrl.NotAppliedOverlay {
		return nil, fmt.Errorf("no overlay applied to the controller")
	}

	nsn := types.NamespacedName{
		Namespace: p.namespace,
		Name:      fmt.Sprintf("%s-database", p.namePrefix),
	}

	var dep appsv1.Deployment
	if err := p.client.Get(ctx, nsn, &dep); err != nil {
		return nil, fmt.Errorf("unable to get deployment: %w", err)
	}

	var conds []metav1.Condition
	for _, cond := range dep.Status.Conditions {
		mv1cond, err := resource.ToCondition(cond)
		if err != nil {
			return nil, fmt.Errorf("error processing condition: %s", err)
		}
		conds = append(conds, mv1cond)
	}

	// if we are not scaled down just check if the number of AvailableReplicas is equal
	// to the number of requested replicas (spec.Replicas).
	if p.Overlay() != mctrl.ScaleDownOverlay {
		var replicas int32
		if dep.Spec.Replicas != nil {
			replicas = *dep.Spec.Replicas
		}

		if replicas != dep.Status.AvailableReplicas {
			return &mctrl.Status{
				Ready:      false,
				Message:    "deployment not fully available yet",
				Conditions: conds,
			}, nil
		}
		return &mctrl.Status{
			Ready:      true,
			Message:    "deployment available",
			Conditions: conds,
		}, nil
	}

	// XXX if we are scaled down then we can't use the status.AvailableReplicas as
	// it indicates we have zero available replicas while we may still have some pods
	// dangling in Terminating state. Hence this hack, we only consider ourselves Ready
	// when all pods are no more.
	if has, err := p.hasDanglingPods(ctx, dep); err != nil {
		return nil, fmt.Errorf("error checking for dangling pods: %w", err)
	} else if has {
		return &mctrl.Status{
			Ready:      false,
			Message:    "deployment scaling down",
			Conditions: conds,
		}, nil
	}

	return &mctrl.Status{
		Ready:      true,
		Message:    "deployment scaled down",
		Conditions: conds,
	}, nil
}

// hasDanglingPods checks if a deployment contains any pod dangling online. Verifies through
// all replicasets owned by the deployment.
func (p *Postgres) hasDanglingPods(ctx context.Context, dep appsv1.Deployment) (bool, error) {
	var rsets appsv1.ReplicaSetList
	if err := p.client.List(ctx, &rsets, client.InNamespace(p.namespace)); err != nil {
		return false, fmt.Errorf("error listing replicasets: %w", err)
	}

	var pods corev1.PodList
	if err := p.client.List(ctx, &pods, client.InNamespace(p.namespace)); err != nil {
		return false, fmt.Errorf("error listing replicasets: %w", err)
	}

	// captures the uids for all replicasets owned by the deployment in a map.
	var rss = map[types.UID]bool{}
	for _, rs := range rsets.Items {
		for _, oref := range rs.GetOwnerReferences() {
			if oref.UID != dep.UID || oref.Kind != "Deployment" {
				continue
			}
			rss[rs.UID] = true
		}
	}

	// verify if any of the pods are children of any of the replica sets, if yes then
	// we are not ready yet as there are still a pod terminating. We don't inspect pod
	// state as it is a postgres and the pod must go away.
	for _, pod := range pods.Items {
		for _, oref := range pod.GetOwnerReferences() {
			_, ok := rss[oref.UID]
			if !ok || oref.Kind != "ReplicaSet" {
				continue
			}
			return true, nil
		}
	}
	return false, nil
}
