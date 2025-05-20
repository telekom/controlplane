// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package fsm_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestFsm(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Fsm Suite")
}
