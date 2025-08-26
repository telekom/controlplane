// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v0_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestV0Handlers(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "V0 Handlers Suite")
}
