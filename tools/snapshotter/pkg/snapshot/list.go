// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package snapshot

type SnapshotList struct {
	Items []*Snapshot `json:"items"`
}

func (l *SnapshotList) Add(snap *Snapshot) {
	l.Items = append(l.Items, snap)
}

func (l *SnapshotList) All() []*Snapshot {
	return l.Items
}

func (l *SnapshotList) New() *Snapshot {
	return &Snapshot{}
}
