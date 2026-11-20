// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v1

// MigrationStatus is the pending Kong change of a Route or Consumer that Migration Mode
// blocked. It is set only while the change is blocked and removed once it is written.
type MigrationStatus struct {
	// Hash identifies the pending change. Setting the annotation
	// gateway.cp.ei.telekom.de/migration-approved-hash to this value approves it.
	Hash string `json:"hash"`
	// Mode is the Migration Mode that blocked the change: on or strict.
	Mode string `json:"mode"`
	// Changes are the Kong entities the change creates, updates or deletes.
	// +optional
	// +listType=atomic
	Changes []MigrationChange `json:"changes,omitempty"`
}

// MigrationChange is the change of one Kong entity.
type MigrationChange struct {
	// Kind is the Kong entity kind, such as route, service or plugin.
	Kind string `json:"kind"`
	// Name is the Kong name of the entity: the plugin name for plugins, the target for
	// targets and the username for consumers.
	Name string `json:"name"`
	// Operation is create, update or delete.
	Operation string `json:"operation"`
	// Diffs are the differences between the desired and the current entity.
	// +optional
	// +listType=atomic
	Diffs []MigrationDiff `json:"diffs,omitempty"`
}

// MigrationDiff is one difference between the desired and the current state of a Kong entity.
type MigrationDiff struct {
	// Path is the JSON path of the difference, such as $.tags or $.config.minute.
	Path string `json:"path"`
	// Desired is the value the operator writes, as JSON. It is empty if Kong has a value the
	// operator removes. Long values are truncated.
	// +optional
	Desired string `json:"desired,omitempty"`
	// Current is the value Kong holds, as JSON. It is empty if the operator adds a value.
	// Long values are truncated.
	// +optional
	Current string `json:"current,omitempty"`
	// Ignored is true if the difference is on the ignore list.
	// +optional
	Ignored bool `json:"ignored,omitempty"`
}
