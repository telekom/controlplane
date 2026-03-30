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
	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/mock"
	fileApi "github.com/telekom/controlplane/file-manager/api"
	"github.com/telekom/controlplane/rover-server/internal/api"
)

var _ = Describe("Roadmap Controller", func() {

	roadmapItems := []api.RoadmapItem{
		{
			Date:        "Q1 2024",
			Title:       "MVP Release",
			Description: "Initial release with core features",
			TitleUrl:    "https://example.com/mvp",
		},
		{
			Date:        "Q2 2024",
			Title:       "Performance Improvements",
			Description: "Optimize response times",
		},
	}

	roadmapItemsJSON, _ := json.Marshal(roadmapItems)

	Context("Get Roadmap resource", func() {
		It("should return the Roadmap successfully", func() {
			mockFileManager.EXPECT().DownloadFile(mock.Anything, "poc--eni--hyperion--eni-test-api-v1", mock.Anything).
				RunAndReturn(func(_ context.Context, _ string, w io.Writer) (*fileApi.FileDownloadResponse, error) {
					w.Write(roadmapItemsJSON)
					return &fileApi.FileDownloadResponse{
						FileHash:    "randomHash",
						ContentType: "application/json",
					}, nil
				}).Maybe()

			req := httptest.NewRequest(http.MethodGet, "/roadmaps/eni--hyperion--eni-test-api-v1", nil)
			responseGroup, err := ExecuteRequest(req, groupToken)
			ExpectStatusWithBody(responseGroup, err, http.StatusOK, "application/json")
		})

		It("should fail to get a non-existent Roadmap", func() {
			req := httptest.NewRequest(http.MethodGet, "/roadmaps/eni--hyperion--nonexistent", nil)
			responseGroup, err := ExecuteRequest(req, groupToken)
			ExpectStatusWithBody(responseGroup, err, http.StatusNotFound, "application/problem+json")
		})

		It("should fail to get a Roadmap from a different team", func() {
			req := httptest.NewRequest(http.MethodGet, "/roadmaps/other--team--eni-test-api-v1", nil)
			responseGroup, err := ExecuteRequest(req, groupToken)
			ExpectStatusWithBody(responseGroup, err, http.StatusForbidden, "application/problem+json")
		})
	})

	Context("GetAll Roadmaps resource", func() {
		It("should return all Roadmaps successfully", func() {
			mockFileManager.EXPECT().DownloadFile(mock.Anything, "poc--eni--hyperion--eni-test-api-v1", mock.Anything).
				RunAndReturn(func(_ context.Context, _ string, w io.Writer) (*fileApi.FileDownloadResponse, error) {
					w.Write(roadmapItemsJSON)
					return &fileApi.FileDownloadResponse{
						FileHash:    "testHash",
						ContentType: "application/json",
					}, nil
				})

			req := httptest.NewRequest(http.MethodGet, "/roadmaps", nil)
			responseGroup, err := ExecuteRequest(req, groupToken)
			ExpectStatusWithBody(responseGroup, err, http.StatusOK, "application/json")
		})

		It("should filter Roadmaps by resourceType=API", func() {
			mockFileManager.EXPECT().DownloadFile(mock.Anything, "poc--eni--hyperion--eni-test-api-v1", mock.Anything).
				RunAndReturn(func(_ context.Context, _ string, w io.Writer) (*fileApi.FileDownloadResponse, error) {
					w.Write(roadmapItemsJSON)
					return &fileApi.FileDownloadResponse{
						FileHash:    "testHash",
						ContentType: "application/json",
					}, nil
				})

			req := httptest.NewRequest(http.MethodGet, "/roadmaps?resourceType=API", nil)
			responseGroup, err := ExecuteRequest(req, groupToken)
			ExpectStatusWithBody(responseGroup, err, http.StatusOK, "application/json")
		})

		It("should return an empty list if no Roadmaps exist", func() {
			req := httptest.NewRequest(http.MethodGet, "/roadmaps", nil)
			responseGroup, err := ExecuteRequest(req, teamNoResources)
			ExpectStatusWithBody(responseGroup, err, http.StatusOK, "application/json")

			var listResp api.RoadmapListResponse
			json.NewDecoder(responseGroup.Body).Decode(&listResp)
			Expect(listResp.Items).To(BeEmpty())
		})
	})

	Context("Create Roadmap resource", func() {
		It("should return StatusNotImplemented", func() {
			roadmapReq := api.RoadmapRequest{
				ResourceName: "/eni/new-api/v1",
				ResourceType: api.RoadmapResourceTypeAPI,
				Items:        roadmapItems,
			}

			reqBody, _ := json.Marshal(roadmapReq)
			req := httptest.NewRequest(http.MethodPost, "/roadmaps", bytes.NewReader(reqBody))
			req.Header.Set("Content-Type", "application/json")

			ExpectStatusNotImplemented(ExecuteRequest(req, groupToken))
			ExpectStatusNotImplemented(ExecuteRequest(req, teamToken))
		})
	})

	Context("Update Roadmap resource", func() {
		It("should update the Roadmap successfully", func() {
			// Specify exact fileId to avoid matching other resource types' expectations
			mockFileManager.EXPECT().UploadFile(mock.Anything, "poc--eni--hyperion--eni-test-api-v1", "application/json", mock.Anything).
				Return(&fileApi.FileUploadResponse{
					FileId:      "poc--eni--hyperion--eni-test-api-v1",
					FileHash:    "updatedHash",
					ContentType: "application/json",
				}, nil)

			// Get() retrieves mock roadmap with fileId "poc--eni--hyperion--eni-test-api-v1"
			mockFileManager.EXPECT().DownloadFile(mock.Anything, "poc--eni--hyperion--eni-test-api-v1", mock.Anything).
				RunAndReturn(func(_ context.Context, _ string, w io.Writer) (*fileApi.FileDownloadResponse, error) {
					w.Write(roadmapItemsJSON)
					return &fileApi.FileDownloadResponse{
						FileHash:    "updatedHash",
						ContentType: "application/json",
					}, nil
				})

			roadmapReq := api.RoadmapRequest{
				ResourceName: "/eni/test-api/v1",
				ResourceType: api.RoadmapResourceTypeAPI,
				Items:        roadmapItems,
			}

			reqBody, _ := json.Marshal(roadmapReq)
			req := httptest.NewRequest(http.MethodPut, "/roadmaps/eni--hyperion--eni-test-api-v1", bytes.NewReader(reqBody))
			req.Header.Set("Content-Type", "application/json")

			responseGroup, err := ExecuteRequest(req, groupToken)
			ExpectStatusWithBody(responseGroup, err, http.StatusOK, "application/json")
		})

		It("should fail to update a Roadmap from a different team", func() {
			roadmapReq := api.RoadmapRequest{
				ResourceName: "/eni/test-api/v1",
				ResourceType: api.RoadmapResourceTypeAPI,
				Items:        roadmapItems,
			}

			reqBody, _ := json.Marshal(roadmapReq)
			req := httptest.NewRequest(http.MethodPut, "/roadmaps/other--team--eni-test-api-v1", bytes.NewReader(reqBody))
			req.Header.Set("Content-Type", "application/json")

			responseGroup, err := ExecuteRequest(req, groupToken)
			ExpectStatusWithBody(responseGroup, err, http.StatusForbidden, "application/problem+json")
		})
	})

	Context("Delete Roadmap resource", func() {
		It("should delete the Roadmap successfully", func() {
			mockFileManager.EXPECT().DeleteFile(mock.Anything, "poc--eni--hyperion--eni-test-api-v1").Return(nil)

			req := httptest.NewRequest(http.MethodDelete, "/roadmaps/eni--hyperion--eni-test-api-v1", nil)
			responseGroup, err := ExecuteRequest(req, groupToken)
			ExpectStatus(responseGroup, err, http.StatusNoContent, "")
		})

		It("should fail to delete a non-existent Roadmap", func() {
			// Use specific fileId to avoid matching delete calls from other resource types
			mockFileManager.EXPECT().DeleteFile(mock.Anything, "poc--eni--hyperion--nonexistent").Return(errors.Errorf("resource not found"))

			req := httptest.NewRequest(http.MethodDelete, "/roadmaps/eni--hyperion--nonexistent", nil)
			responseGroup, err := ExecuteRequest(req, groupToken)
			ExpectStatusWithBody(responseGroup, err, http.StatusNotFound, "application/problem+json")
		})

		It("should fail to delete a Roadmap from a different team", func() {
			req := httptest.NewRequest(http.MethodDelete, "/roadmaps/other--team--eni-test-api-v1", nil)
			responseGroup, err := ExecuteRequest(req, groupToken)
			ExpectStatusWithBody(responseGroup, err, http.StatusForbidden, "application/problem+json")
		})
	})

	Context("Hash optimization", func() {
		It("should skip file upload if hash is unchanged", func() {
			// Download is called once for the response (after skipping upload)
			mockFileManager.EXPECT().DownloadFile(mock.Anything, "poc--eni--hyperion--eni-test-api-v1", mock.Anything).
				RunAndReturn(func(_ context.Context, _ string, w io.Writer) (*fileApi.FileDownloadResponse, error) {
					w.Write(roadmapItemsJSON)
					return &fileApi.FileDownloadResponse{
						FileHash:    "unchangedHash",
						ContentType: "application/json",
					}, nil
				})

			// Upload should NOT be called if hash matches
			// (this is verified by not setting up a mock expectation)

			roadmapReq := api.RoadmapRequest{
				ResourceName: "/eni/test-api/v1",
				ResourceType: api.RoadmapResourceTypeAPI,
				Items:        roadmapItems,
			}

			reqBody, _ := json.Marshal(roadmapReq)
			req := httptest.NewRequest(http.MethodPut, "/roadmaps/eni--hyperion--eni-test-api-v1", bytes.NewReader(reqBody))
			req.Header.Set("Content-Type", "application/json")

			responseGroup, err := ExecuteRequest(req, groupToken)
			ExpectStatusWithBody(responseGroup, err, http.StatusOK, "application/json")
		})
	})
})
