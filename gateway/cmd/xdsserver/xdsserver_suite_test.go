// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package xdsserver

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestXDSServer(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "xDS Server Suite")
}
