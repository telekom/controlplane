// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"

	"github.com/stretchr/testify/mock"

	fileApi "github.com/telekom/controlplane/file-manager/api"
	"github.com/telekom/controlplane/rover-server/internal/api"

	. "github.com/onsi/ginkgo/v2"
)

// TODO: fix the unit-tests. Use Once() or Twice() for mocks

var _ = Describe("EventSpecification Controller", func() {
	specJson := `{"type":"object","properties":{"id":{"type":"string"}}}`

	Context("Get EventSpecification resource", func() {
		It("should return the EventSpecification successfully", func() {
			mockFileManager.EXPECT().DownloadFile(mock.Anything, "eventRandomId", mock.Anything).
				RunAndReturn(func(_ context.Context, _ string, w io.Writer) (*fileApi.FileDownloadResponse, error) {
					w.Write([]byte(specJson))

					return &fileApi.FileDownloadResponse{
						FileHash:    "randomHash",
						ContentType: "application/json",
					}, nil
				})
			req := httptest.NewRequest(http.MethodGet, "/eventspecifications/eni--hyperion--tardis-horizon-demo-cetus-v1", http.NoBody)
			responseGroup, err := ExecuteRequest(req, groupToken)
			ExpectStatusWithBody(responseGroup, err, http.StatusOK, "application/json")
		})

		It("should fail to get a non-existent EventSpecification", func() {
			req := httptest.NewRequest(http.MethodGet, "/eventspecifications/eni--hyperion--blabla", http.NoBody)
			responseGroup, err := ExecuteRequest(req, groupToken)
			ExpectStatusWithBody(responseGroup, err, http.StatusNotFound, "application/problem+json")
		})

		It("should fail to get an EventSpecification from a different team", func() {
			req := httptest.NewRequest(http.MethodGet, "/eventspecifications/other--team--tardis-horizon-demo-cetus-v1", http.NoBody)
			responseGroup, err := ExecuteRequest(req, groupToken)
			ExpectStatusWithBody(responseGroup, err, http.StatusForbidden, "application/problem+json")
		})
	})

	Context("GetAll EventSpecifications resource", func() {
		It("should return all EventSpecifications successfully", func() {
			mockFileManager.EXPECT().DownloadFile(mock.Anything, "eventRandomId", mock.Anything).
				RunAndReturn(func(_ context.Context, _ string, w io.Writer) (*fileApi.FileDownloadResponse, error) {
					w.Write([]byte(specJson))

					return &fileApi.FileDownloadResponse{
						FileHash:    "randomHash",
						ContentType: "application/json",
					}, nil
				})

			req := httptest.NewRequest(http.MethodGet, "/eventspecifications", http.NoBody)
			responseGroup, err := ExecuteRequest(req, groupToken)
			ExpectStatusWithBody(responseGroup, err, http.StatusOK, "application/json")

			responseTeam, err := ExecuteRequest(req, teamToken)
			ExpectStatusWithBody(responseTeam, err, http.StatusOK, "application/json")
		})

		It("should return an empty list if no EventSpecifications exist", func() {
			req := httptest.NewRequest(http.MethodGet, "/eventspecifications", http.NoBody)
			responseGroup, err := ExecuteRequest(req, groupToken)
			ExpectStatusWithBody(responseGroup, err, http.StatusOK, "application/json")

			responseTeam, err := ExecuteRequest(req, teamToken)
			ExpectStatusWithBody(responseTeam, err, http.StatusOK, "application/json")
		})
	})

	Context("Delete EventSpecification resource", func() {
		It("should delete the EventSpecification successfully", func() {
			mockFileManager.EXPECT().DeleteFile(mock.Anything, mock.Anything).Return(nil)
			req := httptest.NewRequest(http.MethodDelete, "/eventspecifications/eni--hyperion--tardis-horizon-demo-cetus-v1", http.NoBody)
			responseGroup, err := ExecuteRequest(req, groupToken)
			ExpectStatus(responseGroup, err, http.StatusNoContent, "")
		})

		It("should fail to delete a non-existent EventSpecification", func() {
			mockFileManager.EXPECT().DeleteFile(mock.Anything, mock.Anything).Return(nil)
			req := httptest.NewRequest(http.MethodDelete, "/eventspecifications/eni--hyperion--blabla", http.NoBody)
			responseGroup, err := ExecuteRequest(req, groupToken)
			ExpectStatusWithBody(responseGroup, err, http.StatusNotFound, "application/problem+json")
		})

		It("should fail to delete an EventSpecification from a different team", func() {
			req := httptest.NewRequest(http.MethodDelete, "/eventspecifications/other--team--tardis-horizon-demo-cetus-v1", http.NoBody)
			responseGroup, err := ExecuteRequest(req, groupToken)
			ExpectStatusWithBody(responseGroup, err, http.StatusForbidden, "application/problem+json")
		})
	})

	Context("GetStatus EventSpecification resource", func() {
		It("should return the status of the EventSpecification successfully", func() {
			req := httptest.NewRequest(http.MethodGet, "/eventspecifications/eni--hyperion--tardis-horizon-demo-cetus-v1/status", http.NoBody)
			responseGroup, err := ExecuteRequest(req, groupToken)
			ExpectStatusWithBody(responseGroup, err, http.StatusOK, "application/json")
		})

		It("should fail to get the status of a non-existent EventSpecification", func() {
			req := httptest.NewRequest(http.MethodGet, "/eventspecifications/eni--hyperion--blabla/status", http.NoBody)
			responseGroup, err := ExecuteRequest(req, groupToken)
			ExpectStatusWithBody(responseGroup, err, http.StatusNotFound, "application/problem+json")
		})

		It("should fail to get the status of an EventSpecification from a different team", func() {
			req := httptest.NewRequest(http.MethodGet, "/eventspecifications/other--team--tardis-horizon-demo-cetus-v1/status", http.NoBody)
			responseGroup, err := ExecuteRequest(req, groupToken)
			ExpectStatusWithBody(responseGroup, err, http.StatusForbidden, "application/problem+json")
		})
	})

	Context("Create EventSpecification resource", func() {
		It("should return StatusNotImplemented", func() {
			eventSpecification, _ := json.Marshal(api.EventSpecificationCreateRequest{
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
		It("should update the EventSpecification successfully", func() {
			eventSpecification, _ := json.Marshal(api.EventSpecification{
				Category:    "SYSTEM",
				Description: "Horizon demo provider",
				Type:        "tardis.horizon.demo.cetus.v1",
				Version:     "1.0.0",
				Specification: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"id": map[string]interface{}{
							"type": "string",
						},
					},
				},
			})

			mockFileManager.EXPECT().UploadFile(mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(
				&fileApi.FileUploadResponse{
					FileHash:    "randomHash",
					FileId:      "randomId",
					ContentType: "application/json",
				}, nil)

			req := httptest.NewRequest(http.MethodPut, "/eventspecifications/eni--hyperion--tardis-horizon-demo-cetus-v1",
				bytes.NewReader(eventSpecification))

			responseGroup, err := ExecuteRequest(req, groupToken)
			ExpectStatusWithBody(responseGroup, err, http.StatusAccepted, "application/json")
		})

		It("should fail to update an EventSpecification from a different team", func() {
			eventSpecification, _ := json.Marshal(api.EventSpecification{
				Category:    "SYSTEM",
				Description: "Horizon demo provider",
				Type:        "tardis.horizon.demo.other.v1",
				Version:     "1.0.0",
			})
			req := httptest.NewRequest(http.MethodPut, "/eventspecifications/other--team--tardis-horizon-demo-other-v1",
				bytes.NewReader(eventSpecification))
			responseGroup, err := ExecuteRequest(req, groupToken)
			ExpectStatusWithBody(responseGroup, err, http.StatusForbidden, "application/problem+json")
		})
	})
})
