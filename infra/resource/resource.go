package resource

import (
	"encoding/json"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	asclv1 "k8s.io/api/autoscaling/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/kustomize/api/resource"
	"sigs.k8s.io/kustomize/kyaml/resid"
)

// ToCondition attempts to convert any information into a metav1.Condition. Use this function
// carefully as it provides little assurance about the output. This function is here as multiple
// packages contain their own version of a Condition struct, they all ressemble each other so I
// guess this can be useful. JSON Marshals and Unmarshals input into a metav1.Condition.
func ToCondition(in interface{}) (metav1.Condition, error) {
	var zero metav1.Condition

	dt, err := json.Marshal(in)
	if err != nil {
		return zero, err
	}

	var cond metav1.Condition
	if err := json.Unmarshal(dt, &cond); err != nil {
		return zero, err
	}
	return cond, nil
}

// ToObject converts provided resource.Resource into a client.Object representation by marshaling
// and unmarshaling into a kubernetes struct. This function will return an error if Resource GVK
// is not mapped to a struct.
func ToObject(res *resource.Resource) (client.Object, error) {
	var obj client.Object

	switch res.GetGvk() {
	case resid.Gvk{
		Version: "v1",
		Kind:    "Secret",
	}:
		obj = &corev1.Secret{}

	case resid.Gvk{
		Version: "v1",
		Kind:    "Service",
	}:
		obj = &corev1.Service{}

	case resid.Gvk{
		Version: "v1",
		Kind:    "ServiceAccount",
	}:
		obj = &corev1.ServiceAccount{}

	case resid.Gvk{
		Version: "v1",
		Kind:    "PersistentVolumeClaim",
	}:
		obj = &corev1.PersistentVolumeClaim{}

	case resid.Gvk{
		Group:   "apps",
		Version: "v1",
		Kind:    "Deployment",
	}:
		obj = &appsv1.Deployment{}

	case resid.Gvk{
		Group:   "autoscaling",
		Version: "v2beta2",
		Kind:    "HorizontalPodAutoscaler",
	}:
		obj = &asclv1.HorizontalPodAutoscaler{}

	default:
		return nil, fmt.Errorf("unmapped type %+v", res.GetGvk())
	}

	rawjson, err := res.MarshalJSON()
	if err != nil {
		return nil, fmt.Errorf("error marshaling resource: %w", err)
	}

	if err := json.Unmarshal(rawjson, obj); err != nil {
		return nil, fmt.Errorf("error unmarshaling object: %w", err)
	}
	return obj, nil
}
