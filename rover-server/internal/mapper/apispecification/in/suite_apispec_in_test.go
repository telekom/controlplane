// Copyright 2025 Deutsche Telekom IT GmbH
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
	apiSpecification = &api.ApiSpecificationUpdateRequest{
		Specification: map[string]interface{}{
			"openapi": "3.0.0",
			"info": map[string]interface{}{
				"title":   "Rover API",
				"version": "1.0.0",
			},
			"servers": []interface{}{
				map[string]interface{}{
					"url": "http://rover-api/eni/distr/v1",
				},
			},
		},
	}

	resourceIdInfo = mapper.ResourceIdInfo{
		Name:        "rover-local-sub",
		Environment: "poc",
		Namespace:   "poc--eni--hyperion",
	}
)

func TestApiSpecificationMapper(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "ApiSpecification Mapper Suite")
}
