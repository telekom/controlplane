// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package kgateway

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestKGateway(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "KGateway Feature Builder Suite")
}
