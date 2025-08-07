// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"

	"github.com/gkampitakis/go-snaps/match"
	. "github.com/onsi/ginkgo/v2"

	"github.com/telekom/controlplane/rover-server/internal/api"
)

var _ = Describe("ApiSpecification Controller", func() {
	Context("Get ApiSpecification resource", func() {
		It("should return the ApiSpecification successfully", func() {
			req := httptest.NewRequest(http.MethodGet, "/apispecifications/eni--hyperion--apispec-sample", nil)
			responseGroup, err := ExecuteRequest(req, groupToken)
			ExpectStatusWithBody(responseGroup, err, http.StatusOK, "application/json", match.Any("status.time"))
		})

		It("should fail to get a non-existent ApiSpecification", func() {
			req := httptest.NewRequest(http.MethodGet, "/apispecifications/eni--hyperion--blabla", nil)
			responseGroup, err := ExecuteRequest(req, groupToken)
			ExpectStatusWithBody(responseGroup, err, http.StatusNotFound, "application/problem+json")
		})

		It("should fail to get an ApiSpecification from a different team", func() {
			req := httptest.NewRequest(http.MethodGet, "/apispecifications/other--team--apispec-sample", nil)
			responseGroup, err := ExecuteRequest(req, groupToken)
			ExpectStatusWithBody(responseGroup, err, http.StatusForbidden, "application/problem+json")
		})
	})

	Context("GetAll ApiSpecifications resource", func() {
		It("should return all ApiSpecifications successfully", func() {
			req := httptest.NewRequest(http.MethodGet, "/apispecifications", nil)
			responseGroup, err := ExecuteRequest(req, groupToken)
			ExpectStatusWithBody(responseGroup, err, http.StatusOK, "application/json", match.Any("items.0.status.time"))

			responseTeam, err := ExecuteRequest(req, teamToken)
			ExpectStatusWithBody(responseTeam, err, http.StatusOK, "application/json", match.Any("items.0.status.time"))
		})

		It("should return an empty list if no ApiSpecifications exist", func() {
			req := httptest.NewRequest(http.MethodGet, "/apispecifications", nil)
			responseGroup, err := ExecuteRequest(req, groupToken)
			ExpectStatusWithBody(responseGroup, err, http.StatusOK, "application/json", match.Any("items.0.status.time"))

			responseTeam, err := ExecuteRequest(req, teamToken)
			ExpectStatusWithBody(responseTeam, err, http.StatusOK, "application/json", match.Any("items.0.status.time"))
		})
	})

	Context("Delete ApiSpecification resource", func() {
		It("should delete the ApiSpecification successfully", func() {
			req := httptest.NewRequest(http.MethodDelete, "/apispecifications/eni--hyperion--apispec-sample", nil)
			responseGroup, err := ExecuteRequest(req, groupToken)
			ExpectStatus(responseGroup, err, http.StatusNoContent, "")
		})

		It("should fail to delete a non-existent ApiSpecification", func() {
			req := httptest.NewRequest(http.MethodDelete, "/apispecifications/eni--hyperion--blabla", nil)
			responseGroup, err := ExecuteRequest(req, groupToken)
			ExpectStatusWithBody(responseGroup, err, http.StatusNotFound, "application/problem+json")
		})

		It("should fail to delete an ApiSpecification from a different team", func() {
			req := httptest.NewRequest(http.MethodDelete, "/apispecifications/other--team--apispec-sample", nil)
			responseGroup, err := ExecuteRequest(req, groupToken)
			ExpectStatusWithBody(responseGroup, err, http.StatusForbidden, "application/problem+json")
		})
	})

	Context("GetStatus ApiSpecification resource", func() {
		It("should return the status of the ApiSpecification successfully", func() {
			req := httptest.NewRequest(http.MethodGet, "/apispecifications/eni--hyperion--apispec-sample/status", nil)
			responseGroup, err := ExecuteRequest(req, groupToken)
			ExpectStatusWithBody(responseGroup, err, http.StatusOK, "application/json")
		})

		It("should fail to get the status of a non-existent ApiSpecification", func() {
			req := httptest.NewRequest(http.MethodGet, "/apispecifications/eni--hyperion--blabla/status", nil)
			responseGroup, err := ExecuteRequest(req, groupToken)
			ExpectStatusWithBody(responseGroup, err, http.StatusNotFound, "application/problem+json")
		})

		It("should fail to get the status of an ApiSpecification from a different team", func() {
			req := httptest.NewRequest(http.MethodGet, "/apispecifications/other--team--apispec-sample/status", nil)
			responseGroup, err := ExecuteRequest(req, groupToken)
			ExpectStatusWithBody(responseGroup, err, http.StatusForbidden, "application/problem+json")
		})
	})

	Context("Create ApiSpecification resource", func() {
		It("should return StatusNotImplemented", func() {
			// Create a request with a JSON body
			var apiSpecification, _ = json.Marshal(api.ApiSpecificationCreateRequest{
				Specification: map[string]interface{}{},
			})
			req := httptest.NewRequest(http.MethodPost, "/apispecifications", bytes.NewReader(apiSpecification))
			ExpectStatusNotImplemented(ExecuteRequest(req, groupToken))
			ExpectStatusNotImplemented(ExecuteRequest(req, teamToken))
		})
	})

	Context("Update ApiSpecification resource", func() {
		It("should update the ApiSpecification successfully", func() {
			var apiSpecification, _ = json.Marshal(api.ApiSpecificationCreateRequest{
				Specification: map[string]interface{}{
					"info": map[string]interface{}{
						"title":   "Rover API",
						"version": "1.0.0",
					},
					"servers": map[string]interface{}{
						"url": "http://rover-api/eni/distr/v1",
					},
				},
			})
			req := httptest.NewRequest(http.MethodPut, "/apispecifications/eni--hyperion--apispec-sample",
				bytes.NewReader(apiSpecification))
			responseGroup, err := ExecuteRequest(req, groupToken)
			ExpectStatusWithBody(responseGroup, err, http.StatusAccepted, "application/json", match.Any("status.time"))
		})

		It("should fail to update a non-existent ApiSpecification", func() {
			var apiSpecification, _ = json.Marshal(api.ApiSpecificationCreateRequest{
				Specification: map[string]interface{}{
					"info": map[string]interface{}{
						"title":   "Rover API",
						"version": "1.0.0",
					},
					"servers": map[string]interface{}{
						"url": "http://rover-api/eni/distr/v1",
					},
				},
			})
			req := httptest.NewRequest(http.MethodPut, "/apispecifications/eni--hyperion--blabla",
				bytes.NewReader(apiSpecification))
			responseGroup, err := ExecuteRequest(req, groupToken)
			ExpectStatusWithBody(responseGroup, err, http.StatusNotFound, "application/problem+json")
		})

		It("should fail to update an ApiSpecification from a different team", func() {
			var apiSpecification, _ = json.Marshal(api.ApiSpecificationCreateRequest{
				Specification: map[string]interface{}{
					"info": map[string]interface{}{
						"title":   "Rover API",
						"version": "1.0.0",
					},
					"servers": map[string]interface{}{
						"url": "http://rover-api/eni/distr/v1",
					},
				},
			})
			req := httptest.NewRequest(http.MethodPut, "/apispecifications/other--team--apispec-sample",
				bytes.NewReader(apiSpecification))
			responseGroup, err := ExecuteRequest(req, groupToken)
			ExpectStatusWithBody(responseGroup, err, http.StatusForbidden, "application/problem+json")
		})
	})
})
