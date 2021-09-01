package main

import (
	"context"
	"log"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"

	"github.com/ricardomaraschini/freighter/ctrls/clair"
	"github.com/ricardomaraschini/freighter/ctrls/postgres"
	"github.com/ricardomaraschini/freighter/infra/mctrl"
)

func main() {
	ctx := context.Background()

	cli, err := client.New(config.GetConfigOrDie(), client.Options{})
	if err != nil {
		log.Fatalf("error creating client: %s", err)
	}

	cm := createCM(ctx, cli)

	var ad mctrl.Ads

	log.Printf("deploying clair postgres")
	pgsql := postgres.New(
		cli,
		postgres.WithNamespace("rmarasch"),
		postgres.WithNamePrefix("clair"),
		postgres.WithOwnerReference(
			metav1.OwnerReference{
				APIVersion: "v1",
				Name:       "testing",
				Kind:       "ConfigMap",
				UID:        cm.UID,
			},
		),
	)
	ad = apply(ctx, pgsql, mctrl.BaseOverlay, ad)

	log.Printf("deploying clair")
	clr := clair.New(
		cli,
		clair.WithNamespace("rmarasch"),
		clair.WithNamePrefix("clair"),
		clair.WithOwnerReference(
			metav1.OwnerReference{
				APIVersion: "v1",
				Name:       "testing",
				Kind:       "ConfigMap",
				UID:        cm.UID,
			},
		),
	)
	apply(ctx, clr, mctrl.BaseOverlay, ad)

}

func apply(
	ctx context.Context, mc mctrl.MicroController, overlay string, ad mctrl.Ads,
) mctrl.Ads {
	if err := mc.Apply(ctx, overlay, ad); err != nil {
		log.Fatal(err)
	}

	status, err := mc.Status(ctx)
	if err != nil {
		log.Fatal(err)
	}

	for !status.Ready {
		time.Sleep(time.Second)
		status, err = mc.Status(ctx)
		if err != nil {
			log.Fatal(err)
		}
	}

	ad, err = mc.Advertise(ctx)
	if err != nil {
		log.Fatal(err)
	}
	return ad
}

func createCM(ctx context.Context, cli client.Client) corev1.ConfigMap {
	nsn := types.NamespacedName{
		Namespace: "rmarasch",
		Name:      "testing",
	}
	var cm corev1.ConfigMap
	if err := cli.Get(ctx, nsn, &cm); err == nil {
		return cm
	} else if !errors.IsNotFound(err) {
		log.Fatal(err)
	}

	cm = corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "testing",
			Namespace: "rmarasch",
		},
		Data: map[string]string{},
	}
	if err := cli.Create(ctx, &cm); err != nil {
		log.Fatal(err)
	}
	return cm
}
