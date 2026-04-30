// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package eventtype

import (
	"context"
	"sort"

	"github.com/pkg/errors"
	"sigs.k8s.io/controller-runtime/pkg/log"

	cclient "github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/handler"
	eventv1 "github.com/telekom/controlplane/event/api/v1"
)

var _ handler.Handler[*eventv1.EventType] = &EventTypeHandler{}

type EventTypeHandler struct{}

func (h *EventTypeHandler) CreateOrUpdate(ctx context.Context, obj *eventv1.EventType) error {
	logger := log.FromContext(ctx)
	c := cclient.ClientFromContextOrDie(ctx)

	// List all EventTypes in the environment
	eventTypeList := &eventv1.EventTypeList{}
	if err := c.List(ctx, eventTypeList); err != nil {
		return errors.Wrap(err, "failed to list EventTypes")
	}

	// Filter to only those matching our spec.type
	var candidates []eventv1.EventType
	for i := range eventTypeList.Items {
		if eventTypeList.Items[i].Spec.Type == obj.Spec.Type {
			candidates = append(candidates, eventTypeList.Items[i])
		}
	}

	// Sort by CreationTimestamp ascending (oldest first)
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].CreationTimestamp.Before(&candidates[j].CreationTimestamp)
	})

	// Find the first non-deleted candidate — that one is the active singleton
	var activeCandidate *eventv1.EventType
	for i := range candidates {
		if candidates[i].DeletionTimestamp == nil || candidates[i].DeletionTimestamp.IsZero() {
			activeCandidate = &candidates[i]
			break
		}
	}

	// Determine if this EventType is the active one
	if activeCandidate != nil && activeCandidate.UID == obj.UID {
		obj.Status.Active = true
		obj.SetCondition(condition.NewReadyCondition("EventTypeActive",
			"EventType is the active singleton for its type"))
		obj.SetCondition(condition.NewDoneProcessingCondition("EventType is active"))
		logger.V(1).Info("EventType is active", "type", obj.Spec.Type)
	} else {
		obj.Status.Active = false
		msg := "Another EventType with the same type identifier is already active"
		obj.SetCondition(condition.NewNotReadyCondition("EventTypeNotActive", msg))
		obj.SetCondition(condition.NewBlockedCondition(msg))
		logger.V(1).Info("EventType is not active (blocked by older resource)", "type", obj.Spec.Type)
	}

	return nil
}

func (h *EventTypeHandler) Delete(ctx context.Context, obj *eventv1.EventType) error {
	// When the active EventType is deleted, the controller-runtime will re-reconcile
	// the remaining EventTypes (via watches in the controller). The next-oldest will
	// naturally become the active one during its next reconciliation.
	// No manual cleanup is needed.
	return nil
}
