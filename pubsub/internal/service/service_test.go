// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("configService", func() {
	var (
		ctx    context.Context
		server *httptest.Server
		svc    ConfigService
		origFn func(ctx context.Context, tokenUrl, clientId, clientSecret string) *http.Client
	)

	BeforeEach(func() {
		ctx = context.Background()
		// Save and override NewAuthorizedHttpClient so we don't need OAuth2
		origFn = NewAuthorizedHttpClient
	})

	AfterEach(func() {
		NewAuthorizedHttpClient = origFn
		if server != nil {
			server.Close()
		}
	})

	// Helper to create a test server and configService pointing at it.
	setupServer := func(handler http.HandlerFunc) {
		server = httptest.NewServer(handler)
		NewAuthorizedHttpClient = func(_ context.Context, _, _, _ string) *http.Client {
			return server.Client()
		}
		svc = NewConfigService(ConfigServiceConfig{
			BaseURL:      server.URL,
			TokenURL:     "https://unused.example.com/token",
			ClientID:     "test-client",
			ClientSecret: "test-secret",
		})
	}

	sampleResource := func() SubscriptionResource {
		return SubscriptionResource{
			ApiVersion: SubscriptionAPIVersion,
			Kind:       SubscriptionKind,
			Metadata: SubscriptionMetadata{
				Name:      "sub-123",
				Namespace: "default",
			},
			Spec: SubscriptionSpec{
				Environment: "test",
				Subscription: SubscriptionPayload{
					SubscriptionId: "sub-123",
					SubscriberId:   "my-app",
					PublisherId:    "publisher-app",
					Type:           "de.telekom.test.v1",
				},
			},
		}
	}

	Describe("PutSubscription", func() {
		It("should succeed with 200 OK", func() {
			setupServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				Expect(r.Method).To(Equal(http.MethodPut))
				Expect(r.URL.Path).To(Equal(pathGVR + "/sub-123"))
				Expect(r.Header.Get("Content-Type")).To(Equal("application/json"))

				body, err := io.ReadAll(r.Body)
				Expect(err).ToNot(HaveOccurred())
				var received SubscriptionResource
				Expect(json.Unmarshal(body, &received)).To(Succeed())
				Expect(received.Metadata.Name).To(Equal("sub-123"))

				w.WriteHeader(http.StatusOK)
			}))

			err := svc.PutSubscription(ctx, "sub-123", sampleResource())
			Expect(err).ToNot(HaveOccurred())
		})

		It("should succeed with 201 Created", func() {
			setupServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusCreated)
			}))

			err := svc.PutSubscription(ctx, "sub-123", sampleResource())
			Expect(err).ToNot(HaveOccurred())
		})

		It("should return error for 400 Bad Request", func() {
			setupServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte("bad request"))
			}))

			err := svc.PutSubscription(ctx, "sub-123", sampleResource())
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("PutSubscription"))
		})

		It("should return error for 500 Internal Server Error", func() {
			setupServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte("internal error"))
			}))

			err := svc.PutSubscription(ctx, "sub-123", sampleResource())
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("PutSubscription"))
		})

		It("should return error when subscriptionID is empty", func() {
			setupServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))

			err := svc.PutSubscription(ctx, "", sampleResource())
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("subscriptionID is required"))
		})
	})

	Describe("DeleteSubscription", func() {
		It("should succeed with 200 OK", func() {
			setupServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				Expect(r.Method).To(Equal(http.MethodDelete))
				Expect(r.URL.Path).To(Equal(pathGVR + "/sub-123"))

				body, err := io.ReadAll(r.Body)
				Expect(err).ToNot(HaveOccurred())
				Expect(body).ToNot(BeEmpty())

				w.WriteHeader(http.StatusOK)
			}))

			err := svc.DeleteSubscription(ctx, "sub-123", sampleResource())
			Expect(err).ToNot(HaveOccurred())
		})

		It("should succeed with 204 No Content", func() {
			setupServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusNoContent)
			}))

			err := svc.DeleteSubscription(ctx, "sub-123", sampleResource())
			Expect(err).ToNot(HaveOccurred())
		})

		It("should return error for 500 Internal Server Error", func() {
			setupServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte("server error"))
			}))

			err := svc.DeleteSubscription(ctx, "sub-123", sampleResource())
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("DeleteSubscription"))
		})

		It("should return error when subscriptionID is empty", func() {
			setupServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))

			err := svc.DeleteSubscription(ctx, "", sampleResource())
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("subscriptionID is required"))
		})
	})

	Describe("buildURL", func() {
		It("should construct correct URL", func() {
			cs := &configService{BasePath: "https://example.com"}
			u, err := cs.buildURL("my-sub-id")
			Expect(err).ToNot(HaveOccurred())
			Expect(u.String()).To(Equal("https://example.com" + pathGVR + "/my-sub-id"))
		})

		It("should return error for empty subscriptionID", func() {
			cs := &configService{BasePath: "https://example.com"}
			_, err := cs.buildURL("")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("subscriptionID is required"))
		})
	})

	Describe("checkResponse", func() {
		It("should return nil for matching status code", func() {
			recorder := httptest.NewRecorder()
			recorder.WriteHeader(http.StatusOK)
			recorder.WriteString("ok")
			resp := recorder.Result()
			defer resp.Body.Close()

			err := checkResponse(resp, "TestOp", http.StatusOK)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should return error for non-matching status code", func() {
			recorder := httptest.NewRecorder()
			recorder.WriteHeader(http.StatusBadGateway)
			recorder.WriteString("bad gateway")
			resp := recorder.Result()
			defer resp.Body.Close()

			err := checkResponse(resp, "TestOp", http.StatusOK)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("TestOp"))
		})
	})
})
