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

	"github.com/pkg/errors"
	"github.com/stretchr/testify/mock"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/telekom/controlplane/common-server/pkg/problems"
	"github.com/telekom/controlplane/common-server/pkg/server/middleware/security"
	cconfig "github.com/telekom/controlplane/common/pkg/config"
	fileApi "github.com/telekom/controlplane/file-manager/api"
	filefake "github.com/telekom/controlplane/file-manager/api/fake"
	"github.com/telekom/controlplane/rover-server/internal/api"
	"github.com/telekom/controlplane/rover-server/internal/file"
	"github.com/telekom/controlplane/rover-server/pkg/store"
	"github.com/telekom/controlplane/rover-server/test/mocks"
	roverv1 "github.com/telekom/controlplane/rover/api/v1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// TODO: fix the unit-tests. Use Once() or Twice() for mocks

var _ = Describe("ApiSpecification Controller", func() {

	specV3 := `
openapi: "3.0.0"
info:
  version: "1.0.0"
  title: "Rover API"
servers:
  - url: "http://rover-api.com/eni/distr/v1"
`

	const apiSpecFileId = "01926a3e-7b2c-7d3e-8f4a-1b2c3d4e5f60"

	Context("Get ApiSpecification resource", func() {
		It("should return the ApiSpecification successfully", func() {
			mockFileManager.EXPECT().DownloadFile(mock.Anything, apiSpecFileId, mock.Anything).
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
			mockFileManager.EXPECT().DownloadFile(mock.Anything, apiSpecFileId, mock.Anything).
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
			responseGroup, err := ExecuteRequest(req, teamNoResources)
			ExpectStatusWithBody(responseGroup, err, http.StatusOK, "application/json")
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

			mockFileManager.EXPECT().UploadFile(mock.Anything, apiSpecFileId, mock.Anything, mock.Anything).Return(
				&fileApi.FileUploadResponse{
					FileHash:    "randomHash",
					FileId:      apiSpecFileId,
					ContentType: "application/yaml",
				}, nil)
			mockFileManager.EXPECT().DownloadFile(mock.Anything, apiSpecFileId, mock.Anything).
				RunAndReturn(func(_ context.Context, _ string, w io.Writer) (*fileApi.FileDownloadResponse, error) {

					w.Write([]byte(specV3))

					return &fileApi.FileDownloadResponse{
						FileHash:    "randomHash",
						ContentType: "application/yaml",
					}, nil
				})

			req := httptest.NewRequest(http.MethodPut, "/apispecifications/eni--hyperion--eni-distr-v1",
				bytes.NewReader(apiSpecification))
			responseGroup, err := ExecuteRequest(req, groupToken)
			ExpectStatusWithBody(responseGroup, err, http.StatusAccepted, "application/json")
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

var _ = Describe("ApiSpecification Controller file-manager handling", func() {
	const (
		resourceId = "eni--hyperion--eni-distr-v1"
		namespace  = "poc--eni--hyperion"
		name       = "eni-distr-v1"
		fileId     = "01926a3e-7b2c-7d3e-8f4a-1b2c3d4e5f60"
	)

	var (
		apiCtx   context.Context
		apiStore *mocks.MockObjectStore[*roverv1.ApiSpecification]
		apiFiles *filefake.MockFileManager
		ctrl     *ApiSpecificationController
	)

	specMap := map[string]any{
		"openapi": "3.0.0",
		"info": map[string]any{
			"title":          "Rover API",
			"version":        "1.0.0",
			"x-api-category": "other",
		},
		"servers": []any{
			map[string]any{"url": "http://rover-api.com/eni/distr/v1"},
		},
	}

	BeforeEach(func() {
		apiCtx = security.ToContext(context.Background(), &security.BusinessContext{
			Environment: "poc", Group: "eni", Team: "hyperion",
		})
		apiStore = mocks.NewMockObjectStore[*roverv1.ApiSpecification](GinkgoT())
		apiFiles = filefake.NewMockFileManager(GinkgoT())
		file.GetFileManager = func() fileApi.FileManager { return apiFiles }
		ctrl = NewApiSpecificationController(&store.Stores{APISpecificationStore: apiStore}, nil)
	})

	AfterEach(func() {
		file.GetFileManager = func() fileApi.FileManager { return mockFileManager }
		cconfig.SetFeatureEnabled(cconfig.FeatureFileManager, true)
	})

	Context("file-manager disabled", func() {
		BeforeEach(func() {
			cconfig.SetFeatureEnabled(cconfig.FeatureFileManager, false)
		})

		It("should create the ApiSpecification without calling the file-manager", func() {
			var stored *roverv1.ApiSpecification
			apiStore.EXPECT().Get(mock.Anything, namespace, name).Return(nil, problems.NotFound(name)).Twice()
			apiStore.EXPECT().CreateOrReplace(mock.Anything, mock.Anything).
				RunAndReturn(func(_ context.Context, obj *roverv1.ApiSpecification) error {
					stored = obj
					return nil
				}).Once()
			apiStore.EXPECT().Get(mock.Anything, namespace, name).
				RunAndReturn(func(context.Context, string, string) (*roverv1.ApiSpecification, error) { return stored, nil }).Once()

			res, err := ctrl.Update(apiCtx, resourceId, api.ApiSpecification{Specification: specMap})
			Expect(err).NotTo(HaveOccurred())
			Expect(res.Id).To(Equal(resourceId))
			Expect(res.Specification).To(BeEmpty())

			Expect(stored.Name).To(Equal(name))
			Expect(stored.Namespace).To(Equal(namespace))
			Expect(stored.Spec.BasePath).To(Equal("/eni/distr/v1"))
			Expect(stored.Spec.Version).To(Equal("1.0.0"))
			Expect(stored.Spec.Category).To(Equal("other"))
			Expect(stored.Spec.Specification).To(BeEmpty())
			Expect(stored.Spec.Hash).To(BeEmpty())
		})

		It("should delete the ApiSpecification without calling the file-manager", func() {
			apiStore.EXPECT().Delete(mock.Anything, namespace, name).Return(nil).Once()

			Expect(ctrl.Delete(apiCtx, resourceId)).To(Succeed())
		})
	})

	Context("file-manager enabled", func() {
		It("should skip the download for an ApiSpecification created while the file-manager was disabled", func() {
			apiStore.EXPECT().Get(mock.Anything, namespace, name).Return(&roverv1.ApiSpecification{
				ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
			}, nil).Once()

			res, err := ctrl.Get(apiCtx, resourceId)
			Expect(err).NotTo(HaveOccurred())
			Expect(res.Specification).To(BeEmpty())
		})

		It("should delete the ApiSpecification when the stored file is already missing", func() {
			apiStore.EXPECT().Get(mock.Anything, namespace, name).Return(&roverv1.ApiSpecification{
				ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
				Spec:       roverv1.ApiSpecificationSpec{Specification: fileId},
			}, nil).Once()
			apiFiles.EXPECT().DeleteFile(mock.Anything, fileId).Return(file.ErrNotFound).Once()
			apiStore.EXPECT().Delete(mock.Anything, namespace, name).Return(nil).Once()

			Expect(ctrl.Delete(apiCtx, resourceId)).To(Succeed())
		})

		It("should keep the ApiSpecification when deleting the stored file fails", func() {
			apiStore.EXPECT().Get(mock.Anything, namespace, name).Return(&roverv1.ApiSpecification{
				ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
				Spec:       roverv1.ApiSpecificationSpec{Specification: fileId},
			}, nil).Once()
			apiFiles.EXPECT().DeleteFile(mock.Anything, fileId).Return(errors.New("file-manager unavailable")).Once()

			Expect(ctrl.Delete(apiCtx, resourceId)).To(MatchError("file-manager unavailable"))
			apiStore.AssertNotCalled(GinkgoT(), "Delete", mock.Anything, namespace, name)
		})
	})
})
