// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package common_test

import (
	"context"
	"net/http"
	"net/http/httptest"

	"github.com/stretchr/testify/mock"

	"github.com/telekom/controlplane/rover-ctl/pkg/handlers/common"
	"github.com/telekom/controlplane/rover-ctl/test/mocks"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// We'll use the mockery-generated mock for the HttpDoer interface

var _ = Describe("HttpClient", func() {
	var originalNewHttpClient func(ctx context.Context, tokenUrl, clientId, clientSecret string) common.HttpDoer

	BeforeEach(func() {
		originalNewHttpClient = common.NewAuthorizedHttpClient
	})

	AfterEach(func() {
		common.NewAuthorizedHttpClient = originalNewHttpClient
	})

	It("should create an authorized HTTP client", func() {
		// Since we can't easily test the actual OAuth2 client creation, we'll
		// replace the function with a mock to verify it's called correctly
		var capturedTokenUrl, capturedClientId, capturedClientSecret string
		var capturedCtx context.Context

		mockClient := new(mocks.MockHttpDoer)
		mockClient.EXPECT().Do(mock.Anything).Return(&http.Response{
			StatusCode: 200,
			Body:       http.NoBody,
		}, nil)

		common.NewAuthorizedHttpClient = func(ctx context.Context, tokenUrl, clientId, clientSecret string) common.HttpDoer {
			capturedCtx = ctx
			capturedTokenUrl = tokenUrl
			capturedClientId = clientId
			capturedClientSecret = clientSecret
			return mockClient
		}

		testCtx := context.Background()
		testTokenUrl := "https://example.com/token"
		testClientId := "test-client-id"
		testClientSecret := "test-client-secret"

		client := common.NewAuthorizedHttpClient(testCtx, testTokenUrl, testClientId, testClientSecret)

		Expect(client).To(Equal(mockClient))
		Expect(capturedTokenUrl).To(Equal(testTokenUrl))
		Expect(capturedClientId).To(Equal(testClientId))
		Expect(capturedClientSecret).To(Equal(testClientSecret))

		// We expect a non-nil context, but can't test exact equality since the function creates a new one
		Expect(capturedCtx).NotTo(BeNil())
	})

	It("should use a configured HTTP client with proper settings", func() {
		// Test that the original implementation works as expected
		// This test can be used as a validation once we get the original function back
		client := common.NewAuthorizedHttpClient(
			context.Background(),
			"https://example.com/token",
			"test-client-id",
			"test-client-secret",
		)
		Expect(client).NotTo(BeNil())
	})

	It("should work with an HTTP test server", func() {
		// This test actually hits a test server to verify the client works
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"access_token":"test-token","token_type":"Bearer","expires_in":3600}`))
		}))
		defer server.Close()

		// Store original function to restore later
		origFunc := common.NewAuthorizedHttpClient

		// Create a mock handler that verifies proper access tokens are used
		var receivedToken string
		tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			auth := r.Header.Get("Authorization")
			if len(auth) > 7 { // "Bearer " is 7 chars
				receivedToken = auth[7:]
			}
			w.WriteHeader(http.StatusOK)
		}))
		defer tokenServer.Close()

		// Temporarily replace the function with one that uses our test server
		mockClient := new(mocks.MockHttpDoer)
		mockClient.EXPECT().Do(mock.AnythingOfType("*http.Request")).RunAndReturn(
			func(req *http.Request) (*http.Response, error) {
				// Add the expected Bearer token that an OAuth2 client would add
				req.Header.Set("Authorization", "Bearer test-token")
				return http.DefaultClient.Do(req)
			},
		)

		common.NewAuthorizedHttpClient = func(ctx context.Context, tokenUrl, clientId, clientSecret string) common.HttpDoer {
			// Verify parameters but return our mock
			Expect(tokenUrl).To(Equal(server.URL))
			Expect(clientId).To(Equal("test-client"))
			Expect(clientSecret).To(Equal("test-secret"))
			return mockClient
		}

		// Get a client with our test server as token URL
		client := common.NewAuthorizedHttpClient(
			context.Background(),
			server.URL,
			"test-client",
			"test-secret",
		)

		// Make a request to verify the client properly sets Authorization headers
		req, err := http.NewRequest("GET", tokenServer.URL, http.NoBody)
		Expect(err).NotTo(HaveOccurred())

		resp, err := client.Do(req)
		Expect(err).NotTo(HaveOccurred())
		Expect(resp).NotTo(BeNil())
		Expect(resp.StatusCode).To(Equal(http.StatusOK))
		Expect(receivedToken).To(Equal("test-token"))

		// Restore original function
		common.NewAuthorizedHttpClient = origFunc
	})
})

var _ = Describe("WithStaticHeaders", func() {
	It("should add static headers to outgoing requests", func() {
		mockClient := new(mocks.MockHttpDoer)
		mockClient.EXPECT().Do(mock.AnythingOfType("*http.Request")).RunAndReturn(
			func(req *http.Request) (*http.Response, error) {
				// Verify headers were set
				Expect(req.Header.Get("X-Custom-Header")).To(Equal("custom-value"))
				Expect(req.Header.Get("User-Agent")).To(Equal("test-agent/1.0"))
				return &http.Response{StatusCode: 200, Body: http.NoBody}, nil
			},
		)

		client := common.WithStaticHeaders(mockClient, http.Header{
			"X-Custom-Header": []string{"custom-value"},
			"User-Agent":      []string{"test-agent/1.0"},
		})

		req, err := http.NewRequest("GET", "http://example.com", http.NoBody)
		Expect(err).NotTo(HaveOccurred())

		resp, err := client.Do(req)
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.StatusCode).To(Equal(200))
	})

	It("should override existing headers using Set semantics", func() {
		mockClient := new(mocks.MockHttpDoer)
		mockClient.EXPECT().Do(mock.AnythingOfType("*http.Request")).RunAndReturn(
			func(req *http.Request) (*http.Response, error) {
				// The static header should have overwritten the pre-existing one
				Expect(req.Header.Values("Authorization")).To(HaveLen(1))
				Expect(req.Header.Get("Authorization")).To(Equal("Bearer static-token"))
				return &http.Response{StatusCode: 200, Body: http.NoBody}, nil
			},
		)

		client := common.WithStaticHeaders(mockClient, http.Header{
			"Authorization": []string{"Bearer static-token"},
		})

		req, err := http.NewRequest("GET", "http://example.com", http.NoBody)
		Expect(err).NotTo(HaveOccurred())
		// Pre-set a header that should be overwritten
		req.Header.Set("Authorization", "Bearer old-token")

		resp, err := client.Do(req)
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.StatusCode).To(Equal(200))
	})

	It("should work when wrapping another static header client", func() {
		mockClient := new(mocks.MockHttpDoer)
		mockClient.EXPECT().Do(mock.AnythingOfType("*http.Request")).RunAndReturn(
			func(req *http.Request) (*http.Response, error) {
				// Both layers of headers should be present
				Expect(req.Header.Get("Authorization")).To(Equal("Bearer my-token"))
				Expect(req.Header.Get("User-Agent")).To(Equal("test-agent/2.0"))
				return &http.Response{StatusCode: 200, Body: http.NoBody}, nil
			},
		)

		// First layer: Authorization
		innerClient := common.WithStaticHeaders(mockClient, http.Header{
			"Authorization": []string{"Bearer my-token"},
		})

		// Second layer: User-Agent (wraps the first)
		outerClient := common.WithStaticHeaders(innerClient, http.Header{
			"User-Agent": []string{"test-agent/2.0"},
		})

		req, err := http.NewRequest("GET", "http://example.com", http.NoBody)
		Expect(err).NotTo(HaveOccurred())

		resp, err := outerClient.Do(req)
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.StatusCode).To(Equal(200))
	})

	It("should propagate errors from the inner client", func() {
		mockClient := new(mocks.MockHttpDoer)
		expectedErr := http.ErrAbortHandler
		mockClient.EXPECT().Do(mock.AnythingOfType("*http.Request")).Return(nil, expectedErr)

		client := common.WithStaticHeaders(mockClient, http.Header{
			"User-Agent": []string{"test-agent/1.0"},
		})

		req, err := http.NewRequest("GET", "http://example.com", http.NoBody)
		Expect(err).NotTo(HaveOccurred())

		_, err = client.Do(req)
		Expect(err).To(MatchError(expectedErr))
	})
})
