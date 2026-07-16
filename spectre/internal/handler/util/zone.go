// SPDX-FileCopyrightText: 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package util

import (
	"context"

	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	"github.com/telekom/controlplane/common/pkg/errors/ctrlerrors"
)

// GetListeningZone determines which zone should host the EventStore subscription
// for a Spectre listener. It ports the legacy ListenerUtil.getListeningZone
// preference logic:
//
//  1. If listenerZone has an EventStore that matches providerZone -> use listenerZone
//  2. If listenerZone has an EventStore that matches consumerZone -> use listenerZone
//  3. Otherwise fall back to providerZone (if it has an EventStore)
//  4. Otherwise fall back to consumerZone (if it has an EventStore)
//  5. If no zone has an EventStore -> return BlockedError
//
// A zone "has an EventStore" if GetEventConfig succeeds for that zone.
func GetListeningZone(ctx context.Context, listenerZone, providerZone, consumerZone *adminv1.Zone) (*adminv1.Zone, error) {
	// Check if the listener zone has an EventConfig (i.e. it is an EventStore zone)
	listenerEC, listenerErr := GetEventConfig(ctx, listenerZone)
	if listenerErr == nil && listenerEC != nil {
		// Listener zone has an EventStore. Check if it supports provider or consumer zone.
		if listenerEC.SupportsZone(providerZone.Name) {
			return listenerZone, nil
		}
		if listenerEC.SupportsZone(consumerZone.Name) {
			return listenerZone, nil
		}
	}

	// Fall back to provider zone
	_, providerErr := GetEventConfig(ctx, providerZone)
	if providerErr == nil {
		return providerZone, nil
	}

	// Fall back to consumer zone
	_, consumerErr := GetEventConfig(ctx, consumerZone)
	if consumerErr == nil {
		return consumerZone, nil
	}

	return nil, ctrlerrors.BlockedErrorf("no zone has an EventStore: listener=%q, provider=%q, consumer=%q",
		listenerZone.Name, providerZone.Name, consumerZone.Name)
}
