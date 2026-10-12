// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package keycloak_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	identityv1 "github.com/telekom/controlplane/identity/api/v1"
	"github.com/telekom/controlplane/identity/pkg/keycloak"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("ServiceFactory Admin API 401 recovery", func() {
	var (
		passwordGrants atomic.Int32
		tokenStatus    atomic.Int32
		adminRequests  atomic.Int32
		adminHandler   func(w http.ResponseWriter, r *http.Request)
		tokenSrv       *httptest.Server
		adminSrv       *httptest.Server
		status         identityv1.RealmStatus
		factory        keycloak.ServiceFactory
		ctx            context.Context
	)

	BeforeEach(func() {
		ctx = context.Background()
		passwordGrants.Store(0)
		tokenStatus.Store(http.StatusOK)
		adminRequests.Store(0)
		adminHandler = func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }

		tokenSrv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			if code := int(tokenStatus.Load()); code != http.StatusOK {
				w.WriteHeader(code)
				_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid_grant"})
				return
			}
			n := passwordGrants.Add(1)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"access_token": fmt.Sprintf("tok-%d", n),
				"token_type":   "Bearer",
				"expires_in":   3600,
			})
		}))
		adminSrv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			adminRequests.Add(1)
			adminHandler(w, r)
		}))
		status = identityv1.RealmStatus{
			AdminUrl:      adminSrv.URL,
			AdminTokenUrl: tokenSrv.URL,
			AdminClientId: "admin-cli",
			AdminUserName: "admin",
			AdminPassword: "secret",
		}
		factory = keycloak.NewServiceFactory()
	})

	AfterEach(func() {
		adminSrv.Close()
		tokenSrv.Close()
	})

	rejectToken := func(token string) func(http.ResponseWriter, *http.Request) {
		return func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Authorization") == "Bearer "+token {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		}
	}

	expectRetryable := func(err error) {
		ExpectWithOffset(1, err).To(HaveOccurred())
		ExpectWithOffset(1, isRetryable(err)).To(BeTrue())
		ExpectWithOffset(1, isBlocked(err)).To(BeFalse())
	}

	It("reuses the cached service on normal operation", func() {
		svc1, err := factory.ServiceFor(status)
		Expect(err).NotTo(HaveOccurred())
		Expect(svc1.DeleteRealm(ctx, "r")).To(Succeed())
		svc2, err := factory.ServiceFor(status)
		Expect(err).NotTo(HaveOccurred())
		Expect(svc2).To(BeIdenticalTo(svc1))
		Expect(svc2.DeleteRealm(ctx, "r")).To(Succeed())
		Expect(passwordGrants.Load()).To(Equal(int32(1)))
	})

	It("retires the service on 401 without replay and recreates it with fresh login", func() {
		adminHandler = rejectToken("tok-1")
		svc1, err := factory.ServiceFor(status)
		Expect(err).NotTo(HaveOccurred())

		expectRetryable(svc1.DeleteRealm(ctx, "r"))
		Expect(adminRequests.Load()).To(Equal(int32(1)))

		By("rejecting further calls on the retired service without HTTP requests")
		expectRetryable(svc1.DeleteRealm(ctx, "r"))
		expectRetryable(svc1.DeleteRealm(ctx, "r"))
		Expect(adminRequests.Load()).To(Equal(int32(1)))

		svc2, err := factory.ServiceFor(status)
		Expect(err).NotTo(HaveOccurred())
		Expect(svc2).NotTo(BeIdenticalTo(svc1))
		Expect(passwordGrants.Load()).To(Equal(int32(2)))
		Expect(svc2.DeleteRealm(ctx, "r")).To(Succeed())
		Expect(adminRequests.Load()).To(Equal(int32(2)))
	})

	It("stays retryable on persistent 401 without internal replay loops", func() {
		adminHandler = func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusUnauthorized) }
		for i := 1; i <= 3; i++ {
			svc, err := factory.ServiceFor(status)
			Expect(err).NotTo(HaveOccurred())
			expectRetryable(svc.DeleteRealm(ctx, "r"))
			Expect(adminRequests.Load()).To(Equal(int32(i)))
			Expect(passwordGrants.Load()).To(Equal(int32(i)))
		}
	})

	It("does not let a late 401 from a retired service evict its replacement", func() {
		release := make(chan struct{})
		blocked := make(chan struct{})
		var once sync.Once
		adminHandler = func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Authorization") != "Bearer tok-1" {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			if strings.HasSuffix(r.URL.Path, "/slow") {
				once.Do(func() { close(blocked) })
				<-release
			}
			w.WriteHeader(http.StatusUnauthorized)
		}

		svc1, err := factory.ServiceFor(status)
		Expect(err).NotTo(HaveOccurred())

		slowErr := make(chan error, 1)
		go func() { slowErr <- svc1.DeleteRealm(ctx, "slow") }()
		Eventually(blocked).WithTimeout(5 * time.Second).WithPolling(10 * time.Millisecond).Should(BeClosed())

		expectRetryable(svc1.DeleteRealm(ctx, "fast"))
		svc2, err := factory.ServiceFor(status)
		Expect(err).NotTo(HaveOccurred())
		Expect(svc2).NotTo(BeIdenticalTo(svc1))

		close(release)
		var lateErr error
		Eventually(slowErr).WithTimeout(5 * time.Second).WithPolling(10 * time.Millisecond).Should(Receive(&lateErr))
		expectRetryable(lateErr)

		svc3, err := factory.ServiceFor(status)
		Expect(err).NotTo(HaveOccurred())
		Expect(svc3).To(BeIdenticalTo(svc2))
		Expect(svc3.DeleteRealm(ctx, "r")).To(Succeed())
		Expect(passwordGrants.Load()).To(Equal(int32(2)))
	})

	It("handles concurrent 401s safely", func() {
		adminHandler = rejectToken("tok-1")
		svc1, err := factory.ServiceFor(status)
		Expect(err).NotTo(HaveOccurred())

		var wg sync.WaitGroup
		errs := make([]error, 8)
		for i := range errs {
			wg.Add(1)
			go func(i int) {
				defer GinkgoRecover()
				defer wg.Done()
				errs[i] = svc1.DeleteRealm(ctx, "r")
				_, _ = factory.ServiceFor(status)
			}(i)
		}
		wg.Wait()
		for _, e := range errs {
			expectRetryable(e)
		}

		svc2, err := factory.ServiceFor(status)
		Expect(err).NotTo(HaveOccurred())
		Expect(svc2.DeleteRealm(ctx, "r")).To(Succeed())
	})

	It("keeps 403 blocked and the service cached", func() {
		adminHandler = func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusForbidden) }
		svc1, err := factory.ServiceFor(status)
		Expect(err).NotTo(HaveOccurred())

		err = svc1.DeleteRealm(ctx, "r")
		Expect(err).To(HaveOccurred())
		Expect(isBlocked(err)).To(BeTrue())

		svc2, err := factory.ServiceFor(status)
		Expect(err).NotTo(HaveOccurred())
		Expect(svc2).To(BeIdenticalTo(svc1))
		Expect(adminRequests.Load()).To(Equal(int32(1)))
	})

	It("keeps token endpoint login failures out of the Admin 401 path", func() {
		tokenStatus.Store(http.StatusUnauthorized)
		_, err := factory.ServiceFor(status)
		Expect(err).To(MatchError(ContainSubstring("failed to retrieve token")))
		Expect(adminRequests.Load()).To(BeZero())
	})
})

func isRetryable(err error) bool {
	var re interface{ IsRetryable() bool }
	return errors.As(err, &re) && re.IsRetryable()
}

func isBlocked(err error) bool {
	var be interface{ IsBlocked() bool }
	return errors.As(err, &be) && be.IsBlocked()
}
