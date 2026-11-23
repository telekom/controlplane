// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package consumer

import (
	"context"

	"github.com/pkg/errors"

	"github.com/telekom/controlplane/common/pkg/changeset"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/errors/ctrlerrors"
	"github.com/telekom/controlplane/common/pkg/handler"
	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
	"github.com/telekom/controlplane/gateway/internal/features"
	"github.com/telekom/controlplane/gateway/internal/features/feature"
	gatewayhandler "github.com/telekom/controlplane/gateway/internal/handler/gateway"
	"github.com/telekom/controlplane/gateway/pkg/kongutil"
)

var _ handler.Handler[*gatewayv1.Consumer] = &ConsumerHandler{}

type ConsumerHandler struct {
	// KongClients provides leased Kong clients for Gateways.
	KongClients *kongutil.ClientFactory
}

func (h *ConsumerHandler) CreateOrUpdate(ctx context.Context, consumer *gatewayv1.Consumer) error {
	// The Kong changes of the Consumer are planned, decided on as one bundle and only then written.
	if err := changeset.Run(ctx, kongutil.Gate(), consumer, "Consumer", h.build); err != nil {
		return err
	}

	consumer.SetCondition(condition.NewDoneProcessingCondition("Consumer is ready"))
	consumer.SetCondition(condition.NewReadyCondition("ConsumerReady", "Consumer is ready"))

	return nil
}

func (h *ConsumerHandler) Delete(ctx context.Context, consumer *gatewayv1.Consumer) error {
	_, gateway, err := gatewayhandler.GetGatewayByRef(ctx, consumer.Spec.Gateway, true)
	if err != nil {
		return err
	}

	kc, release, err := kongutil.GetClientFor(ctx, h.KongClients, gateway)
	if err != nil {
		return errors.Wrap(err, "failed to get kong client")
	}
	defer release()

	return changeset.Run(ctx, kongutil.Gate(), consumer, "Consumer", func(ctx context.Context, target *gatewayv1.Consumer) error {
		if err := kc.DeleteConsumer(ctx, target); err != nil {
			return errors.Wrap(err, "failed to create or update consumer")
		}
		return nil
	})
}

// build builds the Kong entities of consumer. In a ctx with a plan, it records them
// without writing.
func (h *ConsumerHandler) build(ctx context.Context, consumer *gatewayv1.Consumer) error {
	builder, release, err := h.newFeatureBuilder(ctx, consumer)
	if err != nil {
		return errors.Wrap(err, "failed to create feature builder")
	}
	defer release()

	if err := builder.BuildForConsumer(ctx); err != nil {
		return errors.Wrap(err, "failed to build consumer")
	}
	return nil
}

// newFeatureBuilder returns a builder using a leased Kong client and the
// function releasing the lease.
func (h *ConsumerHandler) newFeatureBuilder(ctx context.Context, consumer *gatewayv1.Consumer) (features.FeaturesBuilder, func(), error) {
	ready, gateway, err := gatewayhandler.GetGatewayByRef(ctx, consumer.Spec.Gateway, true)
	if err != nil {
		return nil, nil, err
	}
	if !ready {
		return nil, nil, ctrlerrors.BlockedErrorf("gateway %s is not ready", consumer.Spec.Gateway.Name)
	}

	kc, release, err := kongutil.GetClientFor(ctx, h.KongClients, gateway)
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to get kong client")
	}

	builder := features.NewFeatureBuilder(kc, nil, consumer, gateway)
	builder.EnableFeature(feature.InstanceIpRestrictionFeature)

	return builder, release, nil
}
