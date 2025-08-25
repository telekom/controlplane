// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package handlers

// TEST UTILITIES
// This file contains utility functions for testing purposes.
// These functions should only be used in tests.

// ResetRegistryForTest resets the handlers registry for testing purposes.
// Exported for use in external tests.
func ResetRegistryForTest() {
	mutex.Lock()
	defer mutex.Unlock()

	// Create a new map to clear the registry
	registry = make(map[string]ResourceHandler)
}
