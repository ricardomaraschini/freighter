package redis

import (
	"context"
	"embed"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ktypes "sigs.k8s.io/kustomize/api/types"

	"github.com/ricardomaraschini/carrier/infra/mctrl"
	"github.com/ricardomaraschini/carrier/infra/resource"
)

//go:embed kustomize/*
var kfiles embed.FS

// New returns a new Redis controller. This controller attempts to mantain a redis instance online
// through a deployment. Provides mctrl.ScaleDownOverlay overlay (brings the number of redis pods
// down to zero).
func New(cli client.Client, opts ...Option) *Redis {
	rs := &Redis{
		KustCtrl:   mctrl.NewKustCtrl(cli, kfiles),
		namespace:  "default",
		namePrefix: "undefined",
		client:     cli,
	}

	rs.KMutators = append(rs.KMutators, rs.mutateKustomization)

	for _, opt := range opts {
		opt(rs)
	}
	return rs
}

// Redis controls a redis deployment. Deploys a redis server and keeps track of its status.
// Advertises the redis service address. Redis implements mctrl.MicroController interface so other
// controlers can use when configuring third party applications.
type Redis struct {
	*mctrl.KustCtrl

	client     client.Client
	namespace  string
	namePrefix string
}

// mutateKustomization mutates the base Kustomization for a Redis deployment. Only appends the
// provided name prefix.
func (r *Redis) mutateKustomization(
	ctx context.Context, kust *ktypes.Kustomization, adv mctrl.Advertisement,
) error {
	kust.NamePrefix = fmt.Sprintf("%s-", r.namePrefix)
	return nil
}

// Advertise returns data this component advertises. This component advertises only the redis
// address (service address) and port.
func (r *Redis) Advertise(ctx context.Context) (mctrl.Advertisement, error) {
	var adv mctrl.Advertisement
	if r.Overlay() == mctrl.ScaleDownOverlay || r.Overlay() == mctrl.NotAppliedOverlay {
		return adv, nil
	}

	adv.Put("address", fmt.Sprintf("%s-redis.%s.svc", r.namePrefix, r.namespace))
	adv.Put("port", "6379")
	return adv, nil
}

// Status return the status for this component at the last applied overlay.
func (r *Redis) Status(ctx context.Context) (*mctrl.Status, error) {
	if r.Overlay() == mctrl.NotAppliedOverlay {
		return nil, fmt.Errorf("no overlay applied to the controller")
	}

	nsn := types.NamespacedName{
		Namespace: r.namespace,
		Name:      fmt.Sprintf("%s-redis", r.namePrefix),
	}

	var dep appsv1.Deployment
	if err := r.client.Get(ctx, nsn, &dep); err != nil {
		return nil, fmt.Errorf("error getting deployment: %w", err)
	}

	var replicas int32
	if r.Overlay() != mctrl.ScaleDownOverlay && dep.Spec.Replicas != nil {
		replicas = *dep.Spec.Replicas
	}

	var conds []metav1.Condition
	for _, cond := range dep.Status.Conditions {
		mv1cond, err := resource.ToCondition(cond)
		if err != nil {
			return nil, fmt.Errorf("error converting condition: %w", err)
		}
		conds = append(conds, mv1cond)
	}

	if dep.Status.AvailableReplicas != replicas {
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
