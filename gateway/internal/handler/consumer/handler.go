// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package consumer

import (
	"context"

	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/api/meta"

	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/errors/ctrlerrors"
	"github.com/telekom/controlplane/common/pkg/handler"
	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
	"github.com/telekom/controlplane/gateway/internal/features"
	"github.com/telekom/controlplane/gateway/internal/features/feature"
	"github.com/telekom/controlplane/gateway/internal/handler/gateway"
	"github.com/telekom/controlplane/gateway/pkg/kongutil"
)

var _ handler.Handler[*gatewayv1.Consumer] = &ConsumerHandler{}

type ConsumerHandler struct{}

func (h *ConsumerHandler) CreateOrUpdate(ctx context.Context, consumer *gatewayv1.Consumer) error {
	ready, referencedGateway, err := gateway.GetGatewayByRef(ctx, consumer.Spec.Gateway, false)
	if err != nil {
		return errors.Wrap(err, "failed to create feature builder")
	}
	if !ready {
		return errors.Wrap(
			ctrlerrors.BlockedErrorf("gateway %s is not ready", consumer.Spec.Gateway.Name),
			"failed to create feature builder",
		)
	}
	if referencedGateway.Spec.Type == gatewayv1.GatewayTypeEnvoy {
		if cleanupErr := cleanupKongConsumer(ctx, consumer); cleanupErr != nil {
			return cleanupErr
		}
		consumer.SetCondition(condition.NewDoneProcessingCondition("Consumer is ready"))
		consumer.SetCondition(condition.NewReadyCondition("ConsumerReady", "Consumer is ready"))
		return nil
	}

	builder, err := NewFeatureBuilder(ctx, consumer)
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

func cleanupKongConsumer(ctx context.Context, consumer *gatewayv1.Consumer) error {
	if len(consumer.Status.Properties) == 0 {
		return nil
	}
	_, referencedGateway, err := gateway.GetGatewayByRef(ctx, consumer.Spec.Gateway, true)
	if err != nil {
		return errors.Wrap(err, "failed to resolve gateway for Kong cleanup")
	}
	if !meta.IsStatusConditionTrue(referencedGateway.Status.Conditions, "XDSProgrammed") {
		return ctrlerrors.BlockedErrorf("gateway %s xDS target is not programmed", consumer.Spec.Gateway.Name)
	}
	kc, err := kongutil.GetClientFor(referencedGateway)
	if err != nil {
		return errors.Wrap(err, "failed to get Kong client for backend switch")
	}
	if err := kc.DeleteConsumer(ctx, consumer); err != nil {
		return errors.Wrap(err, "failed to clean up Kong consumer during backend switch")
	}
	consumer.Status.Properties = map[string]string{}
	return nil
}

func (h *ConsumerHandler) Delete(ctx context.Context, consumer *gatewayv1.Consumer) error {
	ready, referencedGateway, err := gateway.GetGatewayByRef(ctx, consumer.Spec.Gateway, false)
	if err != nil {
		return err
	}
	if referencedGateway.Spec.Type == gatewayv1.GatewayTypeEnvoy {
		return nil
	}
	if !ready {
		return ctrlerrors.BlockedErrorf("gateway %s is not ready", consumer.Spec.Gateway.Name)
	}
	_, referencedGateway, err = gateway.GetGatewayByRef(ctx, consumer.Spec.Gateway, true)
	if err != nil {
		return err
	}

	kc, err := kongutil.GetClientFor(referencedGateway)
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
	ready, referencedGateway, err := gateway.GetGatewayByRef(ctx, consumer.Spec.Gateway, true)
	if err != nil {
		return nil, err
	}
	if !ready {
		return nil, ctrlerrors.BlockedErrorf("gateway %s is not ready", consumer.Spec.Gateway.Name)
	}

	kc, err := kongutil.GetClientFor(referencedGateway)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get kong client")
	}

	builder := features.NewFeatureBuilder(kc, nil, consumer, referencedGateway)
	builder.EnableFeature(feature.InstanceIpRestrictionFeature)

	return builder, nil
}
