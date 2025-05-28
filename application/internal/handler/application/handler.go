// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package application

import (
	"context"

	"github.com/pkg/errors"
	admin "github.com/telekom/controlplane/admin/api/v1"
	application "github.com/telekom/controlplane/application/api/v1"
	"github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/handler"
	"github.com/telekom/controlplane/common/pkg/types"
	"github.com/telekom/controlplane/common/pkg/util/contextutil"
	gateway "github.com/telekom/controlplane/gateway/api/v1"
	identity "github.com/telekom/controlplane/identity/api/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

var _ handler.Handler[*application.Application] = &ApplicationHandler{}

type ApplicationHandler struct{}

func (h *ApplicationHandler) CreateOrUpdate(ctx context.Context, app *application.Application) error {
	log := log.FromContext(ctx)

	c := client.ClientFromContextOrDie(ctx)
	c.AddKnownTypeToState(&identity.Client{})
	c.AddKnownTypeToState(&gateway.Consumer{})

	zone, err := GetZone(ctx, c, app.Spec.Zone)
	if err != nil {
		log.Error(err, "❌ Failed to get Zone when creating application")
		return err
	}

	// Create Client only if subscription present
	if app.Spec.NeedsClient {
		err = h.CreateIdentityClient(ctx, zone, app)
		if err != nil {
			log.Error(err, "❌ Failed to create Identity client when creating application")
			return err
		}
	} else {
		app.Status.ClientSecret = "NOT_NEEDED"
	}

	if app.Spec.NeedsConsumer {
		err = h.CreateGatewayConsumer(ctx, zone, app)
		if err != nil {
			return err
		}
	}

	_, err = c.CleanupAll(ctx, client.OwnedBy(app))
	if err != nil {
		return err
	}

	if c.AnyChanged() {
		app.SetCondition(
			condition.NewProcessingCondition("SubResourceProvising", "Atleast one sub-resource has been created or updated"))
		app.SetCondition(
			condition.NewNotReadyCondition("SubResourceProvising", "Atleast one sub-resource has been created or updated"))
	} else {
		app.SetCondition(condition.NewDoneProcessingCondition("All sub-resources are up to date"))
		app.SetCondition(condition.NewReadyCondition("SubResourceProvisioned", "All sub-resources are up to date"))
	}

	app.Status.ClientId = MakeClientName(app)

	return nil
}

func (h *ApplicationHandler) CreateIdentityClient(ctx context.Context, zone *admin.Zone, owner *application.Application) error {
	client := client.ClientFromContextOrDie(ctx)
	clientName := MakeClientName(owner)
	realmName := contextutil.EnvFromContextOrDie(ctx)

	// get namespace from zoneStatus
	namespace := zone.Status.Namespace

	// get Realm with realmref from namespace
	realmRef := &types.ObjectRef{
		Name:      realmName,
		Namespace: namespace,
	}

	idpClient := &identity.Client{
		ObjectMeta: metav1.ObjectMeta{
			Name:      clientName,
			Namespace: owner.GetNamespace(),
		},
	}

	mutator := func() error {
		err := ctrl.SetControllerReference(owner, idpClient, client.Scheme())
		if err != nil {
			return errors.Wrapf(err, "failed to set controller reference for identity client %s", clientName)
		}

		idpClient.Spec = identity.ClientSpec{
			ClientId:     clientName,
			Realm:        realmRef,
			ClientSecret: owner.Spec.Secret,
		}

		return nil
	}

	_, err := client.CreateOrUpdate(ctx, idpClient, mutator)
	if err != nil {
		return errors.Wrapf(err, "❌ failed to create or update Identity Client %s", clientName)
	}

	owner.Status.ClientSecret = idpClient.Spec.ClientSecret
	return nil
}

func (h *ApplicationHandler) CreateGatewayConsumer(ctx context.Context, zone *admin.Zone, owner *application.Application) error {
	client := client.ClientFromContextOrDie(ctx)
	consumerName := MakeClientName(owner)
	realmName := contextutil.EnvFromContextOrDie(ctx)

	realmRef := types.ObjectRef{
		Name:      realmName,
		Namespace: zone.Status.Namespace,
	}

	consumer := &gateway.Consumer{
		ObjectMeta: metav1.ObjectMeta{
			Name:      consumerName,
			Namespace: owner.GetNamespace(),
		},
	}

	mutator := func() error {
		err := ctrl.SetControllerReference(owner, consumer, client.Scheme())
		if err != nil {
			return errors.Wrapf(err, "failed to set controller reference for gateway consumer %s", consumerName)
		}
		consumer.Spec = gateway.ConsumerSpec{
			Realm: realmRef,
			Name:  consumerName,
		}

		return nil
	}

	_, err := client.CreateOrUpdate(ctx, consumer, mutator)
	if err != nil {
		return errors.Wrapf(err, "❌ failed to create or update Gateway Consumer %s", consumerName)
	}

	return nil
}

func (h *ApplicationHandler) Delete(ctx context.Context, app *application.Application) error {
	return nil
}
