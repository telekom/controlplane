// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package util

import (
	"context"

	"github.com/pkg/errors"
	cclient "github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/errors/ctrlerrors"
	ctypes "github.com/telekom/controlplane/common/pkg/types"
	pubsubv1 "github.com/telekom/controlplane/pubsub/api/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

func GetPublisher(ctx context.Context, objRef ctypes.ObjectRef) (*pubsubv1.Publisher, error) {
	c := cclient.ClientFromContextOrDie(ctx)

	publisher := &pubsubv1.Publisher{}
	err := c.Get(ctx, objRef.K8s(), publisher)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, ctrlerrors.BlockedErrorf("Publisher %q not found", objRef.String())
		}
		return nil, errors.Wrapf(err, "failed to get Publisher %q", objRef.String())
	}
	if err := condition.EnsureReady(publisher); err != nil {
		return nil, ctrlerrors.BlockedErrorf("Publisher %q is not ready", objRef.String())
	}

	return publisher, nil
}

func GetEventStore(ctx context.Context, objRef ctypes.ObjectRef) (*pubsubv1.EventStore, error) {
	c := cclient.ClientFromContextOrDie(ctx)

	eventStore := &pubsubv1.EventStore{}
	err := c.Get(ctx, objRef.K8s(), eventStore)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, ctrlerrors.BlockedErrorf("EventStore %q not found", objRef.String())
		}
		return nil, errors.Wrapf(err, "failed to get EventStore %q", objRef.String())
	}
	if err := condition.EnsureReady(eventStore); err != nil {
		return nil, ctrlerrors.BlockedErrorf("EventStore %q is not ready", objRef.String())
	}

	return eventStore, nil
}
