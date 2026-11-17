// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package in

import (
	"encoding/json"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/telekom/controlplane/rover-server/internal/api"
	"github.com/telekom/controlplane/rover-server/internal/mapper"
)

var (
	apiSpecification = &api.ApiSpecificationUpdateRequest{
		Specification: json.RawMessage(`{
			"openapi": "3.0.0",
			"info": {"title": "Rover API", "version": "1.0.0"},
			"servers": [{"url": "http://rover-api.com/eni/distr/v1"}]
		}`),
	}

	resourceIdInfo = mapper.ResourceIdInfo{
		Name:        "eni-distr-v1",
		Environment: "poc",
		Namespace:   "poc--eni--hyperion",
	}
)

func TestApiSpecificationMapper(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "ApiSpecification Mapper Suite")
}
