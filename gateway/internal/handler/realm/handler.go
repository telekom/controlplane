// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package realm

import (
	"context"

	stderrors "errors"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	"github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/handler"
	"github.com/telekom/controlplane/common/pkg/types"
	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
)

var _ handler.Handler[*gatewayv1.Realm] = &RealmHandler{}

type RealmHandler struct{}

func (h *RealmHandler) CreateOrUpdate(ctx context.Context, realm *gatewayv1.Realm) error {
	logger := logr.FromContextOrDiscard(ctx)
	realm.Status.Virtual = realm.Spec.Gateway == nil

	if !realm.Status.Virtual {
		if err := createRoutes(ctx, realm); err != nil {
			return err
		}
	}

	gc := client.ClientFromContextOrDie(ctx)

	n, err := gc.Cleanup(ctx, &gatewayv1.RouteList{}, client.OwnedBy(realm))
	if err != nil {
		return errors.Wrap(err, "failed to cleanup routes")
	}
	if n > 0 {
		logger.V(1).Info("Cleaned up routes", "count", n)
	}

	realm.SetCondition(condition.NewReadyCondition("RealmReady", "Realm has been provisioned"))
	realm.SetCondition(condition.NewDoneProcessingCondition("Realm has been provisioned"))

	return nil
}

func (h *RealmHandler) Delete(ctx context.Context, realm *gatewayv1.Realm) error {
	return nil
}

func createRoutes(ctx context.Context, realm *gatewayv1.Realm) error {

	route, err := CreateRoute(ctx, realm, gatewayv1.RouteTypeIssuer)
	switch {
	case stderrors.Is(err, ErrRouteDisabled):
		// Route has been disabled via RouteOverwrite, so we should not create it and also not return an error
		realm.Status.IssuerRoute = nil
		realm.Status.IssuerUrl = ""
	case err != nil:
		// An error occurred while creating the route, return the error
		return errors.Wrapf(err, "failed to create route %q", gatewayv1.RouteTypeIssuer)
	default:
		// Route has been created successfully, update the status with the route reference and URL
		realm.Status.IssuerRoute = types.ObjectRefFromObject(route)
		realm.Status.IssuerUrl = route.Spec.Downstreams[0].Url()
	}

	route, err = CreateRoute(ctx, realm, gatewayv1.RouteTypeCerts)
	switch {
	case stderrors.Is(err, ErrRouteDisabled):
		// Route has been disabled via RouteOverwrite, so we should not create it and also not return an error
		realm.Status.CertsRoute = nil
		realm.Status.CertsUrl = ""
	case err != nil:
		// An error occurred while creating the route, return the error
		return errors.Wrapf(err, "failed to create route %q", gatewayv1.RouteTypeCerts)
	default:
		// Route has been created successfully, update the status with the route reference and URL
		realm.Status.CertsRoute = types.ObjectRefFromObject(route)
		realm.Status.CertsUrl = route.Spec.Downstreams[0].Url()
	}

	route, err = CreateRoute(ctx, realm, gatewayv1.RouteTypeDiscovery)
	switch {
	case stderrors.Is(err, ErrRouteDisabled):
		// Route has been disabled via RouteOverwrite, so we should not create it and also not return an error
		realm.Status.DiscoveryRoute = nil
		realm.Status.DiscoveryUrl = ""
	case err != nil:
		// An error occurred while creating the route, return the error
		return errors.Wrapf(err, "failed to create route %q", gatewayv1.RouteTypeDiscovery)
	default:
		// Route has been created successfully, update the status with the route reference and URL
		realm.Status.DiscoveryRoute = types.ObjectRefFromObject(route)
		realm.Status.DiscoveryUrl = route.Spec.Downstreams[0].Url()
	}

	return nil
}
