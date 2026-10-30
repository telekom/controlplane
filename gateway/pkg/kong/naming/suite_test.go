// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package naming_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestNaming(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Naming Suite")
}
