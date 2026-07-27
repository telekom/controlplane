// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package ai_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestAiHandler(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "AI Handler Suite")
}
