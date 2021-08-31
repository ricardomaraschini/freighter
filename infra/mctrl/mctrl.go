package mctrl

import (
	"context"
	"fmt"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// The following list is a proposition of possible overlays provided by all controllers. We do not
// enforce all controllers to provide these overlays but it is good to have this list somewhere so
// in the future someone can consult it to reuse the naming conventions.
const (
	NotAppliedOverlay = ""
	BaseOverlay       = "base"
	ScaleDownOverlay  = "scale-down"
)

// MicroController is an entity that wraps a single component. For example a Postgres instance is
// considered a component. It provides tooling to move the component to a different overlay (Apply
// call), is capable of returning its own status (Status call), is capable of returning the data
// needed to reach the component from the outside world (Advertise call) and finally to also
// return its current overlay (Overlay call). Ads is in a nutshel a list of data required to reach
// the service from outside, for example: Ads for a postgres component would contain its 'user',
// its 'pass' or even a fully DBURI. Components communicate with each other through advertisements.
type MicroController interface {
	Apply(ctx context.Context, overlay string, ads Ads) error
	Advertise(ctx context.Context) (Ads, error)
	Status(ctx context.Context) (*Status, error)
	Overlay() string
}

// Status holds the current status for a component at the current overlay. For example, a
// component whose current overlay is 'scaled-down' would return Ready as true if no more replicas
// are running for the given component. It is the responsibility of each component to verify and
// assert its status. It is important to return Ready as true only when the component has finished
// its rollout as some other components may depend on it.
type Status struct {
	Ready      bool
	Message    string
	Conditions []metav1.Condition
}

// Ads holds advertised data by one or more than one micro controller. Advertisement data should
// be used when passing information from one micro controller to another. e.g. the redis micro
// controller may advertise its service name and port or a postgres database may advertise its
// user, pass, service and port. The idea is that when we call Apply in one of the micro
// controllers we pass in an Advertisement so the controller can identify if all required data is
// present, e.g. a micro controller for a go application that depends on a postgres database may
// fail during its Apply due to the absence of postgres access info in received Advertisement.
// TODO(rmarasch): Make this struct concurrent safe.
type Ads struct {
	dict map[string]string
}

// Contains verifies if the Ads contains all provided indexes. Returns nil if all indexes were
// found or an error otherwise.
func (a *Ads) Contains(indexes ...string) error {
	var missing []string
	for _, idx := range indexes {
		if _, ok := a.dict[idx]; ok {
			continue
		}
		missing = append(missing, idx)
	}
	if len(missing) > 0 {
		return fmt.Errorf("%s indexes not advertised", strings.Join(missing, ","))
	}
	return nil
}

// Get returns the advertised data at index 'idx'. Empty string is returned if the data has not
// yet been advertised. Try not to advertise empty strings, please.
func (a *Ads) Get(idx string) string {
	if a.dict == nil {
		return ""
	}
	return a.dict[idx]
}

// Delete removes advertised data at index 'idx'.
func (a *Ads) Delete(idx string) {
	if a.dict == nil {
		return
	}
	delete(a.dict, idx)
}

// Put advertises value 'val' at index 'idx'. Overwrites if 'idx' has already been advertised.
func (a *Ads) Put(idx, val string) {
	if a.dict == nil {
		a.dict = map[string]string{}
	}
	a.dict[idx] = val
}
