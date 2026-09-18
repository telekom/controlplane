// SPDX-FileCopyrightText: 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package util

import (
	"context"

	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	"github.com/telekom/controlplane/common/pkg/errors/ctrlerrors"
)

// GetListeningZone determines which zone should host the EventStore subscription
// for a Spectre listener.
//
// The consumer zone IS the listener zone: the consuming application is the one
// doing the listening, which is why the same application also supplies the
// requester identity for the listener approval.
//
// Preference order:
//
//  1. The consumer zone, if it has an EventStore whose mesh reaches providerZone
//  2. Otherwise the provider zone, if it has an EventStore
//  3. Otherwise BlockedError
//
// A zone "has an EventStore" if GetEventConfig succeeds for that zone. Step 1 is
// what enforces the mesh restriction — a consumer zone whose mesh does not name
// the provider zone cannot carry the subscription, so the provider zone wins.
func GetListeningZone(ctx context.Context, providerZone, consumerZone *adminv1.Zone) (*adminv1.Zone, error) {
	// Prefer the consumer (listener) zone, but only if its mesh reaches the provider zone.
	consumerEC, consumerErr := GetEventConfig(ctx, consumerZone)
	if consumerErr == nil && consumerEC != nil && consumerEC.SupportsZone(providerZone.Name) {
		return consumerZone, nil
	}

	// Fall back to the provider zone.
	if _, providerErr := GetEventConfig(ctx, providerZone); providerErr == nil {
		return providerZone, nil
	}

	return nil, ctrlerrors.BlockedErrorf("no zone has an EventStore: provider=%q, consumer=%q",
		providerZone.Name, consumerZone.Name)
}
