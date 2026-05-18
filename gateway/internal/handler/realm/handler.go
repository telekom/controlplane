// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package realm

import (
	"context"

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
	if err != nil {
		return errors.Wrapf(err, "failed to create route %q", gatewayv1.RouteTypeIssuer)
	}
	if route != nil {
		realm.Status.IssuerRoute = types.ObjectRefFromObject(route)
		realm.Status.IssuerUrl = route.Spec.Downstreams[0].Url()
	} else {
		realm.Status.IssuerRoute = nil
		realm.Status.IssuerUrl = ""
	}

	route, err = CreateRoute(ctx, realm, gatewayv1.RouteTypeCerts)
	if err != nil {
		return errors.Wrapf(err, "failed to create route %q", gatewayv1.RouteTypeCerts)
	}
	if route != nil {
		realm.Status.CertsRoute = types.ObjectRefFromObject(route)
		realm.Status.CertsUrl = route.Spec.Downstreams[0].Url()
	} else {
		realm.Status.CertsRoute = nil
		realm.Status.CertsUrl = ""
	}

	route, err = CreateRoute(ctx, realm, gatewayv1.RouteTypeDiscovery)
	if err != nil {
		return errors.Wrapf(err, "failed to create route %q", gatewayv1.RouteTypeDiscovery)
	}
	if route != nil {
		realm.Status.DiscoveryRoute = types.ObjectRefFromObject(route)
		realm.Status.DiscoveryUrl = route.Spec.Downstreams[0].Url()
	} else {
		realm.Status.DiscoveryRoute = nil
		realm.Status.DiscoveryUrl = ""
	}

	return nil
}
