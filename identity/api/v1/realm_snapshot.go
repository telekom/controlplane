// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v1

// RealmSnapshotState classifies the persisted Realm connection (cleanup) snapshot.
type RealmSnapshotState int

const (
	// RealmSnapshotEmpty means no connection was ever committed: no field and no trust reference.
	RealmSnapshotEmpty RealmSnapshotState = iota
	// RealmSnapshotComplete is a committed cleanup obligation: all six connection fields are set.
	RealmSnapshotComplete
	// RealmSnapshotPartial is an ambiguous snapshot requiring operator repair.
	RealmSnapshotPartial
)

// ClassifyRealmSnapshot reports whether the Realm connection status is empty, complete or partial.
// It is shared by the Identity operator and Admin so both agree on cleanup obligations.
func ClassifyRealmSnapshot(s *RealmStatus) RealmSnapshotState {
	fields := []string{s.IssuerUrl, s.AdminClientId, s.AdminUserName, s.AdminPassword, s.AdminUrl, s.AdminTokenUrl}
	set := 0
	for _, f := range fields {
		if f != "" {
			set++
		}
	}
	switch {
	case set == len(fields):
		return RealmSnapshotComplete
	case set == 0 && s.TLS.Ref() == nil:
		return RealmSnapshotEmpty
	default:
		return RealmSnapshotPartial
	}
}
