// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package instance

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestInstanceHandler(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Instance Handler Suite")
}
