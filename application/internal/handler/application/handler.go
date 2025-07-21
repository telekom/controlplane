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
	"github.com/telekom/controlplane/common/pkg/config"
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

	app.Status.Clients = []types.ObjectRef{}
	app.Status.Consumers = []types.ObjectRef{}

	zone, err := GetZone(ctx, c, app.Spec.Zone)
	if err != nil {
		log.Error(err, "❌ Failed to get Zone when creating application")
		return err
	}
	failoverZones := make([]*admin.Zone, 0, len(app.Spec.FailoverZones))
	if app.Spec.NeedsClient || app.Spec.NeedsConsumer {
		for _, zoneRef := range app.Spec.FailoverZones {
			zone, err := GetZone(ctx, c, zoneRef)
			if err != nil {
				log.Error(err, "❌ Failed to get Zone when creating application")
				return err
			}
			failoverZones = append(failoverZones, zone)
		}
	}

	// Create Client only if subscription present
	if app.Spec.NeedsClient {
		err = CreateIdentityClient(ctx, zone, app)
		if err != nil {
			log.Error(err, "❌ Failed to create Identity client when creating application")
			return err
		}
		for _, failoverZone := range failoverZones {
			err = CreateIdentityClient(ctx, failoverZone, app, WithFailover())
			if err != nil {
				log.Error(err, "❌ Failed to create Identity client for failover zone when creating application")
				return err
			}
		}

	} else {
		app.Status.ClientSecret = "NOT_NEEDED"
	}

	if app.Spec.NeedsConsumer {
		err = CreateGatewayConsumer(ctx, zone, app)
		if err != nil {
			return err
		}

		for _, failoverZone := range failoverZones {
			err = CreateGatewayConsumer(ctx, failoverZone, app, WithFailover())
			if err != nil {
				log.Error(err, "❌ Failed to create Gateway consumer for failover zone when creating application")
				return err
			}
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

func (h *ApplicationHandler) Delete(ctx context.Context, app *application.Application) error {
	// deleted using controller reference
	return nil
}

type CreateOptions struct {
	Failover bool
}

type CreateOption func(*CreateOptions)

func WithFailover() CreateOption {
	return func(opts *CreateOptions) {
		opts.Failover = true
	}
}

func CreateIdentityClient(ctx context.Context, zone *admin.Zone, owner *application.Application, opts ...CreateOption) error {
	options := &CreateOptions{}
	for _, opt := range opts {
		opt(options)
	}

	client := client.ClientFromContextOrDie(ctx)
	clientId := MakeClientName(owner)
	resourceName := clientId + "--" + zone.Name
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
			Name:      resourceName,
			Namespace: owner.GetNamespace(),
		},
	}

	mutator := func() error {
		idpClient.Labels = map[string]string{
			config.BuildLabelKey("application"): owner.Name,
			config.BuildLabelKey("team"):        owner.Spec.Team,
			config.BuildLabelKey("realm"):       realmName,
			config.BuildLabelKey("zone"):        zone.Name,
		}
		if options.Failover {
			idpClient.Labels[config.BuildLabelKey("failover")] = "true"
		}

		err := ctrl.SetControllerReference(owner, idpClient, client.Scheme())
		if err != nil {
			return errors.Wrapf(err, "failed to set controller reference for identity client %s", resourceName)
		}

		idpClient.Spec = identity.ClientSpec{
			ClientId:     clientId,
			Realm:        realmRef,
			ClientSecret: owner.Spec.Secret,
		}

		return nil
	}

	_, err := client.CreateOrUpdate(ctx, idpClient, mutator)
	if err != nil {
		return errors.Wrapf(err, "failed to create or update Identity Client %s", resourceName)
	}

	owner.Status.ClientSecret = idpClient.Spec.ClientSecret
	owner.Status.Clients = append(owner.Status.Clients, *types.ObjectRefFromObject(idpClient))
	return nil
}

func CreateGatewayConsumer(ctx context.Context, zone *admin.Zone, owner *application.Application, opts ...CreateOption) error {
	options := &CreateOptions{}
	for _, opt := range opts {
		opt(options)
	}

	client := client.ClientFromContextOrDie(ctx)
	clientId := MakeClientName(owner)
	resourceName := clientId + "--" + zone.Name
	realmName := contextutil.EnvFromContextOrDie(ctx)

	realmRef := types.ObjectRef{
		Name:      realmName,
		Namespace: zone.Status.Namespace,
	}

	consumer := &gateway.Consumer{
		ObjectMeta: metav1.ObjectMeta{
			Name:      resourceName,
			Namespace: owner.GetNamespace(),
		},
	}

	mutator := func() error {
		consumer.Labels = map[string]string{
			config.BuildLabelKey("application"): owner.Name,
			config.BuildLabelKey("team"):        owner.Spec.Team,
			config.BuildLabelKey("realm"):       realmName,
			config.BuildLabelKey("zone"):        zone.Name,
		}
		if options.Failover {
			consumer.Labels[config.BuildLabelKey("failover")] = "true"
		}

		err := ctrl.SetControllerReference(owner, consumer, client.Scheme())
		if err != nil {
			return errors.Wrapf(err, "failed to set controller reference for gateway consumer %s", resourceName)
		}
		consumer.Spec = gateway.ConsumerSpec{
			Realm: realmRef,
			Name:  clientId,
		}

		if owner.Spec.Security != nil && owner.Spec.Security.IpRestrictions != nil {
			consumer.Spec.Security = &gateway.ConsumerSecurity{
				IpRestrictions: &gateway.IpRestrictions{
					Allow: owner.Spec.Security.IpRestrictions.Allow,
					Deny:  owner.Spec.Security.IpRestrictions.Deny,
				},
			}
		}

		return nil
	}

	_, err := client.CreateOrUpdate(ctx, consumer, mutator)
	if err != nil {
		return errors.Wrapf(err, "failed to create or update Gateway Consumer %s", resourceName)
	}

	owner.Status.Consumers = append(owner.Status.Consumers, *types.ObjectRefFromObject(consumer))

	return nil
}
