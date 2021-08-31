package mctrl

import (
	"context"
	"embed"
	"fmt"
	"path"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/kustomize/api/filesys"
	"sigs.k8s.io/kustomize/api/krusty"
	"sigs.k8s.io/kustomize/api/types"

	"gopkg.in/yaml.v2"

	"github.com/ricardomaraschini/carrier/infra/fsloader"
	"github.com/ricardomaraschini/carrier/infra/resource"
)

// BaseKustomizationPath is the location for the base kustomization.yaml file. This controller
// expects to find this file among the read files from the embed reference. This is the file
// that, after parse, is send over to all registered KMutators.
const BaseKustomizationPath = "/kustomize/base/kustomization.yaml"

// KustCtrl is a base controller to provide some tooling around rendering and creating resources
// based in a kustomize directory struct. Files are expected to be injected into this controller
// by means of an embed.FS struct. The filesystem struct, inside the embed.FS struct, is expected
// to comply with the following layout:
//
// /kustomize
// /kustomize/base/kustomization.yaml
// /kustomize/base/object_a.yaml
// /kustomize/base/object_a.yaml
// /kustomize/overlay0/kustomization.yaml
// /kustomize/overlay0/object_c.yaml
// /kustomize/overlay1/kustomization.yaml
// /kustomize/overlay1/object_d.yaml
//
// In a nutshell, we have a base kustomization under base/ directory and each other directory is
// treated as an overlay to be applied on base's top. This struct, intentionally, does not fully
// comply with the MicroController interface, it is a struct to be used as composition to higher
// specialized constructs.
type KustCtrl struct {
	cli       client.Client
	from      embed.FS
	overlay   string
	fowner    string
	KMutators []func(context.Context, *types.Kustomization, Ads) error
	OMutators []func(context.Context, client.Object) error
}

// NewKustCtrl returns a kustomize controller reading and applying files provided by the embed.FS
// reference. Files are read from 'emb' into a filesys.FileSystem representation and then used as
// argument to Kustomize when generating objects.
func NewKustCtrl(cli client.Client, emb embed.FS) *KustCtrl {
	return &KustCtrl{
		cli:    cli,
		from:   emb,
		fowner: "undefined",
	}
}

// Apply applies provided overlay and creates objects in the kubernetes API using internal client.
// In case of failures there is no rollback so it is possible that this ends up partially creating
// the objects (returns at the first failure). Prior to object creation this function feeds all
// registered OMutators with the objects allowing for last time adjusts.
func (k *KustCtrl) Apply(ctx context.Context, overlay string, ad Ads) error {
	objs, err := k.parse(ctx, overlay, ad)
	if err != nil {
		return fmt.Errorf("error parsing kustomize files: %w", err)
	}

	for _, obj := range objs {
		for _, mut := range k.OMutators {
			if err := mut(ctx, obj); err != nil {
				return fmt.Errorf("error mutating object: %w", err)
			}
		}

		// XXX clarify on the field owner usage.
		err := k.cli.Patch(ctx, obj, client.Apply, client.FieldOwner(k.fowner))
		if err != nil {
			return fmt.Errorf("error patching object: %w", err)
		}
	}

	k.overlay = overlay
	return nil
}

// parse reads kustomize files and returns them all parsed as valid client.Object structs. Loads
// everything from the embed.FS into a filesys.FileSystem instance, mutates the base kustomization
// and returns the objects as a slice of client.Object.
func (k *KustCtrl) parse(ctx context.Context, overlay string, ads Ads) ([]client.Object, error) {
	virtfs, err := fsloader.Load(k.from)
	if err != nil {
		return nil, fmt.Errorf("unable to load overlay: %w", err)
	}

	if err := k.mutateKustomization(ctx, virtfs, ads); err != nil {
		return nil, fmt.Errorf("error setting object name prefix: %w", err)
	}

	res, err := krusty.MakeKustomizer(krusty.MakeDefaultOptions()).Run(
		virtfs, path.Join("kustomize", overlay),
	)
	if err != nil {
		return nil, fmt.Errorf("error running kustomize: %w", err)
	}

	var objs []client.Object
	for _, rsc := range res.Resources() {
		obj, err := resource.ToObject(rsc)
		if err != nil {
			return nil, fmt.Errorf("error parsing object: %w", err)
		}
		objs = append(objs, obj)
	}
	return objs, nil
}

// mutateKustomization feeds all registered KMutators with the parsed BaseKustomizationPath.
// After feeding KMutators the output is marshaled and written back to the filesys.FileSystem.
func (k *KustCtrl) mutateKustomization(ctx context.Context, fs filesys.FileSystem, ads Ads) error {
	if len(k.KMutators) == 0 {
		return nil
	}

	olddt, err := fs.ReadFile(BaseKustomizationPath)
	if err != nil {
		return fmt.Errorf("error reading base kustomization: %w", err)
	}

	var kust types.Kustomization
	if err := yaml.Unmarshal(olddt, &kust); err != nil {
		return fmt.Errorf("error parsing base kustomization: %w", err)
	}

	for _, mut := range k.KMutators {
		if err := mut(ctx, &kust, ads); err != nil {
			return fmt.Errorf("error mutating kustomization: %w", err)
		}
	}

	newdt, err := yaml.Marshal(kust)
	if err != nil {
		return fmt.Errorf("error marshaling base kustomization: %w", err)
	}

	if err := fs.WriteFile(BaseKustomizationPath, newdt); err != nil {
		return fmt.Errorf("error writing base kustomization: %w", err)
	}
	return nil
}

// Overlay returns the last applied overlay.
func (k *KustCtrl) Overlay() string {
	return k.overlay
}
