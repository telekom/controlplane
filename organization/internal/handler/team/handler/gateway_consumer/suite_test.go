// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package gateway_consumer

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestGatewayConsumer(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Gateway Consumer Handler Suite")
}
