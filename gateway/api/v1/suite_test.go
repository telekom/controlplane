// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v1_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestGatewayAPIv1(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Gateway API v1 Suite")
}
