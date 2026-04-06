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
	"github.com/telekom/controlplane/rover-server/internal/api"
)

var _ = Describe("Changelog Controller", func() {

	changelogItemsJSON := `[{"date":"2024-03-15","version":"1.2.3","description":"Security fixes and performance improvements"},{"date":"2024-01-10","version":"1.0.0","description":"Initial release","versionUrl":"https://example.com/releases/v1.0.0"}]`

	Context("Get Changelog resource", func() {
		It("should return the Changelog successfully", func() {
			mockFileManager.EXPECT().DownloadFile(mock.Anything, "poc--eni--hyperion--eni-test-api-v1", mock.Anything).
				RunAndReturn(func(_ context.Context, _ string, w io.Writer) (*fileApi.FileDownloadResponse, error) {
					w.Write([]byte(changelogItemsJSON))
					return &fileApi.FileDownloadResponse{
						FileHash:    "randomHash",
						ContentType: "application/json",
					}, nil
				}).Maybe()

			req := httptest.NewRequest(http.MethodGet, "/changelogs/eni--hyperion--eni-test-api-v1", nil)
			responseGroup, err := ExecuteRequest(req, groupToken)
			ExpectStatusWithBody(responseGroup, err, http.StatusOK, "application/json")
		})

		It("should fail to get a non-existent Changelog", func() {
			req := httptest.NewRequest(http.MethodGet, "/changelogs/eni--hyperion--nonexistent", nil)
			responseGroup, err := ExecuteRequest(req, groupToken)
			ExpectStatusWithBody(responseGroup, err, http.StatusNotFound, "application/problem+json")
		})

		It("should fail to get a Changelog from a different team", func() {
			req := httptest.NewRequest(http.MethodGet, "/changelogs/other--team--eni-test-api-v1", nil)
			responseGroup, err := ExecuteRequest(req, groupToken)
			ExpectStatusWithBody(responseGroup, err, http.StatusForbidden, "application/problem+json")
		})
	})

	Context("GetAll Changelogs resource", func() {
		It("should return all Changelogs successfully", func() {
			mockFileManager.EXPECT().DownloadFile(mock.Anything, "poc--eni--hyperion--eni-test-api-v1", mock.Anything).
				RunAndReturn(func(_ context.Context, _ string, w io.Writer) (*fileApi.FileDownloadResponse, error) {
					w.Write([]byte(changelogItemsJSON))
					return &fileApi.FileDownloadResponse{
						FileHash:    "testHash",
						ContentType: "application/json",
					}, nil
				})

			req := httptest.NewRequest(http.MethodGet, "/changelogs", nil)
			responseGroup, err := ExecuteRequest(req, groupToken)
			ExpectStatusWithBody(responseGroup, err, http.StatusOK, "application/json")
		})

		It("should filter Changelogs by resourceType=API", func() {
			mockFileManager.EXPECT().DownloadFile(mock.Anything, "poc--eni--hyperion--eni-test-api-v1", mock.Anything).
				RunAndReturn(func(_ context.Context, _ string, w io.Writer) (*fileApi.FileDownloadResponse, error) {
					w.Write([]byte(changelogItemsJSON))
					return &fileApi.FileDownloadResponse{
						FileHash:    "testHash",
						ContentType: "application/json",
					}, nil
				})

			req := httptest.NewRequest(http.MethodGet, "/changelogs?resourceType=API", nil)
			responseGroup, err := ExecuteRequest(req, groupToken)
			ExpectStatusWithBody(responseGroup, err, http.StatusOK, "application/json")
		})

		It("should return an empty list if no Changelogs exist", func() {
			req := httptest.NewRequest(http.MethodGet, "/changelogs", nil)
			responseGroup, err := ExecuteRequest(req, teamNoResources)
			ExpectStatusWithBody(responseGroup, err, http.StatusOK, "application/json")

			var listResp api.ChangelogListResponse
			json.NewDecoder(responseGroup.Body).Decode(&listResp)
			Expect(listResp.Items).To(BeEmpty())
		})
	})

	Context("Update Changelog resource", func() {
		It("should update the Changelog successfully", func() {
			mockFileManager.EXPECT().UploadFile(mock.Anything, "poc--eni--hyperion--eni-test-api-v1", "application/json", mock.Anything).
				Return(&fileApi.FileUploadResponse{
					FileId:      "poc--eni--hyperion--eni-test-api-v1",
					FileHash:    "updatedHash",
					ContentType: "application/json",
				}, nil)

			mockFileManager.EXPECT().DownloadFile(mock.Anything, "poc--eni--hyperion--eni-test-api-v1", mock.Anything).
				RunAndReturn(func(_ context.Context, _ string, w io.Writer) (*fileApi.FileDownloadResponse, error) {
					w.Write([]byte(changelogItemsJSON))
					return &fileApi.FileDownloadResponse{
						FileHash:    "updatedHash",
						ContentType: "application/json",
					}, nil
				})

			reqBody := `{"resourceName":"/eni/test-api/v1","resourceType":"API","items":[{"date":"2024-03-15","version":"1.2.3","description":"Security fixes"}]}`
			req := httptest.NewRequest(http.MethodPut, "/changelogs/eni--hyperion--eni-test-api-v1", bytes.NewReader([]byte(reqBody)))
			req.Header.Set("Content-Type", "application/json")

			responseGroup, err := ExecuteRequest(req, groupToken)
			ExpectStatusWithBody(responseGroup, err, http.StatusOK, "application/json")
		})

		It("should fail to update a Changelog from a different team", func() {
			reqBody := `{"resourceName":"/eni/test-api/v1","resourceType":"API","items":[{"date":"2024-03-15","version":"1.2.3","description":"Security fixes"}]}`
			req := httptest.NewRequest(http.MethodPut, "/changelogs/other--team--eni-test-api-v1", bytes.NewReader([]byte(reqBody)))
			req.Header.Set("Content-Type", "application/json")

			responseGroup, err := ExecuteRequest(req, groupToken)
			ExpectStatusWithBody(responseGroup, err, http.StatusForbidden, "application/problem+json")
		})

		It("should reject invalid version format", func() {
			reqBody := `{"resourceName":"/eni/test-api/v1","resourceType":"API","items":[{"date":"2024-03-15","version":"not-semver","description":"Bad version"}]}`
			req := httptest.NewRequest(http.MethodPut, "/changelogs/eni--hyperion--eni-test-api-v1", bytes.NewReader([]byte(reqBody)))
			req.Header.Set("Content-Type", "application/json")

			responseGroup, err := ExecuteRequest(req, groupToken)
			ExpectStatusWithBody(responseGroup, err, http.StatusBadRequest, "application/problem+json")
		})

		It("should reject empty items array", func() {
			reqBody := `{"resourceName":"/eni/test-api/v1","resourceType":"API","items":[]}`
			req := httptest.NewRequest(http.MethodPut, "/changelogs/eni--hyperion--eni-test-api-v1", bytes.NewReader([]byte(reqBody)))
			req.Header.Set("Content-Type", "application/json")

			responseGroup, err := ExecuteRequest(req, groupToken)
			ExpectStatusWithBody(responseGroup, err, http.StatusBadRequest, "application/problem+json")
		})

		It("should reject invalid resourceType", func() {
			reqBody := `{"resourceName":"/eni/test-api/v1","resourceType":"INVALID","items":[{"date":"2024-03-15","version":"1.0.0","description":"Test"}]}`
			req := httptest.NewRequest(http.MethodPut, "/changelogs/eni--hyperion--eni-test-api-v1", bytes.NewReader([]byte(reqBody)))
			req.Header.Set("Content-Type", "application/json")

			responseGroup, err := ExecuteRequest(req, groupToken)
			ExpectStatusWithBody(responseGroup, err, http.StatusBadRequest, "application/problem+json")
		})
	})

	Context("Delete Changelog resource", func() {
		It("should delete the Changelog successfully", func() {
			mockFileManager.EXPECT().DeleteFile(mock.Anything, "poc--eni--hyperion--eni-test-api-v1").Return(nil)

			req := httptest.NewRequest(http.MethodDelete, "/changelogs/eni--hyperion--eni-test-api-v1", nil)
			responseGroup, err := ExecuteRequest(req, groupToken)
			ExpectStatus(responseGroup, err, http.StatusNoContent, "")
		})

		It("should fail to delete a non-existent Changelog", func() {
			req := httptest.NewRequest(http.MethodDelete, "/changelogs/eni--hyperion--nonexistent", nil)
			responseGroup, err := ExecuteRequest(req, groupToken)
			ExpectStatusWithBody(responseGroup, err, http.StatusNotFound, "application/problem+json")
		})

		It("should fail to delete a Changelog from a different team", func() {
			req := httptest.NewRequest(http.MethodDelete, "/changelogs/other--team--eni-test-api-v1", nil)
			responseGroup, err := ExecuteRequest(req, groupToken)
			ExpectStatusWithBody(responseGroup, err, http.StatusForbidden, "application/problem+json")
		})
	})

	Context("Hash optimization", func() {
		It("should skip file upload if hash is unchanged", func() {
			mockFileManager.EXPECT().DownloadFile(mock.Anything, "poc--eni--hyperion--eni-test-api-v1", mock.Anything).
				RunAndReturn(func(_ context.Context, _ string, w io.Writer) (*fileApi.FileDownloadResponse, error) {
					w.Write([]byte(changelogItemsJSON))
					return &fileApi.FileDownloadResponse{
						FileHash:    "unchangedHash",
						ContentType: "application/json",
					}, nil
				})

			reqBody := `{"resourceName":"/eni/test-api/v1","resourceType":"API","items":[{"date":"2024-03-15","version":"1.2.3","description":"Security fixes"}]}`
			req := httptest.NewRequest(http.MethodPut, "/changelogs/eni--hyperion--eni-test-api-v1", bytes.NewReader([]byte(reqBody)))
			req.Header.Set("Content-Type", "application/json")

			responseGroup, err := ExecuteRequest(req, groupToken)
			ExpectStatusWithBody(responseGroup, err, http.StatusOK, "application/json")
		})
	})
})
