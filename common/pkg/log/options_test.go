// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package log

import (
	"go.uber.org/zap/zapcore"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("DefaultOptions", func() {

	It("defaults to Development=false", func() {
		Expect(DefaultOptions().Development).To(BeFalse())
	})

	It("defaults the log level to Info when LOG_LEVEL is unset", func() {
		GinkgoT().Setenv(LevelEnvVar, "")
		Expect(DefaultOptions().Level.Enabled(zapcore.InfoLevel)).To(BeTrue())
		Expect(DefaultOptions().Level.Enabled(zapcore.DebugLevel)).To(BeFalse())
	})

	It("honors LOG_LEVEL when set to a recognized level", func() {
		GinkgoT().Setenv(LevelEnvVar, "debug")
		Expect(DefaultOptions().Level.Enabled(zapcore.DebugLevel)).To(BeTrue())
	})

	It("returns a struct that callers can override", func() {
		opts := DefaultOptions()
		opts.Development = true
		Expect(opts.Development).To(BeTrue())
	})
})
