// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package consumer

import (
	"context"

	"github.com/pkg/errors"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/handler"
	v1 "github.com/telekom/controlplane/gateway/api/v1"
	"github.com/telekom/controlplane/gateway/internal/handler/gateway"
	"github.com/telekom/controlplane/gateway/internal/handler/realm"
	"github.com/telekom/controlplane/gateway/pkg/kongutil"
)

var _ handler.Handler[*v1.Consumer] = &ConsumerHandler{}

type ConsumerHandler struct{}

func (h *ConsumerHandler) CreateOrUpdate(ctx context.Context, consumer *v1.Consumer) error {

	consumer.SetCondition(condition.NewProcessingCondition("Processing", "Processing consumer"))
	consumer.SetCondition(condition.NewNotReadyCondition("ConsumerNotReady", "Consumer not ready"))

	ready, realm, err := realm.GetRealmByRef(ctx, consumer.Spec.Realm)
	if err != nil {
		return err
	}
	if !ready {
		consumer.SetCondition(condition.NewBlockedCondition("Realm not ready"))
		consumer.SetCondition(condition.NewNotReadyCondition("RealmNotReady", "Realm not ready"))
		return nil
	}

	ready, gateway, err := gateway.GetGatewayByRef(ctx, *realm.Spec.Gateway, true)
	if err != nil {
		return err
	}
	if !ready {
		consumer.SetCondition(condition.NewBlockedCondition("Gateway not ready"))
		consumer.SetCondition(condition.NewNotReadyCondition("GatewayNotReady", "Gateway not ready"))
		return nil
	}

	kc, err := kongutil.GetClientFor(gateway)
	if err != nil {
		return errors.Wrap(err, "failed to get kong client")
	}

	err = kc.CreateOrReplaceConsumer(ctx, consumer.Spec.Name)
	if err != nil {
		return errors.Wrap(err, "failed to create or update consumer")
	}

	consumer.SetCondition(condition.NewDoneProcessingCondition("Consumer is ready"))
	consumer.SetCondition(condition.NewReadyCondition("ConsumerReady", "Consumer is ready"))

	return nil
}

func (h *ConsumerHandler) Delete(ctx context.Context, consumer *v1.Consumer) error {
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

	err = kc.DeleteConsumer(ctx, consumer.Spec.Name)
	if err != nil {
		return errors.Wrap(err, "failed to create or update consumer")
	}

	return nil
}
