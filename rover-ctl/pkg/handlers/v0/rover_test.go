// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v0_test

import (
	"context"
	"io"
	"net/http"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/mock"
	"github.com/telekom/controlplane/rover-ctl/pkg/config"
	"github.com/telekom/controlplane/rover-ctl/pkg/handlers/common"
	v0 "github.com/telekom/controlplane/rover-ctl/pkg/handlers/v0"
	"github.com/telekom/controlplane/rover-ctl/pkg/types"
	"github.com/telekom/controlplane/rover-ctl/test/mocks"
)

var _ = Describe("Rover Handler", func() {
	var (
		mockClient         *mocks.MockHttpDoer
		originalHttpClient func(ctx context.Context, tokenUrl, clientId, clientSecret string) common.HttpDoer
		testCtx            context.Context
	)

	BeforeEach(func() {
		// Save the original HTTP client function
		originalHttpClient = common.NewAuthorizedHttpClient

		// Create a mock client
		mockClient = mocks.NewMockHttpDoer(GinkgoT())

		// Override the HTTP client function
		common.NewAuthorizedHttpClient = func(ctx context.Context, tokenUrl, clientId, clientSecret string) common.HttpDoer {
			return mockClient
		}

		// Create a test context with token
		token := &config.Token{
			ClientId:     "test-client",
			ClientSecret: "test-secret",
			TokenUrl:     "https://example.com/token",
			ServerUrl:    "https://api.example.com",
			Group:        "test-group",
			Team:         "test-team",
		}
		testCtx = config.NewContext(context.Background(), token)
	})

	AfterEach(func() {
		// Restore the original HTTP client function
		common.NewAuthorizedHttpClient = originalHttpClient
	})

	Describe("NewRoverHandlerInstance", func() {
		It("should return a properly configured handler", func() {
			// Create a rover handler
			handler := v0.NewRoverHandlerInstance()

			// Verify the handler properties
			Expect(handler).NotTo(BeNil())
			Expect(handler.BaseHandler).NotTo(BeNil())

			// Verify API values
			baseHandler := handler.BaseHandler
			Expect(baseHandler.APIVersion).To(Equal("tcp.ei.telekom.de/v1"))
			Expect(baseHandler.Kind).To(Equal("Rover"))
			Expect(baseHandler.Resource).To(Equal("rovers"))
			Expect(baseHandler.Priority()).To(Equal(100))

			// Verify info is supported
			Expect(baseHandler.SupportsInfo).To(BeTrue())
		})
	})

	Describe("PatchRoverRequest", func() {
		Context("when processing valid rover spec", func() {
			It("should handle empty exposures", func() {
				// Create a test object with empty exposures
				obj := &types.UnstructuredObject{
					Content: map[string]any{
						"spec": map[string]any{
							"exposures": nil,
						},
					},
				}

				// Apply patch
				err := v0.PatchRoverRequest(context.Background(), obj)

				// Verify no error
				Expect(err).NotTo(HaveOccurred())

				// Verify exposures field was removed
				content := obj.GetContent()
				spec, _ := content["spec"].(map[string]any)
				Expect(spec).NotTo(HaveKey("exposures"))
			})

			It("should handle empty subscriptions", func() {
				// Create a test object with empty subscriptions
				obj := &types.UnstructuredObject{
					Content: map[string]any{
						"spec": map[string]any{
							"subscriptions": nil,
						},
					},
				}

				// Apply patch
				err := v0.PatchRoverRequest(context.Background(), obj)

				// Verify no error
				Expect(err).NotTo(HaveOccurred())

				// Verify subscriptions field was removed
				content := obj.GetContent()
				spec, _ := content["spec"].(map[string]any)
				Expect(spec).NotTo(HaveKey("subscriptions"))
			})

			It("should patch API exposures", func() {
				// Create a test object with API exposures
				obj := &types.UnstructuredObject{
					Content: map[string]any{
						"spec": map[string]any{
							"exposures": []any{
								map[string]any{
									"basePath": "/api/v1",
									"security": map[string]any{
										"oauth2": map[string]any{
											"tokenEndpoint": "https://example.com/token",
										},
									},
								},
							},
						},
					},
				}

				// Apply patch
				err := v0.PatchRoverRequest(context.Background(), obj)

				// Verify no error
				Expect(err).NotTo(HaveOccurred())

				// Verify exposures were patched
				content := obj.GetContent()
				Expect(content).To(HaveKey("exposures"))

				exposures := content["exposures"].([]map[string]any)
				Expect(exposures).To(HaveLen(1))

				// Verify type was added
				exposure := exposures[0]
				Expect(exposure).To(HaveKeyWithValue("type", "api"))

				// Verify security type was added
				security := exposure["security"].(map[string]any)
				Expect(security).To(HaveKeyWithValue("type", "oauth2"))
			})

			It("should patch event exposures", func() {
				// Create a test object with event exposures
				obj := &types.UnstructuredObject{
					Content: map[string]any{
						"spec": map[string]any{
							"exposures": []any{
								map[string]any{
									"eventType": "test.event",
									"security": map[string]any{
										"basicAuth": map[string]any{
											"username": "testuser",
										},
									},
								},
							},
						},
					},
				}

				// Apply patch
				err := v0.PatchRoverRequest(context.Background(), obj)

				// Verify no error
				Expect(err).NotTo(HaveOccurred())

				// Verify exposures were patched
				content := obj.GetContent()
				Expect(content).To(HaveKey("exposures"))

				exposures := content["exposures"].([]map[string]any)
				Expect(exposures).To(HaveLen(1))

				// Verify type was added
				exposure := exposures[0]
				Expect(exposure).To(HaveKeyWithValue("type", "event"))

				// Verify security type was added
				security := exposure["security"].(map[string]any)
				Expect(security).To(HaveKeyWithValue("type", "basicAuth"))
			})
		})

		Context("when processing invalid rover spec", func() {
			It("should return error for invalid spec", func() {
				// Create a test object with invalid spec
				obj := &types.UnstructuredObject{
					Content: map[string]any{
						"spec": "not a map",
					},
				}

				// Apply patch
				err := v0.PatchRoverRequest(context.Background(), obj)

				// Verify error
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("invalid spec"))
			})

			It("should return error for invalid exposures format", func() {
				// Create a test object with invalid exposures
				obj := &types.UnstructuredObject{
					Content: map[string]any{
						"spec": map[string]any{
							"exposures": "not an array",
						},
					},
				}

				// Apply patch
				err := v0.PatchRoverRequest(context.Background(), obj)

				// Verify error
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("invalid exposures format"))
			})

			It("should return error for invalid subscriptions format", func() {
				// Create a test object with invalid subscriptions
				obj := &types.UnstructuredObject{
					Content: map[string]any{
						"spec": map[string]any{
							"subscriptions": "not an array",
						},
					},
				}

				// Apply patch
				err := v0.PatchRoverRequest(context.Background(), obj)

				// Verify error
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("invalid subscriptions format"))
			})
		})
	})

	Describe("PatchSubscriptions", func() {
		Context("when patching subscriptions", func() {
			It("should patch API subscriptions correctly", func() {
				// Create test subscriptions
				subscriptions := []any{
					map[string]any{
						"basePath": "/api/v1",
						"security": map[string]any{
							"oauth2": map[string]any{
								"tokenEndpoint": "https://example.com/token",
							},
						},
					},
				}

				// Patch the subscriptions
				result := v0.PatchSubscriptions(subscriptions)

				// Verify the results
				Expect(result).To(HaveLen(1))
				Expect(result[0]).To(HaveKeyWithValue("type", "api"))
				Expect(result[0]["security"]).To(HaveKeyWithValue("type", "oauth2"))
			})

			It("should patch port subscriptions correctly", func() {
				// Create test subscriptions
				subscriptions := []any{
					map[string]any{
						"port": 8080,
						"security": map[string]any{
							"basicAuth": map[string]any{
								"username": "testuser",
							},
						},
					},
				}

				// Patch the subscriptions
				result := v0.PatchSubscriptions(subscriptions)

				// Verify the results
				Expect(result).To(HaveLen(1))
				Expect(result[0]).To(HaveKeyWithValue("type", "port"))
				Expect(result[0]["security"]).To(HaveKeyWithValue("type", "basicAuth"))
			})

			It("should handle nil subscriptions", func() {
				// Test with nil
				result := v0.PatchSubscriptions(nil)
				Expect(result).To(BeNil())
			})

			It("should handle empty subscriptions array", func() {
				// Test with empty array
				result := v0.PatchSubscriptions([]any{})
				Expect(result).To(BeNil())
			})

			It("should skip subscriptions that are not maps", func() {
				// Create test subscriptions with a non-map entry
				subscriptions := []any{
					"not a map",
					map[string]any{
						"port": 8080,
					},
				}

				// Patch the subscriptions
				result := v0.PatchSubscriptions(subscriptions)

				// Verify the results - should only have the valid entry
				Expect(result).To(HaveLen(2))
				Expect(result[0]).To(BeNil())
				Expect(result[1]).To(HaveKeyWithValue("type", "port"))
			})
		})
	})

	Describe("PatchSecurity", func() {
		Context("when patching security objects", func() {
			It("should add oauth2 type", func() {
				security := map[string]any{
					"oauth2": map[string]any{
						"tokenEndpoint": "https://example.com/token",
					},
				}

				v0.PatchSecurity(security)

				Expect(security).To(HaveKeyWithValue("type", "oauth2"))
				Expect(security).To(HaveKeyWithValue("tokenEndpoint", "https://example.com/token"))
				Expect(security).NotTo(HaveKey("oauth2"))
			})

			It("should add basicAuth type", func() {
				security := map[string]any{
					"basicAuth": map[string]any{
						"username": "testuser",
					},
				}

				v0.PatchSecurity(security)

				Expect(security).To(HaveKeyWithValue("type", "basicAuth"))
				Expect(security).To(HaveKeyWithValue("username", "testuser"))
				Expect(security).NotTo(HaveKey("basicAuth"))
			})

			It("should handle nil security", func() {
				// Should not panic
				v0.PatchSecurity(nil)
			})

			It("should handle non-map security", func() {
				// Should not panic
				v0.PatchSecurity("not a map")
			})
		})
	})

	Describe("ResetSecret", func() {
		It("should send a reset secret request and return new credentials", func() {
			// Configure mock to return successful response
			mockClient.EXPECT().Do(mock.AnythingOfType("*http.Request")).RunAndReturn(func(req *http.Request) (*http.Response, error) {
				// Verify request details
				Expect(req.Method).To(Equal(http.MethodPatch))
				Expect(req.URL.String()).To(Equal("https://api.example.com/rovers/test-group--test-team--test-rover/secret"))

				// Return a successful response with credentials
				responseBody := `{"clientId":"new-client-id","secret":"new-secret-value"}`
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader(responseBody)),
					Header:     make(http.Header),
				}, nil
			})

			// Create the rover handler
			handler := v0.NewRoverHandlerInstance()
			handler.Setup(testCtx)

			// Call ResetSecret
			clientId, clientSecret, err := handler.ResetSecret(testCtx, "test-rover")

			// Verify the results
			Expect(err).NotTo(HaveOccurred())
			Expect(clientId).To(Equal("new-client-id"))
			Expect(clientSecret).To(Equal("new-secret-value"))

			// Verify mock expectations
			mockClient.AssertExpectations(GinkgoT())
		})

		It("should handle errors from the API", func() {
			// Configure mock to return error response
			mockClient.EXPECT().Do(mock.AnythingOfType("*http.Request")).RunAndReturn(func(req *http.Request) (*http.Response, error) {
				// Return an error response
				responseBody := `{"type":"ValidationError","status":400,"title":"Reset Failed","detail":"Invalid rover name","instance":"PATCH/rovers/secret"}`
				return &http.Response{
					StatusCode: http.StatusBadRequest,
					Body:       io.NopCloser(strings.NewReader(responseBody)),
					Header:     make(http.Header),
				}, nil
			})

			// Create the rover handler
			handler := v0.NewRoverHandlerInstance()
			handler.Setup(testCtx)

			// Call ResetSecret
			_, _, err := handler.ResetSecret(testCtx, "invalid-rover")

			// Verify error
			Expect(err).To(HaveOccurred())
			apiErr, ok := common.AsApiError(err)
			Expect(ok).To(BeTrue())
			Expect(apiErr.Title).To(Equal("Reset Failed"))

			// Verify mock expectations
			mockClient.AssertExpectations(GinkgoT())
		})
	})
})
