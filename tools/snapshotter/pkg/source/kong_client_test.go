// SPDX-FileCopyrightText: 2026 Deutsche Telekom AG
//
// SPDX-License-Identifier: Apache-2.0

package source_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/spf13/viper"

	"github.com/telekom/controlplane/tools/snapshotter/pkg/config"
	"github.com/telekom/controlplane/tools/snapshotter/pkg/source"
)

var _ = Describe("Snapshotter TLS configuration", func() {
	It("loads the opt-in per source and defaults to certificate verification", func() {
		viper.Reset()
		DeferCleanup(viper.Reset)
		path := filepath.Join(GinkgoT().TempDir(), "config.yaml")
		Expect(os.WriteFile(path, []byte(`storePath: /tmp/store
sources:
  secure:
    environment: env
    zone: zone
    url: https://gateway.example.com/admin
    tokenUrl: https://idp.example.com/realms/test
    clientId: client
    clientSecret: secret
  insecure:
    environment: env
    zone: zone
    url: https://gateway.example.com/admin
    tokenUrl: https://idp.example.com/realms/test
    clientId: client
    clientSecret: secret
    insecureSkipTLSVerify: true
`), 0o600)).To(Succeed())
		cfg, err := config.LoadConfig(path)
		Expect(err).NotTo(HaveOccurred())
		secure, ok := cfg.GetSourceConfig("secure")
		Expect(ok).To(BeTrue())
		Expect(secure.InsecureSkipTLSVerify).To(BeFalse())
		insecure, ok := cfg.GetSourceConfig("insecure")
		Expect(ok).To(BeTrue())
		Expect(insecure.InsecureSkipTLSVerify).To(BeTrue())
	})
})

var _ = Describe("Kong client TLS verification", func() {
	var (
		cfg           config.SourceConfig
		tokenRequests atomic.Int32
		apiRequests   atomic.Int32
	)

	BeforeEach(func() {
		tokenRequests.Store(0)
		apiRequests.Store(0)
		mux := http.NewServeMux()
		mux.HandleFunc("/realms/test/protocol/openid-connect/token", func(w http.ResponseWriter, r *http.Request) {
			defer GinkgoRecover()
			tokenRequests.Add(1)
			w.Header().Set("Content-Type", "application/json")
			Expect(json.NewEncoder(w).Encode(map[string]any{
				"access_token": "test-token", "token_type": "Bearer", "expires_in": 3600,
			})).To(Succeed())
		})
		mux.HandleFunc("/admin/routes/my-route", func(w http.ResponseWriter, r *http.Request) {
			defer GinkgoRecover()
			apiRequests.Add(1)
			Expect(r.Header.Get("Authorization")).To(Equal("Bearer test-token"))
			w.Header().Set("Content-Type", "application/json")
			Expect(json.NewEncoder(w).Encode(map[string]string{"id": "route-id", "name": "my-route"})).To(Succeed())
		})
		server := httptest.NewTLSServer(mux)
		DeferCleanup(server.Close)
		cfg = config.SourceConfig{
			Environment: "env", Zone: "zone",
			Url: server.URL + "/admin", TokenUrl: server.URL + "/realms/test",
			ClientId: "client", ClientSecret: "secret",
		}
	})

	getRoute := func() error {
		client, err := source.NewKongClient(cfg)
		Expect(err).NotTo(HaveOccurred())
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, err = client.GetRouteWithResponse(ctx, "my-route")
		return err
	}

	It("rejects untrusted certificates by default", func() {
		Expect(getRoute()).To(MatchError(ContainSubstring("certificate")))
		Expect(tokenRequests.Load()).To(BeZero())
		Expect(apiRequests.Load()).To(BeZero())
	})

	It("skips verification for both endpoints only on the opted-in source", func() {
		cfg.InsecureSkipTLSVerify = true
		Expect(getRoute()).To(Succeed())
		Expect(tokenRequests.Load()).To(BeEquivalentTo(1))
		Expect(apiRequests.Load()).To(BeEquivalentTo(1))

		By("leaving verification enabled for another client")
		cfg.InsecureSkipTLSVerify = false
		Expect(getRoute()).To(MatchError(ContainSubstring("certificate")))
		Expect(tokenRequests.Load()).To(BeEquivalentTo(1))
		Expect(apiRequests.Load()).To(BeEquivalentTo(1))
	})
})

var _ = Describe("Kong client", func() {
	var (
		cfg           config.SourceConfig
		tokenRequests atomic.Int32
		apiRequests   atomic.Int32
	)

	BeforeEach(func() {
		tokenRequests.Store(0)
		apiRequests.Store(0)
		mux := http.NewServeMux()
		mux.HandleFunc("/realms/test/protocol/openid-connect/token", func(w http.ResponseWriter, r *http.Request) {
			defer GinkgoRecover()
			tokenRequests.Add(1)
			Expect(r.Method).To(Equal(http.MethodPost))
			Expect(r.ParseForm()).To(Succeed())
			Expect(r.PostForm.Get("grant_type")).To(Equal("client_credentials"))
			id, secret, ok := r.BasicAuth()
			Expect(ok).To(BeTrue())
			Expect(id).To(Equal("client"))
			Expect(secret).To(Equal("secret"))
			w.Header().Set("Content-Type", "application/json")
			Expect(json.NewEncoder(w).Encode(map[string]any{
				"access_token": "test-token", "token_type": "Bearer", "expires_in": 3600,
			})).To(Succeed())
		})
		mux.HandleFunc("/admin/routes/my-route", func(w http.ResponseWriter, r *http.Request) {
			defer GinkgoRecover()
			apiRequests.Add(1)
			Expect(r.Header.Get("Authorization")).To(Equal("Bearer test-token"))
			w.Header().Set("Content-Type", "application/json")
			Expect(json.NewEncoder(w).Encode(map[string]string{"id": "route-id", "name": "my-route"})).To(Succeed())
		})
		server := httptest.NewServer(mux)
		DeferCleanup(server.Close)
		cfg = config.SourceConfig{
			Environment: "env", Zone: "zone",
			Url: server.URL + "/admin", TokenUrl: server.URL + "/realms/test",
			ClientId: "client", ClientSecret: "secret",
		}
	})

	It("authenticates API requests and reuses its token", func() {
		client, err := source.NewKongClient(cfg)
		Expect(err).NotTo(HaveOccurred())
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		for range 2 {
			res, err := client.GetRouteWithResponse(ctx, "my-route")
			Expect(err).NotTo(HaveOccurred())
			Expect(res.StatusCode()).To(Equal(http.StatusOK))
			Expect(res.JSON200).NotTo(BeNil())
			Expect(res.JSON200.Name).NotTo(BeNil())
			Expect(*res.JSON200.Name).To(Equal("my-route"))
		}
		Expect(tokenRequests.Load()).To(BeEquivalentTo(1))
		Expect(apiRequests.Load()).To(BeEquivalentTo(2))
	})

	It("does not call the API when token acquisition fails", func() {
		cfg.TokenUrl += "/missing"
		client, err := source.NewKongClient(cfg)
		Expect(err).NotTo(HaveOccurred())
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, err = client.GetRouteWithResponse(ctx, "my-route")
		Expect(err).To(HaveOccurred())
		Expect(apiRequests.Load()).To(BeZero())
	})

	It("rejects malformed admin URLs", func() {
		cfg.Url = "http://[::1"
		client, err := source.NewKongClient(cfg)
		Expect(err).To(MatchError(ContainSubstring("failed to parse gateway URL")))
		Expect(client).To(BeNil())
	})

	It("constructs a source without contacting the gateway", func() {
		src, err := source.NewKongSourceFromConfig(cfg, []string{"tag"})
		Expect(err).NotTo(HaveOccurred())
		Expect(src).NotTo(BeNil())
		Expect(tokenRequests.Load()).To(BeZero())
		Expect(apiRequests.Load()).To(BeZero())
	})

	It("wraps client construction errors", func() {
		cfg.Url = "http://[::1"
		src, err := source.NewKongSourceFromConfig(cfg, nil)
		Expect(err).To(MatchError(ContainSubstring("failed to create kong client from config")))
		Expect(src).To(BeNil())
	})
})
