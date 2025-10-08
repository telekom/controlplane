// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package consumer

import (
	"context"

	"github.com/pkg/errors"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/handler"
	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
	"github.com/telekom/controlplane/gateway/internal/features"
	"github.com/telekom/controlplane/gateway/internal/features/feature"
	"github.com/telekom/controlplane/gateway/internal/handler/gateway"
	"github.com/telekom/controlplane/gateway/internal/handler/realm"
	"github.com/telekom/controlplane/gateway/pkg/kongutil"
)

var _ handler.Handler[*gatewayv1.Consumer] = &ConsumerHandler{}

type ConsumerHandler struct{}

func (h *ConsumerHandler) CreateOrUpdate(ctx context.Context, consumer *gatewayv1.Consumer) error {
	builder, err := NewFeatureBuilder(ctx, consumer)
	if err != nil {
		return errors.Wrap(err, "failed to create feature builder")
	}
	if builder == nil {
		// Conditions are already set in NewFeatureBuilder
		return nil
	}

	if err := builder.BuildForConsumer(ctx); err != nil {
		return errors.Wrap(err, "failed to build route")
	}

	consumer.SetCondition(condition.NewDoneProcessingCondition("Consumer is ready", consumer))
	consumer.SetCondition(condition.NewReadyCondition("ConsumerReady", "Consumer is ready", consumer))

	return nil
}

func (h *ConsumerHandler) Delete(ctx context.Context, consumer *gatewayv1.Consumer) error {
	_, realm, err := realm.GetRealmByRef(ctx, consumer.Spec.Realm)
	if err != nil {
		return err
	}

	_, gateway, err := gateway.GetGatewayByRef(ctx, *realm.Spec.Gateway, true)
	if err != nil {
		return err
	}

	kc, err := kongutil.GetClientFor(gateway)
	if err != nil {
		return errors.Wrap(err, "failed to get kong client")
	}

	err = kc.DeleteConsumer(ctx, consumer)
	if err != nil {
		return errors.Wrap(err, "failed to create or update consumer")
	}

	return nil
}

func NewFeatureBuilder(ctx context.Context, consumer *gatewayv1.Consumer) (features.FeaturesBuilder, error) {
	ready, realm, err := realm.GetRealmByRef(ctx, consumer.Spec.Realm)
	if err != nil {
		return nil, err
	}
	if !ready {
		consumer.SetCondition(condition.NewBlockedCondition("Realm is not ready"))
		consumer.SetCondition(condition.NewNotReadyCondition("RealmNotReady", "Realm is not ready"))
		return nil, nil
	}

	ready, gateway, err := gateway.GetGatewayByRef(ctx, *realm.Spec.Gateway, true)
	if err != nil {
		return nil, err
	}
	if !ready {
		consumer.SetCondition(condition.NewBlockedCondition("Gateway is not ready"))
		consumer.SetCondition(condition.NewNotReadyCondition("GatewayNotReady", "Gateway is not ready"))
		return nil, nil
	}

	kc, err := kongutil.GetClientFor(gateway)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get kong client")
	}

	builder := features.NewFeatureBuilder(kc, nil, consumer, realm, gateway)
	builder.EnableFeature(feature.InstanceIpRestrictionFeature)

	return builder, nil
}
