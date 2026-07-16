// SPDX-FileCopyrightText: 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package util

import (
	"context"

	"github.com/pkg/errors"
	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	cclient "github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/errors/ctrlerrors"
	eventv1 "github.com/telekom/controlplane/event/api/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// EventConfigZoneIndex is the field index path used to look up EventConfigs by zone name.
const EventConfigZoneIndex = ".spec.zone.name"

// GetEventConfig retrieves the EventConfig for the given zone.
// It uses the field index on spec.zone.name for efficient lookup.
// Returns BlockedError if no EventConfig is found or if it is not ready.
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

	eventConfig := &eventConfigList.Items[0]

	if err := condition.EnsureReady(eventConfig); err != nil {
		return nil, ctrlerrors.BlockedErrorf("EventConfig %q for zone %q is not ready", eventConfig.Name, zone.Name)
	}

	return eventConfig, nil
}
