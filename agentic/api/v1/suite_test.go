// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestAgenticTypes(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Agentic API Suite")
}
