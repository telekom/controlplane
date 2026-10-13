// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package in_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestAgentSpecificationIn(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "AgentSpecification In Mapper Suite")
}
