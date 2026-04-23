// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package in

import (
	"testing"

	"github.com/telekom/controlplane/rover-server/internal/api"
	"github.com/telekom/controlplane/rover-server/internal/mapper"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var (
	eventSpecification = api.EventSpecification{
		Type:        "tardis.horizon.demo.cetus.v1",
		Version:     "1.0.0",
		Description: "Horizon demo provider",
		Specification: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"id": map[string]interface{}{
					"type": "string",
				},
			},
		},
	}

	resourceIdInfo = mapper.ResourceIdInfo{
		Name:        "tardis-horizon-demo-cetus-v1",
		Environment: "poc",
		Namespace:   "eni--hyperion",
	}
)

func TestEventSpecificationMapper(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "EventSpecification Mapper Suite")
}
