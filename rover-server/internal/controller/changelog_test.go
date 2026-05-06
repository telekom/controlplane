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

var _ = Describe("ApiChangelog Controller", func() {

	changelogItems := []api.ApiChangelogItem{
		{
			Date:        "2024-03-15",
			Version:     "1.2.3",
			Description: "Security fixes and performance improvements",
		},
		{
			Date:        "2024-01-10",
			Version:     "1.0.0",
			Description: "Initial release",
			VersionUrl:  "https://example.com/releases/v1.0.0",
		},
	}

	changelogItemsJSON, _ := json.Marshal(changelogItems)

	var changelogFileMgr *filefake.MockFileManager

	BeforeEach(func() {
		changelogFileMgr = filefake.NewMockFileManager(GinkgoT())
		file.GetFileManager = func() fileApi.FileManager {
			return changelogFileMgr
		}
	})

	AfterEach(func() {
		file.GetFileManager = func() fileApi.FileManager {
			return mockFileManager
		}
	})

	Context("Get ApiChangelog resource", func() {
		It("should return the ApiChangelog successfully", func() {
			changelogFileMgr.EXPECT().DownloadFile(mock.Anything, "poc--eni--hyperion--eni-test-api", mock.Anything).
				RunAndReturn(func(_ context.Context, _ string, w io.Writer) (*fileApi.FileDownloadResponse, error) {
					w.Write(changelogItemsJSON)
					return &fileApi.FileDownloadResponse{
						FileHash:    "randomHash",
						ContentType: "application/json",
					}, nil
				}).Once()

			req := httptest.NewRequest(http.MethodGet, "/apichangelogs/eni--hyperion--eni-test-api", nil)
			responseGroup, err := ExecuteRequest(req, groupToken)
			ExpectStatusWithBody(responseGroup, err, http.StatusOK, "application/json")
		})

		It("should fail to get a non-existent ApiChangelog", func() {
			req := httptest.NewRequest(http.MethodGet, "/apichangelogs/eni--hyperion--nonexistent", nil)
			responseGroup, err := ExecuteRequest(req, groupToken)
			ExpectStatusWithBody(responseGroup, err, http.StatusNotFound, "application/problem+json")
		})

		It("should fail to get an ApiChangelog from a different team", func() {
			req := httptest.NewRequest(http.MethodGet, "/apichangelogs/other--team--eni-test-api", nil)
			responseGroup, err := ExecuteRequest(req, groupToken)
			ExpectStatusWithBody(responseGroup, err, http.StatusForbidden, "application/problem+json")
		})
	})

	Context("GetAll ApiChangelogs resource", func() {
		It("should return all ApiChangelogs successfully", func() {
			changelogFileMgr.EXPECT().DownloadFile(mock.Anything, "poc--eni--hyperion--eni-test-api", mock.Anything).
				RunAndReturn(func(_ context.Context, _ string, w io.Writer) (*fileApi.FileDownloadResponse, error) {
					w.Write(changelogItemsJSON)
					return &fileApi.FileDownloadResponse{
						FileHash:    "testHash",
						ContentType: "application/json",
					}, nil
				}).Once()

			req := httptest.NewRequest(http.MethodGet, "/apichangelogs", nil)
			responseGroup, err := ExecuteRequest(req, groupToken)
			ExpectStatusWithBody(responseGroup, err, http.StatusOK, "application/json")
		})

		It("should return an empty list if no ApiChangelogs exist", func() {
			req := httptest.NewRequest(http.MethodGet, "/apichangelogs", nil)
			responseGroup, err := ExecuteRequest(req, teamNoResources)
			ExpectStatusWithBody(responseGroup, err, http.StatusOK, "application/json")

			var listResp api.ApiChangelogListResponse
			json.NewDecoder(responseGroup.Body).Decode(&listResp)
			Expect(listResp.Items).To(BeEmpty())
		})
	})

	Context("Create ApiChangelog resource", func() {
		It("should return 501 Not Implemented", func() {
			changelogReq := api.ApiChangelogCreateRequest{
				BasePath: "/eni/new-api/v1",
				Items:    changelogItems,
			}

			reqBody, _ := json.Marshal(changelogReq)
			req := httptest.NewRequest(http.MethodPost, "/apichangelogs", bytes.NewReader(reqBody))
			req.Header.Set("Content-Type", "application/json")

			responseGroup, err := ExecuteRequest(req, groupToken)
			ExpectStatusNotImplemented(responseGroup, err)
		})
	})

	Context("Update ApiChangelog resource", func() {
		It("should update the ApiChangelog successfully", func() {
			changelogFileMgr.EXPECT().UploadFile(mock.Anything, "poc--eni--hyperion--eni-test-api", "application/json", mock.Anything).
				Return(&fileApi.FileUploadResponse{
					FileId:      "poc--eni--hyperion--eni-test-api",
					FileHash:    "updatedHash",
					ContentType: "application/json",
				}, nil).Once()

			changelogReq := api.ApiChangelogUpdateRequest{
				BasePath: "/eni/test-api/v1",
				Items:    changelogItems,
			}

			reqBody, _ := json.Marshal(changelogReq)
			req := httptest.NewRequest(http.MethodPut, "/apichangelogs/eni--hyperion--eni-test-api", bytes.NewReader(reqBody))
			req.Header.Set("Content-Type", "application/json")

			responseGroup, err := ExecuteRequest(req, groupToken)
			ExpectStatusWithBody(responseGroup, err, http.StatusAccepted, "application/json")
		})

		It("should fail to update an ApiChangelog from a different team", func() {
			changelogReq := api.ApiChangelogUpdateRequest{
				BasePath: "/eni/test-api/v1",
				Items:    changelogItems,
			}

			reqBody, _ := json.Marshal(changelogReq)
			req := httptest.NewRequest(http.MethodPut, "/apichangelogs/other--team--eni-test-api", bytes.NewReader(reqBody))
			req.Header.Set("Content-Type", "application/json")

			responseGroup, err := ExecuteRequest(req, groupToken)
			ExpectStatusWithBody(responseGroup, err, http.StatusForbidden, "application/problem+json")
		})

		It("should reject empty basePath", func() {
			changelogReq := api.ApiChangelogUpdateRequest{
				BasePath: "",
				Items:    changelogItems,
			}

			reqBody, _ := json.Marshal(changelogReq)
			req := httptest.NewRequest(http.MethodPut, "/apichangelogs/eni--hyperion--eni-test-api", bytes.NewReader(reqBody))
			req.Header.Set("Content-Type", "application/json")

			responseGroup, err := ExecuteRequest(req, groupToken)
			ExpectStatusWithBody(responseGroup, err, http.StatusBadRequest, "application/problem+json")
		})

		It("should reject empty items array", func() {
			changelogReq := api.ApiChangelogUpdateRequest{
				BasePath: "/eni/test-api/v1",
				Items:    []api.ApiChangelogItem{},
			}

			reqBody, _ := json.Marshal(changelogReq)
			req := httptest.NewRequest(http.MethodPut, "/apichangelogs/eni--hyperion--eni-test-api", bytes.NewReader(reqBody))
			req.Header.Set("Content-Type", "application/json")

			responseGroup, err := ExecuteRequest(req, groupToken)
			ExpectStatusWithBody(responseGroup, err, http.StatusBadRequest, "application/problem+json")
		})

		It("should reject mismatched basePath and resourceId", func() {
			changelogReq := api.ApiChangelogUpdateRequest{
				BasePath: "/eni/different-api/v1",
				Items:    changelogItems,
			}

			reqBody, _ := json.Marshal(changelogReq)
			req := httptest.NewRequest(http.MethodPut, "/apichangelogs/eni--hyperion--eni-test-api", bytes.NewReader(reqBody))
			req.Header.Set("Content-Type", "application/json")

			responseGroup, err := ExecuteRequest(req, groupToken)
			ExpectStatusWithBody(responseGroup, err, http.StatusBadRequest, "application/problem+json")
		})
	})

	Context("Delete ApiChangelog resource", func() {
		It("should delete the ApiChangelog successfully", func() {
			changelogFileMgr.EXPECT().DeleteFile(mock.Anything, "poc--eni--hyperion--eni-test-api").Return(nil).Once()

			req := httptest.NewRequest(http.MethodDelete, "/apichangelogs/eni--hyperion--eni-test-api", nil)
			responseGroup, err := ExecuteRequest(req, groupToken)
			ExpectStatus(responseGroup, err, http.StatusNoContent, "")
		})

		It("should fail to delete a non-existent ApiChangelog", func() {
			// No file manager mock needed - Store.Get returns not-found before DeleteFile is reached
			req := httptest.NewRequest(http.MethodDelete, "/apichangelogs/eni--hyperion--nonexistent", nil)
			responseGroup, err := ExecuteRequest(req, groupToken)
			ExpectStatusWithBody(responseGroup, err, http.StatusNotFound, "application/problem+json")
		})

		It("should fail to delete an ApiChangelog from a different team", func() {
			req := httptest.NewRequest(http.MethodDelete, "/apichangelogs/other--team--eni-test-api", nil)
			responseGroup, err := ExecuteRequest(req, groupToken)
			ExpectStatusWithBody(responseGroup, err, http.StatusForbidden, "application/problem+json")
		})
	})

	Context("GetStatus ApiChangelog resource", func() {
		It("should return the status of the ApiChangelog successfully", func() {
			req := httptest.NewRequest(http.MethodGet, "/apichangelogs/eni--hyperion--eni-test-api/status", nil)
			responseGroup, err := ExecuteRequest(req, groupToken)
			ExpectStatusWithBody(responseGroup, err, http.StatusOK, "application/json")
		})

		It("should fail to get the status of a non-existent ApiChangelog", func() {
			req := httptest.NewRequest(http.MethodGet, "/apichangelogs/eni--hyperion--nonexistent/status", nil)
			responseGroup, err := ExecuteRequest(req, groupToken)
			ExpectStatusWithBody(responseGroup, err, http.StatusNotFound, "application/problem+json")
		})

		It("should fail to get the status of an ApiChangelog from a different team", func() {
			req := httptest.NewRequest(http.MethodGet, "/apichangelogs/other--team--eni-test-api/status", nil)
			responseGroup, err := ExecuteRequest(req, groupToken)
			ExpectStatusWithBody(responseGroup, err, http.StatusForbidden, "application/problem+json")
		})
	})

	// TODO: To properly test hash optimization, the store mock would need to return
	// the actual SHA256 hash of changelogItemsJSON. Currently the stored hash is
	// "changelogRandomHash" which never matches, so UploadFile is always called.
	// This requires a per-test store mock, not just a per-test file manager mock.
})
