// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package source

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/telekom/controlplane/tools/snapshotter/pkg/config"
)

func TestSource(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Source Suite")
}

var _ = Describe("NewKongSourceFromConfig", func() {
	var (
		tokenCalls atomic.Int32
		apiCalls   atomic.Int32
		cfg        config.SourceConfig
	)

	BeforeEach(func() {
		tokenCalls.Store(0)
		apiCalls.Store(0)
		// httptest TLS servers use a certificate that is not trusted by the system roots.
		tokenSrv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			tokenCalls.Add(1)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "t", "token_type": "Bearer", "expires_in": 3600})
		}))
		DeferCleanup(tokenSrv.Close)
		apiSrv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			apiCalls.Add(1)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{}`))
		}))
		DeferCleanup(apiSrv.Close)

		cfg = config.SourceConfig{
			Environment:  "env",
			Zone:         "zone",
			Url:          apiSrv.URL,
			TokenUrl:     tokenSrv.URL + "/realms/rover",
			ClientId:     "client",
			ClientSecret: "secret",
		}
	})

	newSource := func() *KongSource {
		s, err := NewKongSourceFromConfig(cfg, nil)
		Expect(err).NotTo(HaveOccurred())
		ks, ok := s.(*KongSource)
		Expect(ok).To(BeTrue())
		DeferCleanup(ks.Close)
		return ks
	}

	It("verifies TLS by default", func() {
		ks := newSource()
		_, err := ks.kongClient.GetRouteWithResponse(context.Background(), "route")
		Expect(err).To(MatchError(ContainSubstring("certificate")))
		Expect(tokenCalls.Load()).To(BeZero())
		Expect(apiCalls.Load()).To(BeZero())
	})

	It("skips TLS verification for token and API requests if configured", func() {
		cfg.InsecureSkipVerify = true
		ks := newSource()
		_, err := ks.kongClient.GetRouteWithResponse(context.Background(), "route")
		Expect(err).NotTo(HaveOccurred())
		Expect(tokenCalls.Load()).To(BeNumerically(">", 0))
		Expect(apiCalls.Load()).To(BeNumerically(">", 0))
	})

	It("fails for invalid URLs", func() {
		cfg.Url = "not-a-url"
		_, err := NewKongSourceFromConfig(cfg, nil)
		Expect(err).To(HaveOccurred())
	})

	It("tolerates Close without a client", func() {
		NewKongSource(nil).Close()
	})
})
