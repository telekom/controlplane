// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package util

import (
	"context"

	"github.com/pkg/errors"
	cclient "github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/config"
	"github.com/telekom/controlplane/common/pkg/errors/ctrlerrors"
	ctypes "github.com/telekom/controlplane/common/pkg/types"
	pubsubv1 "github.com/telekom/controlplane/pubsub/api/v1"
	secrets "github.com/telekom/controlplane/secret-manager/api"
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

// GetEventStore retrieves the EventStore referenced by objRef and ensures it is ready.
// If the EventStore contains a secret-reference AND the secret manager feature is enabled,
// it resolves the secret and updates the EventStore spec accordingly.
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

	if config.FeatureSecretManager.IsEnabled() {
		clientSecret := eventStore.Spec.ClientSecret
		if secrets.IsRef(clientSecret) {
			clientSecretValue, err := secrets.Get(ctx, clientSecret)
			if err != nil {
				return nil, errors.Wrapf(err, "failed to resolve client secret for EventStore %q", objRef.String())
			}
			eventStore.Spec.ClientSecret = clientSecretValue
		}
	}
	return eventStore, nil
}
