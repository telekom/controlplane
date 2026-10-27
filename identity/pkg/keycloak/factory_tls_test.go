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
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	identityv1 "github.com/telekom/controlplane/identity/api/v1"
	"github.com/telekom/controlplane/identity/pkg/keycloak"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("ServiceFactory TLS verification", func() {
	var (
		server      *httptest.Server
		tokenHits   atomic.Int32
		adminHits   atomic.Int32
		realmStatus identityv1.RealmStatus
		factory     *keycloak.CachedServiceFactory
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
		ctx, cancel := context.WithCancel(context.Background())
		var err error
		factory, err = keycloak.NewServiceFactory(ctx, nil)
		Expect(err).NotTo(HaveOccurred())
		done := make(chan struct{})
		go func() {
			defer GinkgoRecover()
			defer close(done)
			Expect(factory.Start(ctx)).To(Succeed())
		}()
		DeferCleanup(func() {
			cancel()
			Eventually(done).WithTimeout(5 * time.Second).WithPolling(10 * time.Millisecond).Should(BeClosed())
		})
	})

	DescribeTable("rejects an untrusted certificate regardless of the legacy bypass environment variable",
		func(value string, set bool) {
			setOrUnsetEnv("SKIP_TLS_VERIFY", value, set)
			realm := &identityv1.Realm{ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "test", UID: "test"}}
			_, _, err := factory.ServiceFor(context.Background(), realm, realmStatus)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("certificate"))
			Expect(tokenHits.Load()).To(BeZero())
			Expect(adminHits.Load()).To(BeZero())
		},
		Entry("when unset", "", false),
		Entry("when false", "false", true),
		Entry("when not a true value", "yes", true),
		Entry("when true", "true", true),
		Entry("when mixed case with whitespace", " TRUE ", true),
	)
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
