// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package eventstore

import (
	"context"

	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/handler"
	pubsubv1 "github.com/telekom/controlplane/pubsub/api/v1"
)

var _ handler.Handler[*pubsubv1.EventStore] = &EventStoreHandler{}

type EventStoreHandler struct{}

func (h *EventStoreHandler) CreateOrUpdate(ctx context.Context, obj *pubsubv1.EventStore) error {

	obj.SetCondition(condition.NewReadyCondition("EventStoreReady", "EventStore configuration is valid"))
	obj.SetCondition(condition.NewDoneProcessingCondition("EventStore is ready"))

	return nil
}

func (h *EventStoreHandler) Delete(ctx context.Context, obj *pubsubv1.EventStore) error {
	// EventStore is a configuration resource - no external cleanup needed.
	return nil
}
