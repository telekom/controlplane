// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"

	"github.com/telekom/controlplane/rover-server/internal/api"
)

var _ = Describe("EventSpecification Controller", func() {
	Context("GetAll EventSpecifications", func() {
		It("should return StatusNotImplemented", func() {
			req := httptest.NewRequest(http.MethodGet, "/eventspecifications", nil)
			ExpectStatusNotImplemented(ExecuteRequest(req, groupToken))
			ExpectStatusNotImplemented(ExecuteRequest(req, teamToken))
		})
	})
	Context("Get EventSpecification resource", func() {
		It("should return StatusNotImplemented", func() {
			req := httptest.NewRequest(http.MethodGet, "/eventspecifications/eni--hyperion--horizon-local-sub", nil)
			ExpectStatusNotImplemented(ExecuteRequest(req, groupToken))
		})
	})
	Context("GetStatus EventSpecification resource", func() {
		It("should return StatusNotImplemented", func() {
			req := httptest.NewRequest(http.MethodGet, "/eventspecifications/eni--hyperion--horizon-local-sub/status", nil)
			ExpectStatusNotImplemented(ExecuteRequest(req, groupToken))
		})
	})
	Context("Delete EventSpecification resource", func() {
		It("should return StatusNotImplemented", func() {
			req := httptest.NewRequest(http.MethodDelete, "/eventspecifications/eni--hyperion--horizon-local-sub", nil)
			ExpectStatusNotImplemented(ExecuteRequest(req, groupToken))
		})
	})
	Context("Create EventSpecification resource", func() {
		It("should return StatusNotImplemented", func() {
			// Create a request with a JSON body
			var eventSpecification, _ = json.Marshal(api.EventSpecificationCreateRequest{
				Category:    "SYSTEM",
				Description: "Horizon demo provider",
				Type:        "tardis.horizon.demo.cetus.v1",
				Version:     "1.0.0",
			})
			req := httptest.NewRequest(http.MethodPost, "/eventspecifications", bytes.NewReader(eventSpecification))
			ExpectStatusNotImplemented(ExecuteRequest(req, groupToken))
			ExpectStatusNotImplemented(ExecuteRequest(req, teamToken))
		})
	})
	Context("Update EventSpecification resource", func() {
		It("should return StatusNotImplemented", func() {
			// Create a request with a JSON body
			var eventSpecification, _ = json.Marshal(api.EventSpecification{
				Category:    "SYSTEM",
				Description: "Horizon demo provider",
				Type:        "tardis.horizon.demo.cetus.v1",
				Version:     "1.0.0",
			})
			req := httptest.NewRequest(http.MethodPut, "/eventspecifications/eni--hyperion--horizon-local-sub", bytes.NewReader(eventSpecification))
			ExpectStatusNotImplemented(ExecuteRequest(req, groupToken))
		})
	})
})
