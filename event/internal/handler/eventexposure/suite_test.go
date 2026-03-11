// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package eventexposure_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestEventExposureHandler(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "EventExposure Handler Suite")
}
