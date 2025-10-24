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

	. "github.com/onsi/ginkgo/v2"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/mock"
	fileApi "github.com/telekom/controlplane/file-manager/api"
	"github.com/telekom/controlplane/rover-server/internal/api"
)

var _ = Describe("ApiSpecification Controller", func() {

	specV3 := `
openapi: "3.0.0"
info:
  version: "1.0.0"
  title: "Rover API"
servers: 
  - url: "http://rover-api.com/eni/distr/v1"
`

	Context("Get ApiSpecification resource", func() {
		It("should return the ApiSpecification successfully", func() {
			mockFileManager.EXPECT().DownloadFile(mock.Anything, mock.Anything, mock.Anything).
				RunAndReturn(func(_ context.Context, _ string, w io.Writer) (*fileApi.FileDownloadResponse, error) {

					w.Write([]byte(specV3))

					return &fileApi.FileDownloadResponse{
						FileHash:    "randomHash",
						ContentType: "application/yaml",
					}, nil
				})
			req := httptest.NewRequest(http.MethodGet, "/apispecifications/eni--hyperion--eni-distr-v1", nil)
			responseGroup, err := ExecuteRequest(req, groupToken)
			ExpectStatusWithBody(responseGroup, err, http.StatusOK, "application/json")
		})

		It("should fail to get a non-existent ApiSpecification", func() {
			req := httptest.NewRequest(http.MethodGet, "/apispecifications/eni--hyperion--blabla", nil)
			responseGroup, err := ExecuteRequest(req, groupToken)
			ExpectStatusWithBody(responseGroup, err, http.StatusNotFound, "application/problem+json")
		})

		It("should fail to get an ApiSpecification from a different team", func() {
			req := httptest.NewRequest(http.MethodGet, "/apispecifications/other--team--eni-distr-v1", nil)
			responseGroup, err := ExecuteRequest(req, groupToken)
			ExpectStatusWithBody(responseGroup, err, http.StatusForbidden, "application/problem+json")
		})
	})

	Context("GetAll ApiSpecifications resource", func() {
		It("should return all ApiSpecifications successfully", func() {
			mockFileManager.EXPECT().DownloadFile(mock.Anything, mock.Anything, mock.Anything).
				RunAndReturn(func(_ context.Context, _ string, w io.Writer) (*fileApi.FileDownloadResponse, error) {

					w.Write([]byte(specV3))

					return &fileApi.FileDownloadResponse{
						FileHash:    "randomHash",
						ContentType: "application/yaml",
					}, nil
				})

			req := httptest.NewRequest(http.MethodGet, "/apispecifications", nil)
			responseGroup, err := ExecuteRequest(req, groupToken)
			ExpectStatusWithBody(responseGroup, err, http.StatusOK, "application/json")

			responseTeam, err := ExecuteRequest(req, teamToken)
			ExpectStatusWithBody(responseTeam, err, http.StatusOK, "application/json")
		})

		It("should return an empty list if no ApiSpecifications exist", func() {
			req := httptest.NewRequest(http.MethodGet, "/apispecifications", nil)
			responseGroup, err := ExecuteRequest(req, groupToken)
			ExpectStatusWithBody(responseGroup, err, http.StatusOK, "application/json")

			responseTeam, err := ExecuteRequest(req, teamToken)
			ExpectStatusWithBody(responseTeam, err, http.StatusOK, "application/json")
		})
	})

	Context("Delete ApiSpecification resource", func() {
		It("should delete the ApiSpecification successfully", func() {
			mockFileManager.EXPECT().DeleteFile(mock.Anything, mock.Anything).Return(nil)
			req := httptest.NewRequest(http.MethodDelete, "/apispecifications/eni--hyperion--eni-distr-v1", nil)
			responseGroup, err := ExecuteRequest(req, groupToken)
			ExpectStatus(responseGroup, err, http.StatusNoContent, "")
		})

		It("should fail to delete a non-existent ApiSpecification", func() {
			mockFileManager.EXPECT().DeleteFile(mock.Anything, mock.Anything).Return(errors.Errorf("resource not found"))
			req := httptest.NewRequest(http.MethodDelete, "/apispecifications/eni--hyperion--blabla", nil)
			responseGroup, err := ExecuteRequest(req, groupToken)
			ExpectStatusWithBody(responseGroup, err, http.StatusNotFound, "application/problem+json")
		})

		It("should fail to delete an ApiSpecification from a different team", func() {
			req := httptest.NewRequest(http.MethodDelete, "/apispecifications/other--team--eni-distr-v1", nil)
			responseGroup, err := ExecuteRequest(req, groupToken)
			ExpectStatusWithBody(responseGroup, err, http.StatusForbidden, "application/problem+json")
		})
	})

	Context("GetStatus ApiSpecification resource", func() {
		It("should return the status of the ApiSpecification successfully", func() {
			req := httptest.NewRequest(http.MethodGet, "/apispecifications/eni--hyperion--eni-distr-v1/status", nil)
			responseGroup, err := ExecuteRequest(req, groupToken)
			ExpectStatusWithBody(responseGroup, err, http.StatusOK, "application/json")
		})

		It("should fail to get the status of a non-existent ApiSpecification", func() {
			req := httptest.NewRequest(http.MethodGet, "/apispecifications/eni--hyperion--blabla/status", nil)
			responseGroup, err := ExecuteRequest(req, groupToken)
			ExpectStatusWithBody(responseGroup, err, http.StatusNotFound, "application/problem+json")
		})

		It("should fail to get the status of an ApiSpecification from a different team", func() {
			req := httptest.NewRequest(http.MethodGet, "/apispecifications/other--team--eni-distr-v1/status", nil)
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
					"openapi": "3.0.0",
					"info": map[string]interface{}{
						"title":          "Rover API",
						"version":        "1.0.0",
						"x-api-category": "other",
						"x-vendor":       "true",
					},
					"servers": []map[string]interface{}{
						{
							"url": "http://rover-api.com/eni/distr/v1",
						},
					},
				},
			})

			mockFileManager.EXPECT().UploadFile(mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(
				&fileApi.FileUploadResponse{
					FileHash:    "randomHash",
					FileId:      "randomId",
					ContentType: "application/yaml",
				}, nil)

			req := httptest.NewRequest(http.MethodPut, "/apispecifications/eni--hyperion--eni-distr-v1",
				bytes.NewReader(apiSpecification))
			responseGroup, err := ExecuteRequest(req, groupToken)
			ExpectStatusWithBody(responseGroup, err, http.StatusAccepted, "application/json")
		})

		It("should fail to update a non-existent ApiSpecification", func() {
			var apiSpecification, _ = json.Marshal(api.ApiSpecificationCreateRequest{
				Specification: map[string]interface{}{
					"openapi": "3.0.0",
					"info": map[string]interface{}{
						"title":          "Rover API",
						"version":        "1.0.0",
						"x-api-category": "test",
						"x-vendor":       "true",
					},
					"servers": []map[string]interface{}{
						{
							"url": "http://rover-api.com/not/there/v1",
						},
					},
				},
			})
			req := httptest.NewRequest(http.MethodPut, "/apispecifications/eni--hyperion--not-there-v1",
				bytes.NewReader(apiSpecification))
			responseGroup, err := ExecuteRequest(req, groupToken)
			ExpectStatusWithBody(responseGroup, err, http.StatusNotFound, "application/problem+json")
		})

		It("should fail to update an ApiSpecification from a different team", func() {
			var apiSpecification, _ = json.Marshal(api.ApiSpecificationCreateRequest{
				Specification: map[string]interface{}{
					"openapi": "3.0.0",
					"info": map[string]interface{}{
						"title":          "Rover API",
						"version":        "1.0.0",
						"x-api-category": "test",
						"x-vendor":       "true",
					},
					"servers": []map[string]interface{}{
						{
							"url": "http://rover-api/eni/distr-other/v1",
						},
					},
				},
			})
			req := httptest.NewRequest(http.MethodPut, "/apispecifications/other--team--eni-distr-other-v1",
				bytes.NewReader(apiSpecification))
			responseGroup, err := ExecuteRequest(req, groupToken)
			ExpectStatusWithBody(responseGroup, err, http.StatusForbidden, "application/problem+json")
		})
	})
})
