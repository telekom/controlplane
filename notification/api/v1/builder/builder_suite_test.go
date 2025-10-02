// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package builder_test

import (
	"testing"

	fakeclient "github.com/telekom/controlplane/common/pkg/client/fake"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var (
	globalClient *fakeclient.MockJanitorClient
)

func TestBuilder(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Builder Suite")
}

var _ = BeforeSuite(func() {
	globalClient = fakeclient.NewMockJanitorClient(GinkgoT())
})
