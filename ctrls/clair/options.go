package clair

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Option is a function capable of set an optional parameter.
type Option func(*Clair)

// WithOwnerReference ensures all created objects contain the provided Owner Reference.
func WithOwnerReference(oref metav1.OwnerReference) Option {
	return func(c *Clair) {
		c.OMutators = append(
			c.OMutators,
			func(ctx context.Context, obj client.Object) error {
				orefs := obj.GetOwnerReferences()
				orefs = append(orefs, oref)
				obj.SetOwnerReferences(orefs)
				return nil
			},
		)
	}
}

// WithNamespace sets the namespace used by the clair controller.
func WithNamespace(namespace string) Option {
	return func(c *Clair) {
		c.namespace = namespace
		c.OMutators = append(
			c.OMutators,
			func(ctx context.Context, obj client.Object) error {
				obj.SetNamespace(namespace)
				return nil
			},
		)
	}
}

// WithNamePrefix sets the name prefix for objects created by this controller.
func WithNamePrefix(prefix string) Option {
	return func(c *Clair) {
		c.namePrefix = prefix
	}
}
