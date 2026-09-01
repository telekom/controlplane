// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package in

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/telekom/controlplane/rover-server/internal/api"
	"github.com/telekom/controlplane/rover-server/internal/mapper"
)

var (
	fileSpecification = api.FileSpecification{
		Type:        "demo.invoices.v1",
		Version:     "1.0.0",
		Description: "used for dds integration demo",
	}

	resourceIdInfo = mapper.ResourceIdInfo{
		Name:        "demo-invoices-v1",
		Environment: "poc",
		Namespace:   "eni--galatea",
	}
)

func TestFileSpecificationMapper(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "FileSpecification Mapper Suite")
}
