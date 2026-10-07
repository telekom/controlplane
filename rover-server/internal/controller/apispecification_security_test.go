// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/mock"
	"github.com/telekom/controlplane/common-server/pkg/problems"
	cserver "github.com/telekom/controlplane/common-server/pkg/server"
	"github.com/telekom/controlplane/common-server/pkg/server/middleware/security"
	cconfig "github.com/telekom/controlplane/common/pkg/config"
	fileApi "github.com/telekom/controlplane/file-manager/api"
	"github.com/telekom/controlplane/rover-server/internal/api"
	"github.com/telekom/controlplane/rover-server/internal/server"
	s "github.com/telekom/controlplane/rover-server/pkg/store"
	"github.com/telekom/controlplane/rover-server/test/mocks"
	roverv1 "github.com/telekom/controlplane/rover/api/v1"
	"gopkg.in/yaml.v3"
)

var _ = Describe("ApiSpecification OAuth2 security round trip", func() {
	DescribeTable("applies mixed schemes without importing other flow scopes", func(clientScopes string, expectedScopes []string) {
		wasEnabled := cconfig.FeatureFileManager.IsEnabled()
		cconfig.SetFeatureEnabled(cconfig.FeatureFileManager, true)
		DeferCleanup(func() {
			cconfig.SetFeatureEnabled(cconfig.FeatureFileManager, wasEnabled)
		})

		const resourceID = "eni--hyperion--test-v1"
		const fileID = "01926a3e-7b2c-7d3e-8f4a-1b2c3d4e5f60"
		document := fmt.Sprintf(`{
			"openapi": "3.0.3",
			"info": {"title": "Test API", "version": "1.0.0"},
			"servers": [{"url": "https://example.com/test/v1"}],
			"paths": {"/sessions": {"get": {
				"security": [{"service": []}, {"threeLegged": ["user-read"]}],
				"responses": {"200": {"description": "OK"}}
			}}},
			"components": {"securitySchemes": {
				"service": {"type": "oauth2", "flows": {"clientCredentials": {
					"tokenUrl": "https://example.com/token", "scopes": %s
				}}},
				"threeLegged": {"type": "oauth2", "flows": {"authorizationCode": {
					"authorizationUrl": "https://example.com/authorize",
					"tokenUrl": "https://example.com/token", "scopes": {"user-read": "Read sessions"}
				}}}
			}}
		}`, clientScopes)
		var expectedDocument map[string]any
		Expect(json.Unmarshal([]byte(document), &expectedDocument)).To(Succeed())

		var persisted *roverv1.ApiSpecification
		specStore := mocks.NewMockObjectStore[*roverv1.ApiSpecification](GinkgoT())
		specStore.EXPECT().Get(mock.Anything, "poc--eni--hyperion", "test-v1").
			RunAndReturn(func(context.Context, string, string) (*roverv1.ApiSpecification, error) {
				if persisted == nil {
					return nil, problems.NotFound()
				}
				return persisted.DeepCopy(), nil
			}).Times(4)
		specStore.EXPECT().CreateOrReplace(mock.Anything, mock.Anything).
			RunAndReturn(func(_ context.Context, spec *roverv1.ApiSpecification) error {
				persisted = spec.DeepCopy()
				return nil
			}).Once()

		var uploaded []byte
		fileManager := isolateFileManager()
		fileManager.EXPECT().UploadFile(mock.Anything, mock.Anything, "application/yaml", mock.Anything).
			RunAndReturn(func(_ context.Context, _ string, _ string, reader io.Reader) (*fileApi.FileUploadResponse, error) {
				var err error
				uploaded, err = io.ReadAll(reader)
				Expect(err).NotTo(HaveOccurred())
				return &fileApi.FileUploadResponse{FileId: fileID, FileHash: computeHash(uploaded)}, nil
			}).Once()
		fileManager.EXPECT().DownloadFile(mock.Anything, fileID, mock.Anything).
			RunAndReturn(func(_ context.Context, _ string, writer io.Writer) (*fileApi.FileDownloadResponse, error) {
				_, err := writer.Write(uploaded)
				return &fileApi.FileDownloadResponse{ContentType: "application/yaml"}, err
			}).Twice()

		localStores := &s.Stores{APISpecificationStore: specStore}
		localServer := &server.Server{ApiSpecifications: NewApiSpecificationController(localStores, nil)}
		localApp := cserver.NewApp()
		family := cserver.JWTFamily(security.SecurityOpts{
			Mode: security.ModeMock,
			BusinessContextOpts: []security.Option[*security.BusinessContextOpts]{
				security.WithDefaultScope("tardis:team:all"),
				security.WithScopePrefix("tardis:"),
			},
			CheckAccessOpts: []security.Option[*security.CheckAccessOpts]{
				security.WithPathParamKey("resourceId"),
				security.WithTemplates(server.SecurityTemplates),
			},
		})
		localServer.RegisterRoutes(localApp, family(localApp))

		body, err := json.Marshal(api.ApiSpecification{Specification: expectedDocument})
		Expect(err).NotTo(HaveOccurred())
		request := httptest.NewRequest(http.MethodPut, "/apispecifications/"+resourceID, bytes.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Authorization", "Bearer "+groupToken)
		response, err := localApp.Test(request, -1)
		ExpectStatus(response, err, http.StatusAccepted, "application/json")
		DeferCleanup(func() { Expect(response.Body.Close()).To(Succeed()) })

		Expect(persisted).NotTo(BeNil())
		Expect(persisted.Spec.Oauth2Scopes).To(ConsistOf(expectedScopes))
		var uploadedDocument map[string]any
		Expect(yaml.Unmarshal(uploaded, &uploadedDocument)).To(Succeed())
		Expect(uploadedDocument).To(Equal(expectedDocument))
		var result api.ApiSpecificationResponse
		Expect(json.NewDecoder(response.Body).Decode(&result)).To(Succeed())
		Expect(result.Specification).To(Equal(expectedDocument))

		getRequest := httptest.NewRequest(http.MethodGet, "/apispecifications/"+resourceID, nil)
		getRequest.Header.Set("Authorization", "Bearer "+groupToken)
		getResponse, err := localApp.Test(getRequest, -1)
		ExpectStatus(getResponse, err, http.StatusOK, "application/json")
		DeferCleanup(func() { Expect(getResponse.Body.Close()).To(Succeed()) })
		Expect(json.NewDecoder(getResponse.Body).Decode(&result)).To(Succeed())
		Expect(result.Specification).To(Equal(expectedDocument))
	},
		Entry("empty client-credentials scopes", `{}`, []string{}),
		Entry("non-empty client-credentials scopes", `{"service-read": "Read as a service"}`, []string{"service-read"}),
	)
})
