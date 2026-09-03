// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package out

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	roverv1 "github.com/telekom/controlplane/rover/api/v1"

	"github.com/telekom/controlplane/rover-server/test/mocks"
)

var fileSpecification *roverv1.FileSpecification

func TestMapper(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "FileSpecification Out Mapper Suite")
}

var _ = BeforeSuite(func() {
	fileSpecification = mocks.GetFileSpecification(GinkgoT(), mocks.FileSpecificationFileName)
})
