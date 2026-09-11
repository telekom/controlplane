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
	"github.com/stretchr/testify/mock"
	fileApi "github.com/telekom/controlplane/file-manager/api"
	filefake "github.com/telekom/controlplane/file-manager/api/fake"
	"github.com/telekom/controlplane/rover-server/internal/api"
	"github.com/telekom/controlplane/rover-server/internal/file"
)

var _ = Describe("Roadmap Controller", func() {

	roadmapItems := []api.ApiRoadmapItem{
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

	var roadmapFileMgr *filefake.MockFileManager

	BeforeEach(func() {
		roadmapFileMgr = filefake.NewMockFileManager(GinkgoT())
		file.GetFileManager = func() fileApi.FileManager {
			return roadmapFileMgr
		}
	})

	AfterEach(func() {
		file.GetFileManager = func() fileApi.FileManager {
			return mockFileManager
		}
	})

	Context("Get ApiRoadmap resource", func() {
		It("should return the ApiRoadmap successfully", func() {
			roadmapFileMgr.EXPECT().DownloadFile(mock.Anything, "poc--eni--hyperion--eni-test-api", mock.Anything).
				RunAndReturn(func(_ context.Context, _ string, w io.Writer) (*fileApi.FileDownloadResponse, error) {
					w.Write(roadmapItemsJSON)
					return &fileApi.FileDownloadResponse{
						FileHash:    "randomHash",
						ContentType: "application/json",
					}, nil
				}).Once()

			req := httptest.NewRequest(http.MethodGet, "/apiroadmaps/eni--hyperion--eni-test-api", nil)
			responseGroup, err := ExecuteRequest(req, groupToken)
			ExpectStatusWithBody(responseGroup, err, http.StatusOK, "application/json")
		})

		It("should fail to get a non-existent ApiRoadmap", func() {
			req := httptest.NewRequest(http.MethodGet, "/apiroadmaps/eni--hyperion--nonexistent", nil)
			responseGroup, err := ExecuteRequest(req, groupToken)
			ExpectStatusWithBody(responseGroup, err, http.StatusNotFound, "application/problem+json")
		})

		It("should fail to get an ApiRoadmap from a different team", func() {
			req := httptest.NewRequest(http.MethodGet, "/apiroadmaps/other--team--eni-test-api", nil)
			responseGroup, err := ExecuteRequest(req, groupToken)
			ExpectStatusWithBody(responseGroup, err, http.StatusForbidden, "application/problem+json")
		})
	})

	Context("GetAll ApiRoadmaps resource", func() {
		It("should return all ApiRoadmaps successfully", func() {
			roadmapFileMgr.EXPECT().DownloadFile(mock.Anything, "poc--eni--hyperion--eni-test-api", mock.Anything).
				RunAndReturn(func(_ context.Context, _ string, w io.Writer) (*fileApi.FileDownloadResponse, error) {
					w.Write(roadmapItemsJSON)
					return &fileApi.FileDownloadResponse{
						FileHash:    "testHash",
						ContentType: "application/json",
					}, nil
				}).Once()

			req := httptest.NewRequest(http.MethodGet, "/apiroadmaps", nil)
			responseGroup, err := ExecuteRequest(req, groupToken)
			ExpectStatusWithBody(responseGroup, err, http.StatusOK, "application/json")
		})

		It("should return an empty list if no ApiRoadmaps exist", func() {
			req := httptest.NewRequest(http.MethodGet, "/apiroadmaps", nil)
			responseGroup, err := ExecuteRequest(req, teamNoResources)
			ExpectStatusWithBody(responseGroup, err, http.StatusOK, "application/json")

			var listResp api.ApiRoadmapListResponse
			json.NewDecoder(responseGroup.Body).Decode(&listResp)
			Expect(listResp.Items).To(BeEmpty())
		})
	})

	Context("Create ApiRoadmap resource", func() {
		It("should return 501 Not Implemented", func() {
			roadmapReq := api.ApiRoadmapCreateRequest{
				BasePath: "/eni/new-api/v1",
				Items:    roadmapItems,
			}

			reqBody, _ := json.Marshal(roadmapReq)
			req := httptest.NewRequest(http.MethodPost, "/apiroadmaps", bytes.NewReader(reqBody))
			req.Header.Set("Content-Type", "application/json")

			responseGroup, err := ExecuteRequest(req, groupToken)
			ExpectStatusNotImplemented(responseGroup, err)
		})
	})

	Context("Update ApiRoadmap resource", func() {
		It("should update the ApiRoadmap successfully", func() {
			roadmapFileMgr.EXPECT().UploadFile(mock.Anything, "poc--eni--hyperion--eni-test-api", "application/json", mock.Anything).
				Return(&fileApi.FileUploadResponse{
					FileId:      "poc--eni--hyperion--eni-test-api",
					FileHash:    "updatedHash",
					ContentType: "application/json",
				}, nil).Once()

			roadmapReq := api.ApiRoadmapUpdateRequest{
				BasePath: "/eni/test-api/v1",
				Items:    roadmapItems,
			}

			reqBody, _ := json.Marshal(roadmapReq)
			req := httptest.NewRequest(http.MethodPut, "/apiroadmaps/eni--hyperion--eni-test-api", bytes.NewReader(reqBody))
			req.Header.Set("Content-Type", "application/json")

			responseGroup, err := ExecuteRequest(req, groupToken)
			ExpectStatusWithBody(responseGroup, err, http.StatusAccepted, "application/json")
		})

		It("should fail to update an ApiRoadmap from a different team", func() {
			roadmapReq := api.ApiRoadmapUpdateRequest{
				BasePath: "/eni/test-api/v1",
				Items:    roadmapItems,
			}

			reqBody, _ := json.Marshal(roadmapReq)
			req := httptest.NewRequest(http.MethodPut, "/apiroadmaps/other--team--eni-test-api", bytes.NewReader(reqBody))
			req.Header.Set("Content-Type", "application/json")

			responseGroup, err := ExecuteRequest(req, groupToken)
			ExpectStatusWithBody(responseGroup, err, http.StatusForbidden, "application/problem+json")
		})
	})

	Context("Delete ApiRoadmap resource", func() {
		It("should delete the ApiRoadmap successfully", func() {
			roadmapFileMgr.EXPECT().DeleteFile(mock.Anything, "poc--eni--hyperion--eni-test-api").Return(nil).Once()

			req := httptest.NewRequest(http.MethodDelete, "/apiroadmaps/eni--hyperion--eni-test-api", nil)
			responseGroup, err := ExecuteRequest(req, groupToken)
			ExpectStatus(responseGroup, err, http.StatusNoContent, "")
		})

		It("should fail to delete a non-existent ApiRoadmap", func() {
			// No file manager mock needed - Store.Get returns not-found before DeleteFile is reached
			req := httptest.NewRequest(http.MethodDelete, "/apiroadmaps/eni--hyperion--nonexistent", nil)
			responseGroup, err := ExecuteRequest(req, groupToken)
			ExpectStatusWithBody(responseGroup, err, http.StatusNotFound, "application/problem+json")
		})

		It("should fail to delete an ApiRoadmap from a different team", func() {
			req := httptest.NewRequest(http.MethodDelete, "/apiroadmaps/other--team--eni-test-api", nil)
			responseGroup, err := ExecuteRequest(req, groupToken)
			ExpectStatusWithBody(responseGroup, err, http.StatusForbidden, "application/problem+json")
		})
	})

	Context("GetStatus ApiRoadmap resource", func() {
		It("should return the status of the ApiRoadmap successfully", func() {
			req := httptest.NewRequest(http.MethodGet, "/apiroadmaps/eni--hyperion--eni-test-api/status", nil)
			responseGroup, err := ExecuteRequest(req, groupToken)
			ExpectStatusWithBody(responseGroup, err, http.StatusOK, "application/json")
		})

		It("should fail to get the status of a non-existent ApiRoadmap", func() {
			req := httptest.NewRequest(http.MethodGet, "/apiroadmaps/eni--hyperion--nonexistent/status", nil)
			responseGroup, err := ExecuteRequest(req, groupToken)
			ExpectStatusWithBody(responseGroup, err, http.StatusNotFound, "application/problem+json")
		})

		It("should fail to get the status of an ApiRoadmap from a different team", func() {
			req := httptest.NewRequest(http.MethodGet, "/apiroadmaps/other--team--eni-test-api/status", nil)
			responseGroup, err := ExecuteRequest(req, groupToken)
			ExpectStatusWithBody(responseGroup, err, http.StatusForbidden, "application/problem+json")
		})
	})

	// TODO: To properly test hash optimization, the store mock would need to return
	// the actual SHA256 hash of roadmapItemsJSON. Currently the stored hash is
	// "roadmapRandomHash" which never matches, so UploadFile is always called.
	// This requires a per-test store mock, not just a per-test file manager mock.
})
