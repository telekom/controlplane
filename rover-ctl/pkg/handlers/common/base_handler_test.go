// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package common_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/mock"
	"github.com/telekom/controlplane/rover-ctl/pkg/config"
	"github.com/telekom/controlplane/rover-ctl/pkg/handlers/common"
	"github.com/telekom/controlplane/rover-ctl/pkg/types"
	"github.com/telekom/controlplane/rover-ctl/test/mocks"
)

var _ = Describe("BaseHandler", func() {
	var (
		handler                *common.BaseHandler
		mockClient             *mocks.MockHttpDoer
		testCtx                context.Context
		originalHttpClientFunc func(ctx context.Context, tokenUrl, clientId, clientSecret string) common.HttpDoer
	)

	BeforeEach(func() {
		// Save the original client function
		originalHttpClientFunc = common.NewAuthorizedHttpClient

		// Create a mockery-generated mock client
		mockClient = mocks.NewMockHttpDoer(GinkgoT())

		// Replace the client creation function with one that returns our mock
		common.NewAuthorizedHttpClient = func(ctx context.Context, tokenUrl, clientId, clientSecret string) common.HttpDoer {
			return mockClient
		}

		// Create a test token and context
		token := &config.Token{
			ClientId:     "test-client",
			ClientSecret: "test-secret",
			TokenUrl:     "https://example.com/token",
			ServerUrl:    "https://api.example.com",
			Group:        "my-group",
			Team:         "my-team",
		}
		testCtx = config.NewContext(context.Background(), token)

		// Create a handler for testing
		handler = common.NewBaseHandler("v1", "Test", "resources", 100)
		handler.Setup(testCtx)
	})

	AfterEach(func() {
		// Restore the original client function
		common.NewAuthorizedHttpClient = originalHttpClientFunc
	})

	Describe("Apply", func() {
		Context("when sending a valid request", func() {
			It("should send an Apply request to the correct URL", func() {
				// Prepare a test object
				testObj := map[string]any{
					"apiVersion": "v1",
					"kind":       "Test",
					"metadata": map[string]any{
						"name": "test-resource",
					},
					"spec": map[string]any{
						"foo": "bar",
					},
				}

				// Configure the mock to return a success response
				mockClient.EXPECT().Do(mock.AnythingOfType("*http.Request")).RunAndReturn(func(req *http.Request) (*http.Response, error) {
					// Verify the request
					Expect(req.Method).To(Equal(http.MethodPut))
					Expect(req.URL.String()).To(Equal("https://api.example.com/resources/my-group--my-team--test-resource"))

					// Verify the request body
					body, err := io.ReadAll(req.Body)
					Expect(err).NotTo(HaveOccurred())

					var sentObj map[string]any
					err = json.Unmarshal(body, &sentObj)
					Expect(err).NotTo(HaveOccurred())
					Expect(sentObj).To(HaveKeyWithValue("apiVersion", "v1"))
					Expect(sentObj).To(HaveKeyWithValue("kind", "Test"))

					// Create a successful response
					return &http.Response{
						StatusCode: http.StatusAccepted,
						Body: io.NopCloser(bytes.NewBufferString(`{
							"apiVersion": "v1",
							"kind": "Test",
							"metadata": {"name": "test-resource"},
							"status": {"state": "processing"}
						}`)),
						Header: make(http.Header),
					}, nil
				})

				// Call Apply
				obj := &types.UnstructuredObject{Content: testObj}
				err := handler.Apply(testCtx, obj)

				// Verify results
				Expect(err).NotTo(HaveOccurred())

				// Verify that our mock expectations were met
				mockClient.AssertExpectations(GinkgoT())
			})
		})

		Context("when the API returns an error", func() {
			It("should handle API error responses", func() {
				// Prepare a test object
				testObj := map[string]any{
					"apiVersion": "v1",
					"kind":       "Test",
					"metadata": map[string]any{
						"name": "test-resource",
					},
				}

				// Configure the mock to return an error response
				mockClient.EXPECT().Do(mock.AnythingOfType("*http.Request")).RunAndReturn(func(req *http.Request) (*http.Response, error) {
					return &http.Response{
						StatusCode: http.StatusBadRequest,
						Body: io.NopCloser(bytes.NewBufferString(`{
							"type": "ValidationError",
							"status": 400,
							"title": "Validation failed",
							"detail": "Field 'spec' is required",
							"instance": "PUT/resources",
							"fields": [
								{
									"field": "spec",
									"detail": "Field is required"
								}
							]
						}`)),
						Header: make(http.Header),
					}, nil
				})

				// Call Apply
				obj := &types.UnstructuredObject{Content: testObj}
				err := handler.Apply(testCtx, obj)

				// Verify error
				Expect(err).To(HaveOccurred())
				apiErr, ok := common.AsApiError(err)
				Expect(ok).To(BeTrue())
				Expect(apiErr).NotTo(BeNil())
				// The error message comes from the mock response we defined above
				Expect(apiErr.Title).To(Equal("Validation failed"))
			})
		})
	})

	Describe("Delete", func() {
		Context("when sending a valid request", func() {
			It("should send a Delete request to the correct URL", func() {
				// Prepare a test object
				testObj := map[string]any{
					"apiVersion": "v1",
					"kind":       "Test",
					"metadata": map[string]any{
						"name": "test-resource",
					},
				}
				obj := &types.UnstructuredObject{Content: testObj}

				// Configure the mock to return a success response
				mockClient.EXPECT().Do(mock.AnythingOfType("*http.Request")).RunAndReturn(func(req *http.Request) (*http.Response, error) {
					// Verify the request
					Expect(req.Method).To(Equal(http.MethodDelete))
					Expect(req.URL.String()).To(Equal("https://api.example.com/resources/my-group--my-team--test-resource"))

					// Create a successful response
					return &http.Response{
						StatusCode: http.StatusOK, // or StatusNoContent
						Body: io.NopCloser(bytes.NewBufferString(`{
							"apiVersion": "v1",
							"kind": "Test",
							"metadata": {"name": "test-resource"},
							"status": {"state": "deleting"}
						}`)),
						Header: make(http.Header),
					}, nil
				})

				// Call Delete with a proper object
				err := handler.Delete(testCtx, obj)

				// Verify results
				Expect(err).NotTo(HaveOccurred())

				// Verify that our mock expectations were met
				mockClient.AssertExpectations(GinkgoT())
			})
		})

		Context("when object is nil", func() {
			It("should return an error", func() {
				// Call Delete with nil object
				err := handler.Delete(testCtx, nil)

				// Verify error
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("object cannot be nil"))

				// No HTTP requests should be made
				mockClient.AssertNotCalled(GinkgoT(), "Do", mock.Anything)
			})
		})

		Context("when the API returns an error", func() {
			It("should handle API error responses", func() {
				// Prepare a test object
				testObj := map[string]any{
					"apiVersion": "v1",
					"kind":       "Test",
					"metadata": map[string]any{
						"name": "test-resource",
					},
				}
				obj := &types.UnstructuredObject{Content: testObj}

				// Configure the mock to return an error response
				mockClient.EXPECT().Do(mock.AnythingOfType("*http.Request")).RunAndReturn(func(req *http.Request) (*http.Response, error) {
					return &http.Response{
						StatusCode: http.StatusBadRequest,
						Body: io.NopCloser(bytes.NewBufferString(`{
							"type": "ValidationError",
							"status": 400,
							"title": "Delete failed",
							"detail": "Resource is in use",
							"instance": "DELETE/resources",
							"fields": []
						}`)),
						Header: make(http.Header),
					}, nil
				})

				// Call Delete
				err := handler.Delete(testCtx, obj)

				// Verify error
				Expect(err).To(HaveOccurred())
				apiErr, ok := common.AsApiError(err)
				Expect(ok).To(BeTrue())
				Expect(apiErr).NotTo(BeNil())
				Expect(apiErr.Title).To(Equal("Delete failed"))
			})
		})
	})

	Describe("Get", func() {
		Context("when sending a valid request", func() {
			It("should send a Get request to the correct URL", func() {
				// Configure the mock to return a success response
				mockClient.EXPECT().Do(mock.AnythingOfType("*http.Request")).RunAndReturn(func(req *http.Request) (*http.Response, error) {
					// Verify the request
					Expect(req.Method).To(Equal(http.MethodGet))
					Expect(req.URL.String()).To(Equal("https://api.example.com/resources/my-group--my-team--test-resource"))

					// Create a successful response
					return &http.Response{
						StatusCode: http.StatusOK,
						Body: io.NopCloser(bytes.NewBufferString(`{
							"apiVersion": "v1",
							"kind": "Test",
							"metadata": {"name": "test-resource"},
							"spec": {"foo": "bar"},
							"status": {"state": "ready"}
						}`)),
						Header: make(http.Header),
					}, nil
				})

				// Call Get
				result, err := handler.Get(testCtx, "test-resource")

				// Verify results
				Expect(err).NotTo(HaveOccurred())
				Expect(result).NotTo(BeNil())

				// With mockery, we validate requests through mock expectations
				mockClient.AssertExpectations(GinkgoT())

				// The mock expectations validate that the URL is correctly formed
			})
		})
	})

	Describe("List", func() {
		Context("when sending a valid request", func() {
			It("should send a List request to the correct URL", func() {
				// Configure the mock to return a success response
				mockClient.EXPECT().Do(mock.AnythingOfType("*http.Request")).RunAndReturn(func(req *http.Request) (*http.Response, error) {
					// Verify the request
					Expect(req.Method).To(Equal(http.MethodGet))
					Expect(req.URL.String()).To(Equal("https://api.example.com/resources"))

					// Create a successful response
					return &http.Response{
						StatusCode: http.StatusOK,
						Body: io.NopCloser(bytes.NewBufferString(`{
							"apiVersion": "v1",
							"kind": "TestList",
							"items": [
								{
									"apiVersion": "v1",
									"kind": "Test",
									"metadata": {"name": "test-resource-1"},
									"spec": {"foo": "bar1"},
									"status": {"state": "ready"}
								},
								{
									"apiVersion": "v1",
									"kind": "Test",
									"metadata": {"name": "test-resource-2"},
									"spec": {"foo": "bar2"},
									"status": {"state": "ready"}
								}
							]
						}`)),
						Header: make(http.Header),
					}, nil
				})

				// Call List
				items, err := handler.List(testCtx)

				// Verify results
				Expect(err).NotTo(HaveOccurred())
				Expect(items).NotTo(BeNil())
				Expect(items).To(HaveLen(2))

				// The URL is verified through our mock expectations
			})
		})
	})

	Describe("Status", func() {
		Context("when sending a valid request", func() {
			It("should send a Status request to the correct URL", func() {
				// Configure the mock to return a success response
				mockClient.EXPECT().Do(mock.AnythingOfType("*http.Request")).RunAndReturn(func(req *http.Request) (*http.Response, error) {
					// Verify the request
					Expect(req.Method).To(Equal(http.MethodGet))
					Expect(req.URL.String()).To(Equal("https://api.example.com/resources/my-group--my-team--test-resource/status"))

					// Create a successful response
					return &http.Response{
						StatusCode: http.StatusOK,
						Body: io.NopCloser(bytes.NewBufferString(`{
							"apiVersion": "v1",
							"kind": "Test",
							"metadata": {"name": "test-resource"},
							"status": {
								"state": "ready",
								"message": "Resource is ready"
							}
						}`)),
						Header: make(http.Header),
					}, nil
				})

				// Call Status
				status, err := handler.Status(testCtx, "test-resource")

				// Verify results
				Expect(err).NotTo(HaveOccurred())
				Expect(status).NotTo(BeNil())

				// With mockery, we validate requests through mock expectations
				mockClient.AssertExpectations(GinkgoT())

				// The mock expectations validate that the URL is correctly formed
			})
		})
	})
})
