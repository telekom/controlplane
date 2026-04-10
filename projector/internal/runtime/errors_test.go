// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package runtime_test

import (
	"errors"
	"fmt"

	"github.com/telekom/controlplane/projector/internal/runtime"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Error sentinels", func() {

	Describe("ErrSkipSync", func() {
		It("is detectable via errors.Is", func() {
			Expect(errors.Is(runtime.ErrSkipSync, runtime.ErrSkipSync)).To(BeTrue())
		})

		It("is detectable when wrapped", func() {
			wrapped := fmt.Errorf("missing field: %w", runtime.ErrSkipSync)
			Expect(errors.Is(wrapped, runtime.ErrSkipSync)).To(BeTrue())
		})

		It("is detectable via IsSkipSync helper", func() {
			wrapped := fmt.Errorf("reason: %w", runtime.ErrSkipSync)
			Expect(runtime.IsSkipSync(wrapped)).To(BeTrue())
		})

		It("does not match other sentinels", func() {
			Expect(runtime.IsSkipSync(runtime.ErrDependencyMissing)).To(BeFalse())
			Expect(runtime.IsSkipSync(runtime.ErrDeleteKeyLost)).To(BeFalse())
		})
	})

	Describe("ErrDependencyMissing", func() {
		It("is detectable via errors.Is", func() {
			Expect(errors.Is(runtime.ErrDependencyMissing, runtime.ErrDependencyMissing)).To(BeTrue())
		})

		It("is detectable when wrapped", func() {
			wrapped := fmt.Errorf("team lookup: %w", runtime.ErrDependencyMissing)
			Expect(errors.Is(wrapped, runtime.ErrDependencyMissing)).To(BeTrue())
		})

		It("is detectable via IsDependencyMissing helper", func() {
			wrapped := fmt.Errorf("zone: %w", runtime.ErrDependencyMissing)
			Expect(runtime.IsDependencyMissing(wrapped)).To(BeTrue())
		})

		It("does not match other sentinels", func() {
			Expect(runtime.IsDependencyMissing(runtime.ErrSkipSync)).To(BeFalse())
			Expect(runtime.IsDependencyMissing(runtime.ErrDeleteKeyLost)).To(BeFalse())
		})
	})

	Describe("ErrDeleteKeyLost", func() {
		It("is detectable via errors.Is", func() {
			Expect(errors.Is(runtime.ErrDeleteKeyLost, runtime.ErrDeleteKeyLost)).To(BeTrue())
		})

		It("is detectable when wrapped", func() {
			wrapped := fmt.Errorf("application: %w", runtime.ErrDeleteKeyLost)
			Expect(errors.Is(wrapped, runtime.ErrDeleteKeyLost)).To(BeTrue())
		})

		It("is detectable via IsDeleteKeyLost helper", func() {
			wrapped := fmt.Errorf("no cache: %w", runtime.ErrDeleteKeyLost)
			Expect(runtime.IsDeleteKeyLost(wrapped)).To(BeTrue())
		})

		It("does not match other sentinels", func() {
			Expect(runtime.IsDeleteKeyLost(runtime.ErrSkipSync)).To(BeFalse())
			Expect(runtime.IsDeleteKeyLost(runtime.ErrDependencyMissing)).To(BeFalse())
		})
	})

	Describe("WrapDependencyMissing", func() {
		It("wraps ErrDependencyMissing with entity context", func() {
			err := runtime.WrapDependencyMissing("Team", "my-team")
			Expect(err).To(MatchError(ContainSubstring("Team")))
			Expect(err).To(MatchError(ContainSubstring("my-team")))
			Expect(errors.Is(err, runtime.ErrDependencyMissing)).To(BeTrue())
		})

		It("is detectable via IsDependencyMissing", func() {
			err := runtime.WrapDependencyMissing("Zone", "zone-a")
			Expect(runtime.IsDependencyMissing(err)).To(BeTrue())
		})

		It("preserves the full error chain when double-wrapped", func() {
			inner := runtime.WrapDependencyMissing("Zone", "zone-a")
			outer := fmt.Errorf("upsert failed: %w", inner)
			Expect(runtime.IsDependencyMissing(outer)).To(BeTrue())
		})
	})

	Describe("nil error handling", func() {
		It("returns false for nil errors", func() {
			Expect(runtime.IsSkipSync(nil)).To(BeFalse())
			Expect(runtime.IsDependencyMissing(nil)).To(BeFalse())
			Expect(runtime.IsDeleteKeyLost(nil)).To(BeFalse())
		})
	})
})
