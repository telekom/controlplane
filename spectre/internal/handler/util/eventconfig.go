// SPDX-FileCopyrightText: 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package util

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	cclient "github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/errors/ctrlerrors"
	eventv1 "github.com/telekom/controlplane/event/api/v1"
	pubsubv1 "github.com/telekom/controlplane/pubsub/api/v1"
)

// EventConfigZoneIndex is the field index path used to look up EventConfigs by zone name.
const EventConfigZoneIndex = ".spec.zone.name"

// GetEventConfig retrieves the unique EventConfig for the given zone.
// It uses the field index on spec.zone.name for efficient lookup.
// Returns an error if multiple EventConfigs exist for the zone (ambiguity),
// BlockedError if no EventConfig is found, and BlockedError if it is not ready.
func GetEventConfig(ctx context.Context, zone *adminv1.Zone) (*eventv1.EventConfig, error) {
	c := cclient.ClientFromContextOrDie(ctx)

	eventConfigList := &eventv1.EventConfigList{}
	err := c.List(ctx, eventConfigList,
		client.MatchingFields{EventConfigZoneIndex: zone.Name})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to list EventConfigs for zone %q", zone.Name)
	}

	if len(eventConfigList.Items) == 0 {
		return nil, ctrlerrors.BlockedErrorf("no EventConfig found for zone %q", zone.Name)
	}

	if len(eventConfigList.Items) > 1 {
		return nil, fmt.Errorf("found %d EventConfigs for zone %q, expected exactly one", len(eventConfigList.Items), zone.Name)
	}

	eventConfig := &eventConfigList.Items[0]

	if err := condition.EnsureReady(eventConfig); err != nil {
		return nil, ctrlerrors.BlockedErrorf("EventConfig %q for zone %q is not ready", eventConfig.Name, zone.Name)
	}

	return eventConfig, nil
}

// ResolveEventStore resolves the EventStore for the given zone by following the
// EventConfig's Status.EventStore reference. Callers that already fetched the
// EventConfig via GetEventConfig should pass it directly to avoid a duplicate List.
func ResolveEventStore(ctx context.Context, eventConfig *eventv1.EventConfig) (*pubsubv1.EventStore, error) {
	c := cclient.ClientFromContextOrDie(ctx)

	if eventConfig.Status.EventStore == nil {
		return nil, ctrlerrors.BlockedErrorf("EventConfig %q has no EventStore reference", eventConfig.Name)
	}

	eventStore := &pubsubv1.EventStore{}
	if err := c.Get(ctx, eventConfig.Status.EventStore.K8s(), eventStore); err != nil {
		return nil, errors.Wrapf(err, "failed to get EventStore %q referenced by EventConfig %q",
			eventConfig.Status.EventStore.String(), eventConfig.Name)
	}

	if err := condition.EnsureReady(eventStore); err != nil {
		return nil, ctrlerrors.BlockedErrorf("EventStore %q is not ready", eventStore.Name)
	}

	return eventStore, nil
}
