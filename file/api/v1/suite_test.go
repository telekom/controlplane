// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v1_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestFileApiV1(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "File API V1 Suite")
}
