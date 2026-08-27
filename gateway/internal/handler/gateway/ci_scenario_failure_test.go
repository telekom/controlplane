// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package gateway_test

// This file exists only to validate the CI redesign's failure-isolation
// behavior: gateway's build should fail while unrelated in-scope modules
// (e.g. permission, changed in the same PR but sharing no dependency)
// still build/package successfully. Not meant to be merged.

import "testing"

func TestCIScenarioDeliberateFailure(t *testing.T) {
	t.Fatal("intentional failure for CI redesign scenario testing - do not merge")
}
