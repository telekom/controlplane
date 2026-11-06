// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package kongutil_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/telekom/controlplane/common/pkg/util/contextutil"
	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
	"github.com/telekom/controlplane/gateway/pkg/kong/legacy"
	"github.com/telekom/controlplane/gateway/pkg/kongutil"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

type recordingMigrator struct {
	routes []legacy.Route
}

func (m *recordingMigrator) Candidates(route legacy.Route) []legacy.Candidate {
	m.routes = append(m.routes, route)
	return nil
}

var _ = Describe("SetMigrator", func() {
	AfterEach(func() {
		kongutil.SetMigrator(nil)
	})

	It("injects the current Migrator into leased clients without changing existing clients", func() {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer GinkgoRecover()
			w.Header().Set("Content-Type", "application/json")
			switch {
			case strings.HasSuffix(r.URL.Path, "/token"):
				_, err := fmt.Fprint(w, `{"access_token":"token","token_type":"Bearer","expires_in":3600}`)
				Expect(err).NotTo(HaveOccurred())
			case r.URL.RawQuery != "":
				_, err := fmt.Fprint(w, `{"data":[]}`)
				Expect(err).NotTo(HaveOccurred())
			default:
				w.WriteHeader(http.StatusNotFound)
			}
		}))
		DeferCleanup(srv.Close)
		ctx := contextutil.WithEnv(context.Background(), "test")
		factory, err := kongutil.NewClientFactory(ctx, nil)
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() {
			stopCtx, cancel := context.WithCancel(ctx)
			cancel()
			Expect(factory.Start(stopCtx)).To(Succeed())
		})
		gw := &gatewayv1.Gateway{
			ObjectMeta: metav1.ObjectMeta{Name: "gw", Namespace: "test", UID: "gw-uid"},
			Spec: gatewayv1.GatewaySpec{Admin: gatewayv1.AdminConfig{
				Url: srv.URL, IssuerUrl: srv.URL, ClientId: "client", ClientSecret: "secret",
			}},
		}
		before, release, err := kongutil.GetClientFor(ctx, factory, gw)
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(release)

		m := &recordingMigrator{}
		kongutil.SetMigrator(m)

		after, release, err := kongutil.GetClientFor(ctx, factory, gw)
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(release)
		route := &gatewayv1.Route{ObjectMeta: metav1.ObjectMeta{Name: "test-route"}}
		route.SetNamePrefix("bond")
		Expect(before.DeleteRoute(ctx, route)).To(Succeed())
		Expect(m.routes).To(BeEmpty())
		Expect(after.DeleteRoute(ctx, route)).To(Succeed())
		Expect(m.routes).NotTo(BeEmpty())
		for _, seen := range m.routes {
			Expect(seen).To(Equal(legacy.Route{Name: "test-route", NamePrefix: "bond"}))
		}

		kongutil.SetMigrator(nil)
		m.routes = nil
		again, release, err := kongutil.GetClientFor(ctx, factory, gw)
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(release)
		Expect(again.DeleteRoute(ctx, route)).To(Succeed())
		Expect(m.routes).To(BeEmpty())
	})
})
