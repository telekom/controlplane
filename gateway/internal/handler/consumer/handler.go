// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package consumer

import (
	"context"

	"github.com/pkg/errors"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/errors/ctrlerrors"
	"github.com/telekom/controlplane/common/pkg/handler"
	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
	"github.com/telekom/controlplane/gateway/internal/features"
	"github.com/telekom/controlplane/gateway/internal/features/kong"
	"github.com/telekom/controlplane/gateway/internal/features/kong/feature"
	"github.com/telekom/controlplane/gateway/internal/handler/gateway"
	"github.com/telekom/controlplane/gateway/pkg/kongutil"
)

var _ handler.Handler[*gatewayv1.Consumer] = &ConsumerHandler{}

type ConsumerHandler struct{}

func (h *ConsumerHandler) CreateOrUpdate(ctx context.Context, consumer *gatewayv1.Consumer) error {
	ready, gw, err := gateway.GetGatewayByRef(ctx, consumer.Spec.Gateway, true)
	if err != nil {
		return err
	}
	if !ready {
		return ctrlerrors.BlockedErrorf("gateway %s is not ready", consumer.Spec.Gateway.Name)
	}

	// Envoy has no notion of consumers. Report Ready (owners may depend on it) and skip.
	if gw.Spec.GatewayClassName == gatewayv1.GatewayClassNameEnvoy {
		consumer.SetCondition(condition.NewDoneProcessingCondition("Consumer is not supported for Envoy gateway"))
		consumer.SetCondition(condition.NewReadyCondition("ConsumerReady", "Consumer is ready"))
		return nil
	}

	builder, err := NewFeatureBuilder(ctx, gw, consumer)
	if err != nil {
		return errors.Wrap(err, "failed to create feature builder")
	}

	if err := builder.BuildForConsumer(ctx); err != nil {
		return errors.Wrap(err, "failed to build consumer")
	}

	consumer.SetCondition(condition.NewDoneProcessingCondition("Consumer is ready"))
	consumer.SetCondition(condition.NewReadyCondition("ConsumerReady", "Consumer is ready"))

	return nil
}

func (h *ConsumerHandler) Delete(ctx context.Context, consumer *gatewayv1.Consumer) error {

	_, gateway, err := gateway.GetGatewayByRef(ctx, consumer.Spec.Gateway, true)
	if err != nil {
		return err
	}

	// Envoy has no notion of consumers; nothing was created, nothing to delete.
	if gateway.Spec.GatewayClassName == gatewayv1.GatewayClassNameEnvoy {
		return nil
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

func NewFeatureBuilder(ctx context.Context, gateway *gatewayv1.Gateway, consumer *gatewayv1.Consumer) (features.KongFeatureBuilder, error) {

	kc, err := kongutil.GetClientFor(gateway)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get kong client")
	}

	builder := kong.NewFeatureBuilder(kc, nil, consumer, gateway)
	builder.EnableFeature(feature.InstanceIpRestrictionFeature)

	return builder, nil
}
