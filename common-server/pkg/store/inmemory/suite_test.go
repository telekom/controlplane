// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package inmemory_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestInmemory(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Inmemory Store Suite")
}
