// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package envoy

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestEnvoy(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Envoy Feature Builder Suite")
}
