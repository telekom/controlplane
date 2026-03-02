// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v2_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestCacheV2(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Cache V2 Suite")
}
