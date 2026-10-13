// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package keycloak_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"

	identityv1 "github.com/telekom/controlplane/identity/api/v1"
	"github.com/telekom/controlplane/identity/pkg/keycloak"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("NewKeycloakServiceFor TLS verification", func() {
	var (
		server      *httptest.Server
		tokenHits   atomic.Int32
		adminHits   atomic.Int32
		realmStatus identityv1.RealmStatus
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
			w.WriteHeader(http.StatusNoContent)
		}))
		DeferCleanup(server.Close)
		realmStatus = identityv1.RealmStatus{
			AdminUrl:      server.URL + "/admin/realms",
			AdminTokenUrl: server.URL + "/realms/master/protocol/openid-connect/token",
			AdminClientId: "admin-cli",
			AdminUserName: "admin",
			AdminPassword: "secret",
		}
	})

	DescribeTable("rejects an untrusted certificate unless explicitly enabled",
		func(value string, set bool) {
			setOrUnsetEnv(keycloak.EnvSkipTlsVerify, value, set)
			_, err := keycloak.NewKeycloakServiceFor(&realmStatus)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("certificate"))
			Expect(tokenHits.Load()).To(BeZero())
		},
		Entry("when unset", "", false),
		Entry("when false", "false", true),
		Entry("when not a true value", "yes", true),
	)

	It("permits token and admin requests when SKIP_TLS_VERIFY=true", func() {
		GinkgoT().Setenv(keycloak.EnvSkipTlsVerify, "true")
		svc, err := keycloak.NewKeycloakServiceFor(&realmStatus)
		Expect(err).NotTo(HaveOccurred())
		Expect(tokenHits.Load()).To(Equal(int32(1)))

		Expect(svc.DeleteRealm(context.Background(), "test")).To(Succeed())
		Expect(adminHits.Load()).To(BeNumerically(">=", 1))
	})

	It("does not affect the default HTTP client when SKIP_TLS_VERIFY=true", func() {
		GinkgoT().Setenv(keycloak.EnvSkipTlsVerify, "true")
		_, err := keycloak.NewKeycloakServiceFor(&realmStatus)
		Expect(err).NotTo(HaveOccurred())

		resp, err := http.Get(server.URL + "/admin/realms") //nolint:noctx // test-only request
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
