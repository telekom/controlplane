// Copyright 2026 Deutsche Telekom IT GmbH
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

var _ = Describe("FileSpecification Controller", func() {

	Context("Get FileSpecification resource", func() {
		It("should return the FileSpecification successfully", func() {
			req := httptest.NewRequest(http.MethodGet, "/filespecifications/eni--galatea--demo-invoices-v1", nil)
			responseGroup, err := ExecuteRequest(req, groupToken)
			ExpectStatusWithBody(responseGroup, err, http.StatusOK, "application/json")
		})

		It("should fail to get a non-existent FileSpecification", func() {
			req := httptest.NewRequest(http.MethodGet, "/filespecifications/eni--galatea--blabla", nil)
			responseGroup, err := ExecuteRequest(req, groupToken)
			ExpectStatusWithBody(responseGroup, err, http.StatusNotFound, "application/problem+json")
		})

		It("should fail to get a FileSpecification from a different team", func() {
			req := httptest.NewRequest(http.MethodGet, "/filespecifications/other--team--demo-invoices-v1", nil)
			responseGroup, err := ExecuteRequest(req, groupToken)
			ExpectStatusWithBody(responseGroup, err, http.StatusForbidden, "application/problem+json")
		})
	})

	Context("GetAll FileSpecifications resource", func() {
		It("should return all FileSpecifications successfully", func() {
			req := httptest.NewRequest(http.MethodGet, "/filespecifications", nil)
			responseGroup, err := ExecuteRequest(req, groupToken)
			ExpectStatusWithBody(responseGroup, err, http.StatusOK, "application/json")

			responseTeam, err := ExecuteRequest(req, teamToken)
			ExpectStatusWithBody(responseTeam, err, http.StatusOK, "application/json")
		})

		It("should return an empty list if no FileSpecifications exist", func() {
			req := httptest.NewRequest(http.MethodGet, "/filespecifications", nil)
			responseGroup, err := ExecuteRequest(req, groupToken)
			ExpectStatusWithBody(responseGroup, err, http.StatusOK, "application/json")

			responseTeam, err := ExecuteRequest(req, teamToken)
			ExpectStatusWithBody(responseTeam, err, http.StatusOK, "application/json")
		})
	})

	Context("Delete FileSpecification resource", func() {
		It("should delete the FileSpecification successfully", func() {
			req := httptest.NewRequest(http.MethodDelete, "/filespecifications/eni--galatea--demo-invoices-v1", nil)
			responseGroup, err := ExecuteRequest(req, groupToken)
			ExpectStatus(responseGroup, err, http.StatusNoContent, "")
		})

		It("should fail to delete a non-existent FileSpecification", func() {
			req := httptest.NewRequest(http.MethodDelete, "/filespecifications/eni--galatea--blabla", nil)
			responseGroup, err := ExecuteRequest(req, groupToken)
			ExpectStatusWithBody(responseGroup, err, http.StatusNotFound, "application/problem+json")
		})

		It("should fail to delete a FileSpecification from a different team", func() {
			req := httptest.NewRequest(http.MethodDelete, "/filespecifications/other--team--demo-invoices-v1", nil)
			responseGroup, err := ExecuteRequest(req, groupToken)
			ExpectStatusWithBody(responseGroup, err, http.StatusForbidden, "application/problem+json")
		})
	})

	Context("GetStatus FileSpecification resource", func() {
		It("should return the status of the FileSpecification successfully", func() {
			req := httptest.NewRequest(http.MethodGet, "/filespecifications/eni--galatea--demo-invoices-v1/status", nil)
			responseGroup, err := ExecuteRequest(req, groupToken)
			ExpectStatusWithBody(responseGroup, err, http.StatusOK, "application/json")
		})

		It("should fail to get the status of a non-existent FileSpecification", func() {
			req := httptest.NewRequest(http.MethodGet, "/filespecifications/eni--galatea--blabla/status", nil)
			responseGroup, err := ExecuteRequest(req, groupToken)
			ExpectStatusWithBody(responseGroup, err, http.StatusNotFound, "application/problem+json")
		})

		It("should fail to get the status of a FileSpecification from a different team", func() {
			req := httptest.NewRequest(http.MethodGet, "/filespecifications/other--team--demo-invoices-v1/status", nil)
			responseGroup, err := ExecuteRequest(req, groupToken)
			ExpectStatusWithBody(responseGroup, err, http.StatusForbidden, "application/problem+json")
		})
	})

	Context("Create FileSpecification resource", func() {
		It("should return StatusNotImplemented", func() {
			var fileSpecification, _ = json.Marshal(api.FileSpecificationCreateRequest{
				Description: "used for dds integration demo",
				Type:        "demo.invoices.v1",
				Version:     "1.0.0",
			})
			req := httptest.NewRequest(http.MethodPost, "/filespecifications", bytes.NewReader(fileSpecification))
			ExpectStatusNotImplemented(ExecuteRequest(req, groupToken))
			ExpectStatusNotImplemented(ExecuteRequest(req, teamToken))
		})
	})

	Context("Update FileSpecification resource", func() {
		It("should update the FileSpecification successfully", func() {
			var fileSpecification, _ = json.Marshal(api.FileSpecification{
				Description: "used for dds integration demo",
				Type:        "demo.invoices.v1",
				Version:     "1.0.0",
			})

			req := httptest.NewRequest(http.MethodPut, "/filespecifications/eni--galatea--demo-invoices-v1",
				bytes.NewReader(fileSpecification))

			responseGroup, err := ExecuteRequest(req, groupToken)
			ExpectStatusWithBody(responseGroup, err, http.StatusAccepted, "application/json")
		})

		It("should fail to update a FileSpecification from a different team", func() {
			var fileSpecification, _ = json.Marshal(api.FileSpecification{
				Description: "used for dds integration demo",
				Type:        "demo.other.v1",
				Version:     "1.0.0",
			})
			req := httptest.NewRequest(http.MethodPut, "/filespecifications/other--team--demo-other-v1",
				bytes.NewReader(fileSpecification))
			responseGroup, err := ExecuteRequest(req, groupToken)
			ExpectStatusWithBody(responseGroup, err, http.StatusForbidden, "application/problem+json")
		})
	})
})
