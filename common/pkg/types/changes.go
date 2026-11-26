// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package types

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// MigrationStatus is the pending backend change of an object that Migration Mode blocked.
// It is set only while the change is blocked.
type MigrationStatus struct {
	// Hash identifies the pending change. Setting the annotation
	// cp.ei.telekom.de/migration-approved-hash to this value approves it.
	Hash string `json:"hash"`
	// Mode is the Migration Mode that blocked the change: on or strict.
	Mode string `json:"mode"`
	// Changes are the backend entities the change creates, updates or deletes.
	// +optional
	// +listType=atomic
	Changes []Change `json:"changes,omitempty"`
	// Truncated is true if changes or values were cut to fit the size limits.
	// +optional
	Truncated bool `json:"truncated,omitempty"`
}

// AppliedChanges records the last backend change of an object that was written. It is
// replaced on every applied change and kept as an audit record.
type AppliedChanges struct {
	// Hash identifies the applied change.
	Hash string `json:"hash"`
	// AppliedAt is the time the change was written.
	AppliedAt metav1.Time `json:"appliedAt"`
	// Generation is the metadata.generation the change was built from.
	Generation int64 `json:"generation"`
	// Mode is the Migration Mode at the time of the change: off, on or strict.
	Mode string `json:"mode"`
	// Decision tells why the change was written: allowed or approved.
	Decision string `json:"decision"`
	// ApprovedHash is the approved hash if the change was approved.
	// +optional
	ApprovedHash string `json:"approvedHash,omitempty"`
	// Changes are the backend entities the change created, updated or deleted.
	// +optional
	// +listType=atomic
	Changes []Change `json:"changes,omitempty"`
	// Truncated is true if changes or values were cut to fit the size limits.
	// +optional
	Truncated bool `json:"truncated,omitempty"`
}

// Change is the change of one backend entity.
type Change struct {
	// Kind is the backend entity kind, such as route, plugin or client.
	Kind string `json:"kind"`
	// Name names the entity in the backend.
	Name string `json:"name"`
	// Operation is create, update or delete.
	Operation string `json:"operation"`
	// Diffs are the differences between the desired and the current entity.
	// +optional
	// +listType=atomic
	Diffs []Diff `json:"diffs,omitempty"`
}

// Diff is one difference between the desired and the current state of a backend entity.
type Diff struct {
	// Path is the JSON path of the difference, such as $.tags or $.config.minute.
	Path string `json:"path"`
	// Desired is the value the operator writes, as JSON. It is empty if the backend has a
	// value the operator removes. Secrets are masked and long values are truncated.
	// +optional
	Desired string `json:"desired,omitempty"`
	// Current is the value the backend holds, as JSON. It is empty if the operator adds a
	// value. Secrets are masked and long values are truncated.
	// +optional
	Current string `json:"current,omitempty"`
	// Ignored is true if the difference is on the ignore list.
	// +optional
	Ignored bool `json:"ignored,omitempty"`
}

// DeepCopyInto copies the receiver into out.
func (in *MigrationStatus) DeepCopyInto(out *MigrationStatus) {
	*out = *in
	out.Changes = copyChanges(in.Changes)
}

// DeepCopy returns a copy of the receiver.
func (in *MigrationStatus) DeepCopy() *MigrationStatus {
	if in == nil {
		return nil
	}
	out := new(MigrationStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto copies the receiver into out.
func (in *AppliedChanges) DeepCopyInto(out *AppliedChanges) {
	*out = *in
	in.AppliedAt.DeepCopyInto(&out.AppliedAt)
	out.Changes = copyChanges(in.Changes)
}

// DeepCopy returns a copy of the receiver.
func (in *AppliedChanges) DeepCopy() *AppliedChanges {
	if in == nil {
		return nil
	}
	out := new(AppliedChanges)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto copies the receiver into out.
func (in *Change) DeepCopyInto(out *Change) {
	*out = *in
	if in.Diffs != nil {
		out.Diffs = make([]Diff, len(in.Diffs))
		copy(out.Diffs, in.Diffs)
	}
}

// DeepCopy returns a copy of the receiver.
func (in *Change) DeepCopy() *Change {
	if in == nil {
		return nil
	}
	out := new(Change)
	in.DeepCopyInto(out)
	return out
}

func copyChanges(in []Change) []Change {
	if in == nil {
		return nil
	}
	out := make([]Change, len(in))
	for i := range in {
		in[i].DeepCopyInto(&out[i])
	}
	return out
}
