// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package log

import (
	"go.uber.org/zap/zapcore"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("LevelFromValue", func() {

	DescribeTable("recognized level names",
		func(value string, expected zapcore.Level) {
			atomicLevel := LevelFromValue(value)
			Expect(atomicLevel.Level()).To(Equal(expected))
		},
		Entry("debug", "debug", zapcore.DebugLevel),
		Entry("uppercase DEBUG", "DEBUG", zapcore.DebugLevel),
		Entry("info", "info", zapcore.InfoLevel),
		Entry("warn", "warn", zapcore.WarnLevel),
		Entry("error", "error", zapcore.ErrorLevel),
	)

	It("defaults to Info when the value is empty", func() {
		Expect(LevelFromValue("").Level()).To(Equal(zapcore.InfoLevel))
	})

	It("defaults to Info when the value is unrecognized", func() {
		Expect(LevelFromValue("not-a-real-level").Level()).To(Equal(zapcore.InfoLevel))
	})

	It("defaults to Info when the value is only whitespace", func() {
		Expect(LevelFromValue("   ").Level()).To(Equal(zapcore.InfoLevel))
	})
})

var _ = Describe("LevelFromEnv", func() {

	It("defaults to Info when LOG_LEVEL is unset", func() {
		GinkgoT().Setenv(LevelEnvVar, "")
		Expect(LevelFromEnv().Level()).To(Equal(zapcore.InfoLevel))
	})

	It("uses the value of LOG_LEVEL when set to a recognized level", func() {
		GinkgoT().Setenv(LevelEnvVar, "debug")
		Expect(LevelFromEnv().Level()).To(Equal(zapcore.DebugLevel))
	})

	It("defaults to Info when LOG_LEVEL is set to an invalid value", func() {
		GinkgoT().Setenv(LevelEnvVar, "verbose")
		Expect(LevelFromEnv().Level()).To(Equal(zapcore.InfoLevel))
	})
})
