// SPDX-FileCopyrightText: 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"testing"

	ctypes "github.com/telekom/controlplane/common/pkg/types"
)

// The janitor client casts every registered List type to types.ObjectList in
// cleanupStateUnstructured. A List type that does not implement it makes
// CleanupAll return "object is not a valid list" and abort, which skips cleanup
// of every other owned type in that reconcile. rover registers both of these
// types whenever the spectre feature gate is on, so both must implement it.
func TestListTypesImplementObjectList(t *testing.T) {
	t.Run("SpectreApplicationList", func(t *testing.T) {
		var obj any = &SpectreApplicationList{}
		list, ok := obj.(ctypes.ObjectList)
		if !ok {
			t.Fatal("SpectreApplicationList does not implement types.ObjectList")
		}
		if got := len(list.GetItems()); got != 0 {
			t.Errorf("empty list GetItems() = %d, want 0", got)
		}
	})

	t.Run("ListenerList", func(t *testing.T) {
		var obj any = &ListenerList{}
		list, ok := obj.(ctypes.ObjectList)
		if !ok {
			t.Fatal("ListenerList does not implement types.ObjectList")
		}
		if got := len(list.GetItems()); got != 0 {
			t.Errorf("empty list GetItems() = %d, want 0", got)
		}
	})

	t.Run("GetItems returns each item", func(t *testing.T) {
		sl := &SpectreApplicationList{Items: []SpectreApplication{{}, {}}}
		if got := len(ctypes.ObjectList(sl).GetItems()); got != 2 {
			t.Errorf("SpectreApplicationList GetItems() = %d, want 2", got)
		}

		ll := &ListenerList{Items: []Listener{{}, {}, {}}}
		if got := len(ctypes.ObjectList(ll).GetItems()); got != 3 {
			t.Errorf("ListenerList GetItems() = %d, want 3", got)
		}
	})
}
