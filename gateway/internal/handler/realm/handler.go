// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package realm

import (
	"context"

	"github.com/pkg/errors"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/handler"
	"github.com/telekom/controlplane/common/pkg/types"
	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
)

var _ handler.Handler[*gatewayv1.Realm] = &RealmHandler{}

type RealmHandler struct{}

func (h *RealmHandler) CreateOrUpdate(ctx context.Context, realm *gatewayv1.Realm) error {

	realm.SetCondition(condition.NewProcessingCondition("RealmProcessing", "Realm is being provisioned"))

	realm.Status.Virtual = realm.Spec.Gateway == nil

	if !realm.Status.Virtual {
		if err := createRoutes(ctx, realm); err != nil {
			return err
		}
	}

	realm.SetCondition(condition.NewReadyCondition("RealmReady", "Realm has been provisioned"))
	realm.SetCondition(condition.NewDoneProcessingCondition("Realm has been provisioned"))

	return nil
}

func (h *RealmHandler) Delete(ctx context.Context, realm *gatewayv1.Realm) error {
	return nil
}

func createRoutes(ctx context.Context, realm *gatewayv1.Realm) error {

	route, err := CreateRoute(ctx, realm, RouteTypeIssuer)
	if err != nil {
		return errors.Wrapf(err, "failed to create route '%s'", RouteTypeIssuer)
	}
	realm.Status.IssuerRoute = types.ObjectRefFromObject(route)
	realm.Status.IssuerUrl = route.Spec.Downstreams[0].Url()

	route, err = CreateRoute(ctx, realm, RouteTypeCerts)
	if err != nil {
		return errors.Wrapf(err, "failed to create route '%s'", RouteTypeCerts)
	}
	realm.Status.CertsRoute = types.ObjectRefFromObject(route)
	realm.Status.CertsUrl = route.Spec.Downstreams[0].Url()

	route, err = CreateRoute(ctx, realm, RouteTypeDiscovery)
	if err != nil {
		return errors.Wrapf(err, "failed to create route '%s'", RouteTypeDiscovery)
	}
	realm.Status.DiscoveryRoute = types.ObjectRefFromObject(route)
	realm.Status.DiscoveryUrl = route.Spec.Downstreams[0].Url()

	return nil
}
