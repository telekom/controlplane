// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package util

import (
	"github.com/telekom/controlplane/common/pkg/types"
)

// AppendUniqueRef appends ref to refs unless an entry with the same Namespace
// and Name is already present. Existing entries keep their order; a new ref is
// appended at the end.
//
// Notification names are deterministic, so a reconcile that re-sends the same
// notification must not grow the ref list.
func AppendUniqueRef(refs []types.ObjectRef, ref types.ObjectRef) []types.ObjectRef {
	for _, existing := range refs {
		if existing.Namespace == ref.Namespace && existing.Name == ref.Name {
			return refs
		}
	}
	return append(refs, ref)
}

// DedupRefs returns refs with duplicate Namespace/Name pairs removed, keeping the
// first occurrence and preserving order. The input is returned unchanged when it
// contains no duplicates. nil in, nil out.
//
// status.notificationRefs is a map-typed list keyed on namespace and name, so
// objects persisted before that schema change must be normalised before the next
// status write.
func DedupRefs(refs []types.ObjectRef) []types.ObjectRef {
	type key struct {
		namespace string
		name      string
	}

	seen := make(map[key]struct{}, len(refs))
	for i, ref := range refs {
		k := key{ref.Namespace, ref.Name}
		if _, ok := seen[k]; !ok {
			seen[k] = struct{}{}
			continue
		}

		// First duplicate found: copy the unique prefix and filter the rest.
		deduped := make([]types.ObjectRef, i, len(refs)-1)
		copy(deduped, refs[:i])
		for _, rest := range refs[i+1:] {
			k := key{rest.Namespace, rest.Name}
			if _, ok := seen[k]; ok {
				continue
			}
			seen[k] = struct{}{}
			deduped = append(deduped, rest)
		}
		return deduped
	}

	return refs
}
