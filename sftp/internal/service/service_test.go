// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/telekom/controlplane/common/pkg/errors/ctrlerrors"
	secretsapi "github.com/telekom/controlplane/secret-manager/api"
	secretsapifake "github.com/telekom/controlplane/secret-manager/api/fake"
	sftpv1 "github.com/telekom/controlplane/sftp/api/v1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const testBasePath = "/test"

var _ = Describe("HTTPService", func() {
	It("creates or updates an SFTP user", func() {
		var received RoverSftpUserModel
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			Expect(r.Method).To(Equal(http.MethodPost))
			Expect(r.URL.Path).To(Equal(testBasePath + "/sftp-user"))
			Expect(r.Header.Get("Content-Type")).To(Equal("application/json"))
			Expect(json.NewDecoder(r.Body).Decode(&received)).To(Succeed())
			w.WriteHeader(http.StatusCreated)
		}))
		defer server.Close()

		baseURL, err := url.Parse(server.URL)
		Expect(err).NotTo(HaveOccurred())

		baseURL = baseURL.JoinPath(testBasePath)

		svc, err := NewHTTPService(Config{Endpoint: baseURL})
		Expect(err).NotTo(HaveOccurred())

		err = svc.CreateOrUpdateSFTPUser(context.Background(), RoverSftpUserModel{
			SftpUserName: "cetus--team--files",
			Description:  ptrTo("Team transfer user"),
		})

		Expect(err).NotTo(HaveOccurred())
		Expect(received.SftpUserName).To(Equal("cetus--team--files"))
		Expect(received.Description).NotTo(BeNil())
		Expect(*received.Description).To(Equal("Team transfer user"))
	})

	It("updates public keys for an SFTP user", func() {
		var received ClientPublicKeyMap
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			Expect(r.Method).To(Equal(http.MethodPost))
			Expect(r.URL.Path).To(Equal(testBasePath + "/sftp-user/cetus--team--files/keys"))
			Expect(r.URL.Query().Get("clientId")).To(Equal("client-123"))
			Expect(json.NewDecoder(r.Body).Decode(&received)).To(Succeed())
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		baseURL, err := url.Parse(server.URL)
		Expect(err).NotTo(HaveOccurred())

		baseURL = baseURL.JoinPath(testBasePath)

		svc, err := NewHTTPService(Config{Endpoint: baseURL})
		Expect(err).NotTo(HaveOccurred())

		err = svc.UpdatePublicKeysForSFTPUser(context.Background(), "cetus--team--files", "client-123", ClientPublicKeyMap{
			"client-123": {
				{
					PublicKey:    "ssh-rsa AAAAB3",
					Description:  ptrTo("build key"),
					SftpUserName: "cetus--team--files",
				},
			},
		})

		Expect(err).NotTo(HaveOccurred())
		Expect(received).To(HaveKey("client-123"))
		Expect(received["client-123"]).To(HaveLen(1))
		Expect(received["client-123"][0].Description).NotTo(BeNil())
		Expect(*received["client-123"][0].Description).To(Equal("build key"))
	})

	It("deletes an SFTP user", func() {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			Expect(r.Method).To(Equal(http.MethodDelete))
			Expect(r.URL.Path).To(Equal(testBasePath + "/sftp-user/cetus--team--files"))
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		baseURL, err := url.Parse(server.URL)
		Expect(err).NotTo(HaveOccurred())

		baseURL = baseURL.JoinPath(testBasePath)

		svc, err := NewHTTPService(Config{Endpoint: baseURL})
		Expect(err).NotTo(HaveOccurred())

		err = svc.DeleteSFTPUser(context.Background(), "cetus--team--files")

		Expect(err).NotTo(HaveOccurred())
	})

	It("maps client errors to blocked errors", func() {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"title":"bad request","detail":"invalid user"}`))
		}))
		defer server.Close()

		baseURL, err := url.Parse(server.URL)
		Expect(err).NotTo(HaveOccurred())

		baseURL = baseURL.JoinPath(testBasePath)

		svc, err := NewHTTPService(Config{Endpoint: baseURL})
		Expect(err).NotTo(HaveOccurred())

		err = svc.DeleteSFTPUser(context.Background(), "bad-user")

		var blocked ctrlerrors.BlockedError
		Expect(errors.As(err, &blocked)).To(BeTrue())
		Expect(err.Error()).To(ContainSubstring("bad request"))
		Expect(err.Error()).To(ContainSubstring("invalid user"))
	})

	It("parses API errors with non-RFC3339 timestamps", func() {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"title":"bad request","detail":"invalid user","timestamp":"2026-06-18T11:15:02.096673893","status":400}`))
		}))
		defer server.Close()

		baseURL, err := url.Parse(server.URL)
		Expect(err).NotTo(HaveOccurred())

		baseURL = baseURL.JoinPath(testBasePath)

		svc, err := NewHTTPService(Config{Endpoint: baseURL})
		Expect(err).NotTo(HaveOccurred())

		err = svc.DeleteSFTPUser(context.Background(), "bad-user")

		var blocked ctrlerrors.BlockedError
		Expect(errors.As(err, &blocked)).To(BeTrue())
		Expect(err.Error()).To(ContainSubstring("bad request"))
		Expect(err.Error()).To(ContainSubstring("invalid user"))
	})
})

var _ = Describe("HTTPServiceFactory", func() {
	It("reuses an SFTPServiceConfig client with a valid OAuth2 token", func() {
		var tokenRequests int32
		tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			atomic.AddInt32(&tokenRequests, 1)
			Expect(r.Method).To(Equal(http.MethodPost))
			Expect(r.ParseForm()).To(Succeed())
			Expect(r.Form.Get("grant_type")).To(Equal("client_credentials"))

			clientID, clientSecret, ok := r.BasicAuth()
			Expect(ok).To(BeTrue())
			Expect(clientID).To(Equal("zone-client"))
			Expect(clientSecret).To(Equal("resolved-secret"))

			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"access_token":"zone-token","token_type":"Bearer","expires_in":3600}`))
		}))
		defer tokenServer.Close()

		apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			Expect(r.Method).To(Equal(http.MethodPost))
			Expect(r.URL.Path).To(Equal(testBasePath + "/sftp-user"))
			Expect(r.Header.Get("Authorization")).To(Equal("Bearer zone-token"))
			w.WriteHeader(http.StatusCreated)
		}))
		defer apiServer.Close()

		apiBaseURL, err := url.Parse(apiServer.URL)
		Expect(err).NotTo(HaveOccurred())

		apiBaseURL = apiBaseURL.JoinPath(testBasePath)
		secretRef := secretsapi.ToRef("sftp/cetus/client-secret")

		sftpServiceConfig := &sftpv1.SFTPServiceConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "cetus",
				Namespace: "controlplane-system",
			},
			Spec: sftpv1.SFTPServiceConfigSpec{
				API: sftpv1.APIEndpoint{
					ClientID:     "zone-client",
					ClientSecret: secretRef,
					Endpoint:     apiBaseURL.String(),
					Issuer:       tokenServer.URL,
				},
			},
		}

		ctx := context.Background()
		secretManager := secretsapifake.NewMockSecretManager(GinkgoT())
		secretManager.EXPECT().Get(ctx, secretRef).Return("resolved-secret", nil).Once()

		originalAPI := secretsapi.API
		DeferCleanup(func() {
			secretsapi.API = originalAPI
		})
		secretsapi.API = func() secretsapi.SecretManager {
			return secretManager
		}

		factory := NewHTTPServiceFactory()

		cached := factory.IsServiceCached(client.ObjectKeyFromObject(sftpServiceConfig))
		Expect(cached).To(BeFalse())

		Expect(factory.CreateOrUpdate(ctx, sftpServiceConfig)).To(Succeed())

		cached = factory.IsServiceCached(client.ObjectKeyFromObject(sftpServiceConfig))
		Expect(cached).To(BeTrue())

		svc, err := factory.ServiceFor(ctx, client.ObjectKeyFromObject(sftpServiceConfig))
		Expect(err).NotTo(HaveOccurred())

		err = svc.CreateOrUpdateSFTPUser(ctx, RoverSftpUserModel{
			SftpUserName: "cetus--team--files",
		})
		Expect(err).NotTo(HaveOccurred())

		Expect(factory.CreateOrUpdate(ctx, sftpServiceConfig)).To(Succeed())

		svc, err = factory.ServiceFor(ctx, client.ObjectKeyFromObject(sftpServiceConfig))
		Expect(err).NotTo(HaveOccurred())

		err = svc.CreateOrUpdateSFTPUser(ctx, RoverSftpUserModel{
			SftpUserName: "cetus--team--files",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(atomic.LoadInt32(&tokenRequests)).To(Equal(int32(2)))

		factory.Delete(sftpServiceConfig)

		cached = factory.IsServiceCached(client.ObjectKeyFromObject(sftpServiceConfig))
		Expect(cached).To(BeFalse())
	})
})

func ptrTo[T any](value T) *T {
	return &value
}
