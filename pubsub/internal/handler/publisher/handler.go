// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package publisher

import (
	"context"

	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"

	cclient "github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/errors/ctrlerrors"
	"github.com/telekom/controlplane/common/pkg/handler"
	pubsubv1 "github.com/telekom/controlplane/pubsub/api/v1"
)

var _ handler.Handler[*pubsubv1.Publisher] = &PublisherHandler{}

type PublisherHandler struct{}

func (h *PublisherHandler) CreateOrUpdate(ctx context.Context, obj *pubsubv1.Publisher) error {
	c := cclient.ClientFromContextOrDie(ctx)

	// Validate that the referenced EventStore exists and is ready
	eventStore := &pubsubv1.EventStore{}
	err := c.Get(ctx, obj.Spec.EventStore.K8s(), eventStore)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return ctrlerrors.BlockedErrorf("EventStore %q not found", obj.Spec.EventStore.String())
		}
		return errors.Wrapf(err, "failed to get EventStore %q", obj.Spec.EventStore.String())
	}
	if err := condition.EnsureReady(eventStore); err != nil {
		return ctrlerrors.BlockedErrorf("EventStore %q is not ready", obj.Spec.EventStore.String())
	}

	// TODO: Call quasar/Config Server REST API to register publisher (if needed in the future)

	obj.SetCondition(condition.NewReadyCondition("PublisherReady", "Publisher is valid and EventStore is ready"))
	obj.SetCondition(condition.NewDoneProcessingCondition("Publisher is ready"))

	return nil
}

func (h *PublisherHandler) Delete(ctx context.Context, obj *pubsubv1.Publisher) error {
	// TODO: Call quasar/Config Server REST API to deregister publisher (if needed in the future)
	return nil
}
