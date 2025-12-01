// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package rover_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const (
	testEnvironment = "test-handler"
	teamId          = "eni--hyperion"
	teamNamespace   = testEnvironment + "--" + teamId
)

func TestRover(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Rover Handler Suite")
}
