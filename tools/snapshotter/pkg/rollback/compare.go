// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package rollback

import (
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/telekom/controlplane/tools/snapshotter/pkg/snapshot"
)

// timestampFields are ignored when comparing entities and omitted from payloads.
var timestampFields = []string{"created_at", "updated_at"}

// toPayload converts a Kong entity into a generic JSON object without timestamps.
func toPayload(entity any) (map[string]any, error) {
	data, err := json.Marshal(entity)
	if err != nil {
		return nil, fmt.Errorf("marshalling entity: %w", err)
	}
	obj := map[string]any{}
	if err := json.Unmarshal(data, &obj); err != nil {
		return nil, fmt.Errorf("unmarshalling entity: %w", err)
	}
	for _, f := range timestampFields {
		delete(obj, f)
	}
	return obj, nil
}

// equalIgnoringTimestamps reports whether two entities are equal apart from timestamps.
func equalIgnoringTimestamps(a, b any) bool {
	pa, errA := toPayload(a)
	pb, errB := toPayload(b)
	if errA != nil || errB != nil {
		return false
	}
	return reflect.DeepEqual(pa, pb)
}

// withoutTimestamps returns a deep copy of the snapshot with all entity timestamps removed.
// The original snapshot is not modified.
func withoutTimestamps(snap *snapshot.Snapshot) *snapshot.Snapshot {
	c := snap.DeepCopy()
	if c == nil || c.State == nil {
		return c
	}
	s := c.State
	if s.Service != nil {
		s.Service.CreatedAt, s.Service.UpdatedAt = nil, nil
	}
	if s.Route != nil {
		s.Route.CreatedAt, s.Route.UpdatedAt = nil, nil
	}
	if s.Upstream != nil {
		s.Upstream.CreatedAt = nil
	}
	for i := range s.Plugins {
		s.Plugins[i].CreatedAt, s.Plugins[i].UpdatedAt = nil, nil
	}
	for i := range s.Targets {
		s.Targets[i].CreatedAt = nil
	}
	return c
}
