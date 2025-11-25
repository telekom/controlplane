// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package k8s_test

import (
	"net/http"
	"net/http/httptest"

	jwtware "github.com/gofiber/contrib/jwt"
	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/telekom/controlplane/common-server/pkg/server"
	k8s "github.com/telekom/controlplane/common-server/pkg/server/middleware/kubernetes"
)

// Test constants
const (
	// JWT and authentication constants
	testAudience      = "test-audience"
	testWrongAudience = "wrong-audience"
	testSecretKey     = "secret"

	// Service account constants
	testAllowedSA      = "allowed-sa"
	testUnknownSA      = "unknown-sa"
	testNamespace      = "test-ns"
	testDeployName     = "test-deploy"
	testPodName        = "test-deploy-pod"
	testInvalidPodName = "wrong-deploy-pod"

	// API endpoints
	testEndpoint = "/foo"
)

// Helper function to create a JWT token with Kubernetes service account claims
// Used in multiple test scenarios to create valid and invalid tokens
func createServiceAccountToken(audience []string, serviceAccountName, namespace, podName string) string {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, &k8s.ServiceAccountTokenClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Audience: audience,
		},
		Kubernetes: k8s.Kubernetes{
			Namespace: namespace,
			ServiceAccount: k8s.NamedObject{
				Name: serviceAccountName,
			},
			Pod: k8s.NamedObject{
				Name: podName,
			},
		},
	})

	// Sign the token with a dummy key (not actually validated in tests)
	tokenString, _ := token.SignedString([]byte(testSecretKey))
	return tokenString
}

var _ = Describe("Kubernetes Authentication Middleware", func() {

	Describe("IsReadOnly", func() {
		DescribeTable("should correctly identify read-only and write methods",
			func(method string, expectedReadOnly bool) {
				Expect(k8s.IsReadOnly(method)).To(Equal(expectedReadOnly))
			},
			// Read-only methods
			Entry("GET method", fiber.MethodGet, true),
			Entry("HEAD method", fiber.MethodHead, true),
			Entry("OPTIONS method", fiber.MethodOptions, true),

			// Write methods
			Entry("POST method", fiber.MethodPost, false),
			Entry("PUT method", fiber.MethodPut, false),
			Entry("DELETE method", fiber.MethodDelete, false),
			Entry("PATCH method", fiber.MethodPatch, false),
		)
	})

	Describe("AccessTypeSet", func() {
		DescribeTable("should correctly determine if AccessType is in the set",
			func(set k8s.AccessTypeSet, accessType k8s.AccessType, expectedResult bool) {
				Expect(set.Has(accessType)).To(Equal(expectedResult))
			},
			// Empty set
			Entry("Empty set with Read", k8s.AccessTypeSet{}, k8s.AccessTypeRead, false),
			Entry("Empty set with Write", k8s.AccessTypeSet{}, k8s.AccessTypeWrite, false),
			Entry("Empty set with None", k8s.AccessTypeSet{}, k8s.AccessTypeNone, false),

			// Read-only set
			Entry("Read-only set with Read", k8s.AccessTypeSet{k8s.AccessTypeRead: struct{}{}}, k8s.AccessTypeRead, true),
			Entry("Read-only set with Write", k8s.AccessTypeSet{k8s.AccessTypeRead: struct{}{}}, k8s.AccessTypeWrite, false),
			Entry("Read-only set with None", k8s.AccessTypeSet{k8s.AccessTypeRead: struct{}{}}, k8s.AccessTypeNone, false),

			// Mixed set
			Entry("Mixed set with Read",
				k8s.AccessTypeSet{k8s.AccessTypeRead: struct{}{}, k8s.AccessTypeWrite: struct{}{}},
				k8s.AccessTypeRead, true),
			Entry("Mixed set with Write",
				k8s.AccessTypeSet{k8s.AccessTypeRead: struct{}{}, k8s.AccessTypeWrite: struct{}{}},
				k8s.AccessTypeWrite, true),
			Entry("Mixed set with None",
				k8s.AccessTypeSet{k8s.AccessTypeRead: struct{}{}, k8s.AccessTypeWrite: struct{}{}},
				k8s.AccessTypeNone, false),
		)
	})

	Describe("NewSuccessHandler", func() {
		var (
			options    *k8s.KubernetesAuthzOptions
			handler    fiber.Handler
			nextCalled bool
			app        *fiber.App
		)

		// Helper function to configure and set up the Fiber app with necessary middleware
		setupTestApp := func(accessConfig []k8s.ServiceAccessConfig) {
			// Reset nextCalled flag before each test
			nextCalled = false

			options = &k8s.KubernetesAuthzOptions{
				Audience:     testAudience,
				AccessConfig: accessConfig,
			}

			app = server.NewApp()

			// We need both JWT middleware and our handler for complete tests
			jwtMiddleware := jwtware.New(jwtware.Config{
				SigningKey: jwtware.SigningKey{Key: []byte(testSecretKey)},
				Claims:     &k8s.ServiceAccountTokenClaims{},
			})

			handler = k8s.NewSuccessHandler(options)

			// First parse the JWT, then apply our handler
			app.Use(jwtMiddleware)
			app.Use(handler)

			nextHandler := func(c *fiber.Ctx) error {
				nextCalled = true
				return c.SendStatus(fiber.StatusOK)
			}
			app.Get(testEndpoint, nextHandler)
			app.Post(testEndpoint, nextHandler)
		}

		BeforeEach(func() {
			// Initialize with default empty access config
			setupTestApp([]k8s.ServiceAccessConfig{})
		})

		// Note: Using the global createServiceAccountToken helper function

		// Helper function to create and test a request
		testRequest := func(method, path string, token string) *http.Response {
			req := httptest.NewRequest(method, path, nil)
			if token != "" {
				req.Header.Set("Authorization", "Bearer "+token)
			}

			// Reset the nextCalled flag before each test
			nextCalled = false

			res, err := app.Test(req)
			Expect(err).To(BeNil())
			return res
		}

		// Helper to get a valid token for testing
		getValidToken := func() string {
			return createServiceAccountToken(
				[]string{testAudience},
				testAllowedSA,
				testNamespace,
				testPodName,
			)
		}

		Context("Authentication tests", func() {
			It("should deny access when no JWT token is provided", func() {
				res := testRequest(http.MethodGet, testEndpoint, "")
				Expect(res.StatusCode).To(Equal(http.StatusBadRequest))
				Expect(nextCalled).To(BeFalse())
			})

			It("should deny access when JWT token has invalid claims", func() {
				res := testRequest(http.MethodGet, testEndpoint, "e30=.eyJzdWIiOiIxMjM0NTY3ODkwIiwibmFtZSI6IkpvaG4gRG9lIiwiaWF0IjoxNTE2MjM5MDIyfQ.e30=")
				Expect(res.StatusCode).To(Equal(http.StatusUnauthorized))
				Expect(nextCalled).To(BeFalse())
			})

			It("should deny access when token has invalid audience", func() {
				tokenString := createServiceAccountToken(
					[]string{testWrongAudience}, // Wrong audience
					"test-sa",
					testNamespace,
					"test-pod",
				)

				res := testRequest(http.MethodGet, testEndpoint, tokenString)
				Expect(res.StatusCode).To(Equal(http.StatusForbidden)) // Should be forbidden
				Expect(nextCalled).To(BeFalse())
			})
		})

		Context("Authorization tests with read-only access", func() {
			BeforeEach(func() {
				// Configure access config with read access for specific service account
				setupTestApp([]k8s.ServiceAccessConfig{
					{
						ServiceAccountName: testAllowedSA,
						Namespace:          testNamespace,
						DeploymentName:     testDeployName,
						AllowedAccess:      []k8s.AccessType{k8s.AccessTypeRead},
					},
				})
			})

			It("should deny access when token has unknown service account", func() {
				tokenString := createServiceAccountToken(
					[]string{testAudience}, // Valid audience
					testUnknownSA,          // Unknown service account
					testNamespace,
					testPodName,
				)

				res := testRequest(http.MethodGet, testEndpoint, tokenString)
				Expect(res.StatusCode).To(Equal(http.StatusForbidden))
				Expect(nextCalled).To(BeFalse())
			})

			It("should deny access when pod name doesn't match deployment name", func() {
				tokenString := createServiceAccountToken(
					[]string{testAudience},
					testAllowedSA,
					testNamespace,
					testInvalidPodName, // Pod name doesn't match deployment name prefix
				)

				res := testRequest(http.MethodGet, testEndpoint, tokenString)
				Expect(res.StatusCode).To(Equal(http.StatusForbidden))
				Expect(nextCalled).To(BeFalse())
			})

			It("should deny write request with read-only permissions", func() {
				// Send POST request (write operation) with a valid token
				res := testRequest(http.MethodPost, testEndpoint, getValidToken())
				Expect(res.StatusCode).To(Equal(http.StatusForbidden))
				Expect(nextCalled).To(BeFalse())
			})

			It("should allow access for read request with read permissions", func() {
				// Send GET request (read operation) with a valid token
				res := testRequest(http.MethodGet, testEndpoint, getValidToken())
				Expect(res.StatusCode).To(Equal(http.StatusOK))
				Expect(nextCalled).To(BeTrue())
			})

			It("should allow access for all read-only methods with read permissions", func() {
				// Test all read-only methods
				methods := []string{http.MethodGet, http.MethodHead, http.MethodOptions}

				for _, method := range methods {
					res := testRequest(method, testEndpoint, getValidToken())
					// Note: HEAD and OPTIONS might return different status codes depending on the app
					// The important thing is that they don't return a forbidden status
					Expect(res.StatusCode).NotTo(Equal(http.StatusForbidden))
				}
			})
		})

		Context("Authorization tests with read-write access", func() {
			BeforeEach(func() {
				// Configure access config with both read and write access
				setupTestApp([]k8s.ServiceAccessConfig{
					{
						ServiceAccountName: testAllowedSA,
						Namespace:          testNamespace,
						DeploymentName:     testDeployName,
						AllowedAccess:      []k8s.AccessType{k8s.AccessTypeRead, k8s.AccessTypeWrite},
					},
				})
			})

			It("should allow both read and write access with appropriate permissions", func() {
				// Test both read and write methods
				methods := []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodPatch}

				for _, method := range methods {
					res := testRequest(method, testEndpoint, getValidToken())
					Expect(res.StatusCode).NotTo(Equal(http.StatusForbidden))
				}
			})
		})

		Context("Authorization tests with multiple access configs", func() {
			BeforeEach(func() {
				// Configure access config with multiple service accounts
				setupTestApp([]k8s.ServiceAccessConfig{
					{
						// This one should NOT match our test token
						ServiceAccountName: testUnknownSA,
						Namespace:          testNamespace,
						DeploymentName:     testDeployName,
						AllowedAccess:      []k8s.AccessType{k8s.AccessTypeRead, k8s.AccessTypeWrite},
					},
					{
						// This one SHOULD match our test token, but with read-only access
						ServiceAccountName: testAllowedSA,
						Namespace:          testNamespace,
						DeploymentName:     testDeployName,
						AllowedAccess:      []k8s.AccessType{k8s.AccessTypeRead},
					},
				})
			})

			It("should correctly select the matching service account configuration", func() {
				// Should allow read access (matching second config)
				resRead := testRequest(http.MethodGet, testEndpoint, getValidToken())
				Expect(resRead.StatusCode).To(Equal(http.StatusOK))
				Expect(nextCalled).To(BeTrue())

				// Should deny write access (matching second config which is read-only)
				resWrite := testRequest(http.MethodPost, testEndpoint, getValidToken())
				Expect(resWrite.StatusCode).To(Equal(http.StatusForbidden))
				Expect(nextCalled).To(BeFalse())
			})
		})

		Context("Authorization tests with empty access types", func() {
			BeforeEach(func() {
				// Configure access config with empty access types (should deny all access)
				setupTestApp([]k8s.ServiceAccessConfig{
					{
						ServiceAccountName: testAllowedSA,
						Namespace:          testNamespace,
						DeploymentName:     testDeployName,
						AllowedAccess:      []k8s.AccessType{}, // Empty access types
					},
				})
			})

			It("should deny all access when access types are empty", func() {
				// Should deny read access with empty access types
				resRead := testRequest(http.MethodGet, testEndpoint, getValidToken())
				Expect(resRead.StatusCode).To(Equal(http.StatusForbidden))
				Expect(nextCalled).To(BeFalse())

				// Should deny write access with empty access types
				resWrite := testRequest(http.MethodPost, testEndpoint, getValidToken())
				Expect(resWrite.StatusCode).To(Equal(http.StatusForbidden))
				Expect(nextCalled).To(BeFalse())
			})
		})

		Context("Authorization tests with AccessTypeNone", func() {
			BeforeEach(func() {
				// Configure access config with AccessTypeNone (should deny all access)
				setupTestApp([]k8s.ServiceAccessConfig{
					{
						ServiceAccountName: testAllowedSA,
						Namespace:          testNamespace,
						DeploymentName:     testDeployName,
						AllowedAccess:      []k8s.AccessType{k8s.AccessTypeNone},
					},
				})
			})

			It("should deny all access with AccessTypeNone", func() {
				// Should deny read access with AccessTypeNone
				resRead := testRequest(http.MethodGet, testEndpoint, getValidToken())
				Expect(resRead.StatusCode).To(Equal(http.StatusForbidden))
				Expect(nextCalled).To(BeFalse())

				// Should deny write access with AccessTypeNone
				resWrite := testRequest(http.MethodPost, testEndpoint, getValidToken())
				Expect(resWrite.StatusCode).To(Equal(http.StatusForbidden))
				Expect(nextCalled).To(BeFalse())
			})
		})
	})
})
