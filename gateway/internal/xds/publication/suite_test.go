// SPDX-FileCopyrightText: 2026 Deutsche Telekom AG
// SPDX-License-Identifier: Apache-2.0

package publication

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestPublication(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "xDS Publication Suite")
}
