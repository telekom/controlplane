// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package gateway

import (
	"context"

	"github.com/pkg/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	cc "github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/handler"
	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
)

var _ handler.Handler[*gatewayv1.Gateway] = &GatewayHandler{}

type GatewayHandler struct{}

func (h *GatewayHandler) CreateOrUpdate(ctx context.Context, gw *gatewayv1.Gateway) error {

	gw.SetCondition(condition.NewDoneProcessingCondition("Created Gateway", gw))
	gw.SetCondition(condition.NewReadyCondition("Ready", "Gateway is ready", gw))
	return nil
}

func (h *GatewayHandler) Delete(ctx context.Context, object *gatewayv1.Gateway) error {
	cclient := cc.ClientFromContextOrDie(ctx)

	// If the Gateway which is referenced by the realms is deleted, we need to delete
	// the realms as well.
	realms := &gatewayv1.RealmList{}
	err := cclient.List(ctx, realms, client.InNamespace(object.Namespace))
	if err != nil {
		return errors.Wrap(err, "failed to list realms")
	}

	for _, realm := range realms.Items {
		if realm.Spec.Gateway.Equals(object) {
			err := cclient.Delete(ctx, &realm)
			if err != nil {
				return errors.Wrapf(err, "failed to delete realm %s", realm.Name)
			}
		}
	}

	return nil
}
