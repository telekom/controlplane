// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package apiexposure

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestApiExposureHandler(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "ApiExposure Handler Suite")
}
