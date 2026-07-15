// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package mcpsubscription_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestMcpSubscriptionHandler(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "McpSubscription Handler Suite")
}
