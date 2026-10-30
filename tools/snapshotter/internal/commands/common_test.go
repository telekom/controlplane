// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package commands

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/telekom/controlplane/tools/snapshotter/pkg/config"
)

var _ = Describe("resolveStorePath", func() {
	It("prefers the override over the config", func() {
		p, err := resolveStorePath(config.Config{StorePath: "/cfg"}, "/flag", false)
		Expect(err).NotTo(HaveOccurred())
		Expect(p).To(Equal("/flag"))
	})

	It("falls back to the config store path", func() {
		p, err := resolveStorePath(config.Config{StorePath: "/cfg"}, "", false)
		Expect(err).NotTo(HaveOccurred())
		Expect(p).To(Equal("/cfg"))
	})

	It("returns an error when no path is configured and storing is needed", func() {
		_, err := resolveStorePath(config.Config{}, "", false)
		Expect(err).To(MatchError("no store path configured: set storePath in the config file or pass --store"))
	})

	It("returns no error when no path is configured and storing is disabled", func() {
		p, err := resolveStorePath(config.Config{}, "", true)
		Expect(err).NotTo(HaveOccurred())
		Expect(p).To(BeEmpty())
	})
})

var _ = Describe("validateSnapLimit", func() {
	It("accepts the default limit", func() {
		Expect(validateSnapLimit(100, false, "", "")).To(Succeed())
	})

	It("accepts the default limit with a route or consumer", func() {
		Expect(validateSnapLimit(100, false, "r", "")).To(Succeed())
		Expect(validateSnapLimit(100, false, "", "c")).To(Succeed())
	})

	It("accepts an explicit limit 0 for global snapshots", func() {
		Expect(validateSnapLimit(0, true, "", "")).To(Succeed())
	})

	It("rejects a negative limit", func() {
		Expect(validateSnapLimit(-1, true, "", "")).To(MatchError("--limit must be >= 0"))
	})

	It("rejects an explicit limit together with --route or --consumer", func() {
		Expect(validateSnapLimit(10, true, "r", "")).To(MatchError("--limit is only valid without --route/--consumer"))
		Expect(validateSnapLimit(10, true, "", "c")).To(MatchError("--limit is only valid without --route/--consumer"))
	})
})
