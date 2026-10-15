// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package kongutil_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"

	kong "github.com/telekom/controlplane/gateway/pkg/kong/api"
	"github.com/telekom/controlplane/gateway/pkg/kongutil"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("NewClientFor TLS verification", func() {
	var (
		server    *httptest.Server
		tokenHits atomic.Int32
		adminHits atomic.Int32
		gwCfg     kongutil.GatewayAdminConfig
	)

	BeforeEach(func() {
		tokenHits.Store(0)
		adminHits.Store(0)
		server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.HasSuffix(r.URL.Path, "/token") {
				tokenHits.Add(1)
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"access_token":"tok","token_type":"Bearer","expires_in":300}`))
				return
			}
			adminHits.Add(1)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":[]}`))
		}))
		DeferCleanup(server.Close)
		gwCfg = kongutil.NewGatewayConfig(server.URL+"/admin-api", "client", "secret", server.URL+"/realms/master")
	})

	listServices := func() error {
		c, err := kongutil.NewClientFor(gwCfg)
		Expect(err).NotTo(HaveOccurred())
		_, err = c.ListServiceWithResponse(context.Background(), &kong.ListServiceParams{})
		return err
	}

	DescribeTable("rejects an untrusted certificate unless explicitly enabled",
		func(value string, set bool) {
			setOrUnsetEnv(kongutil.EnvSkipTlsVerify, value, set)
			err := listServices()
			Expect(err).To(MatchError(ContainSubstring("certificate")))
			Expect(tokenHits.Load()).To(BeZero())
			Expect(adminHits.Load()).To(BeZero())
		},
		Entry("when unset", "", false),
		Entry("when false", "false", true),
		Entry("when not a true value", "yes", true),
	)

	It("permits token and admin requests when SKIP_TLS_VERIFY=true", func() {
		GinkgoT().Setenv(kongutil.EnvSkipTlsVerify, "true")
		Expect(listServices()).To(Succeed())
		Expect(tokenHits.Load()).To(Equal(int32(1)))
		Expect(adminHits.Load()).To(Equal(int32(1)))
	})

	It("does not affect the default HTTP client when SKIP_TLS_VERIFY=true", func() {
		GinkgoT().Setenv(kongutil.EnvSkipTlsVerify, "true")
		Expect(listServices()).To(Succeed())

		resp, err := http.Get(server.URL + "/admin-api") //nolint:noctx // test-only request
		if resp != nil {
			_ = resp.Body.Close()
		}
		Expect(err).To(MatchError(ContainSubstring("certificate")))
	})
})

// setOrUnsetEnv sets or explicitly unsets key for the current spec and
// restores the previous value afterwards.
func setOrUnsetEnv(key, value string, set bool) {
	previous, wasSet := os.LookupEnv(key)
	DeferCleanup(func() {
		if wasSet {
			Expect(os.Setenv(key, previous)).To(Succeed())
			return
		}
		Expect(os.Unsetenv(key)).To(Succeed())
	})
	if set {
		Expect(os.Setenv(key, value)).To(Succeed())
		return
	}
	Expect(os.Unsetenv(key)).To(Succeed())
}
