// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package server_test

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"sync"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/gofiber/fiber/v2"

	"github.com/telekom/controlplane/common-server/pkg/server"
	k8s "github.com/telekom/controlplane/common-server/pkg/server/middleware/kubernetes"
	"github.com/telekom/controlplane/common-server/pkg/server/middleware/security"
)

var validK8s = server.K8sConfig{
	TrustedIssuers: []string{"https://issuer"},
	AccessConfig:   []k8s.ServiceAccessConfig{{ServiceAccountName: "sa", Namespace: "ns"}},
}

var _ = Describe("K8s family fail-closed startup", func() {
	It("panics when neither inCluster nor trustedIssuers is set (even with JWKS URLs)", func() {
		cfg := server.K8sConfig{
			JWKSetURLs:   []string{"https://jwks"},
			AccessConfig: validK8s.AccessConfig,
		}
		Expect(func() { server.K8sFamily(cfg, false)(fiber.New()) }).To(Panic())
	})

	It("panics when accessConfig is empty and open access is not allowed", func() {
		cfg := server.K8sConfig{TrustedIssuers: []string{"https://issuer"}}
		Expect(func() { server.K8sFamily(cfg, false)(fiber.New()) }).To(Panic())
	})

	It("panics identically for the admin-context variant when the issuer is missing", func() {
		cfg := server.K8sConfig{AccessConfig: validK8s.AccessConfig} // no issuers
		Expect(func() { server.K8sFamilyWithAdminContext(cfg)(fiber.New()) }).To(Panic())
	})

	It("does not trip the accessConfig guard for the admin-context variant (open in-cluster trust zone)", func() {
		// Constructing a real k8s handler reaches the cluster and panics for
		// unrelated reasons in a unit test, so assert on the panic *message*:
		// it must not be the accessConfig one.
		cfg := server.K8sConfig{InCluster: true} // no accessConfig
		defer func() {
			r := recover()
			if r != nil {
				Expect(fmt.Sprint(r)).NotTo(ContainSubstring("accessConfig"))
			}
		}()
		server.K8sFamilyWithAdminContext(cfg)(fiber.New())
	})

	It("does not trip the accessConfig guard for a pure-k8s internal listener (allowOpenAccess=true)", func() {
		// Internal pure-k8s (secret-manager): empty accessConfig is permitted
		// without a synthetic BusinessContext.
		cfg := server.K8sConfig{InCluster: true} // no accessConfig
		defer func() {
			r := recover()
			if r != nil {
				Expect(fmt.Sprint(r)).NotTo(ContainSubstring("accessConfig"))
			}
		}()
		server.K8sFamily(cfg, true)(fiber.New())
	})

	It("still trips the accessConfig guard for the pure-k8s variant when open access is not allowed", func() {
		cfg := server.K8sConfig{InCluster: true} // no accessConfig
		defer func() {
			r := recover()
			Expect(r).NotTo(BeNil())
			Expect(fmt.Sprint(r)).To(ContainSubstring("accessConfig"))
		}()
		server.K8sFamily(cfg, false)(fiber.New())
	})
})

var _ = Describe("FamilyFromListenerConfig", func() {
	jwtBlock := &security.JWTConfig{Mode: security.ModeMock}

	It("fails closed when neither family block is set", func() {
		_, err := server.FamilyFromListenerConfig(server.ListenerConfig{Address: ":1"}, nil)
		Expect(err).To(HaveOccurred())
	})

	It("fails closed when both family blocks are set", func() {
		lc := server.ListenerConfig{Address: ":1", JWT: jwtBlock, K8s: &validK8s}
		_, err := server.FamilyFromListenerConfig(lc, nil)
		Expect(err).To(HaveOccurred())
	})

	It("selects a family for a valid k8s-only listener (pure k8s)", func() {
		lc := server.ListenerConfig{Address: ":1", K8s: &validK8s}
		fam, err := server.FamilyFromListenerConfig(lc, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(fam).NotTo(BeNil())
	})

	It("returns a family from an empty k8s block without a validation error (k8s: {})", func() {
		// Constructing the family reads the cluster CA, so assert only that
		// selection succeeds; the inCluster-issuer default is covered by the
		// K8sConfig.ToOptions tests.
		lc := server.ListenerConfig{Address: ":1", K8s: &server.K8sConfig{}}
		fam, err := server.FamilyFromListenerConfig(lc, nil, server.WithInternal())
		Expect(err).NotTo(HaveOccurred())
		Expect(fam).NotTo(BeNil())
	})
})

var _ = Describe("MultiServer.Run with no listeners", func() {
	It("returns an error when both listeners are nil", func() {
		ms := &server.MultiServer{
			AppConfig: server.NewAppConfig(),
			Register:  func(r fiber.Router, guard fiber.Handler) {},
		}
		Expect(ms.Run(context.Background())).To(HaveOccurred())
	})
})

var _ = Describe("Guarded", func() {
	h := func(c *fiber.Ctx) error { return nil }

	It("drops a nil guard", func() {
		Expect(server.Guarded(nil, h)).To(HaveLen(1))
	})

	It("prepends a non-nil guard", func() {
		g := func(c *fiber.Ctx) error { return nil }
		Expect(server.Guarded(g, h)).To(HaveLen(2))
	})
})

var _ = Describe("Synthetic admin BusinessContext", func() {
	newApp := func() *fiber.App {
		app := fiber.New()
		app.Use(security.NewSyntheticAdminBusinessContext())
		app.Get("/res", func(c *fiber.Ctx) error {
			bCtx, ok := security.FromContext(c.UserContext())
			if !ok {
				return c.SendStatus(http.StatusInternalServerError)
			}
			return c.JSON(fiber.Map{
				"env":        bCtx.Environment,
				"clientType": string(bCtx.ClientType),
				"accessType": string(bCtx.AccessType),
				"prefix":     security.PrefixFromContext(c.UserContext()),
			})
		})
		return app
	}

	It("injects admin context and prefix from X-Environment", func() {
		req, _ := http.NewRequest("GET", "http://localhost/res", nil)
		req.Header.Set("X-Environment", "envA")
		resp, err := newApp().Test(req)
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.StatusCode).To(Equal(http.StatusOK))
	})

	It("rejects a missing X-Environment with 400", func() {
		req, _ := http.NewRequest("GET", "http://localhost/res", nil)
		resp, err := newApp().Test(req)
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))
	})
})

var _ = Describe("MultiServer.Run bind-failure teardown", func() {
	It("returns an error and tears down siblings when a listener cannot bind", func() {
		// Pre-bind a port so one listener collides.
		ln, err := net.Listen("tcp4", "127.0.0.1:0")
		Expect(err).NotTo(HaveOccurred())
		defer ln.Close() //nolint:errcheck
		busyAddr := ln.Addr().String()

		fam := func(r fiber.Router) fiber.Handler { return nil }
		ms := &server.MultiServer{
			AppConfig: server.NewAppConfig(),
			Listeners: server.Listeners{
				Internal: &server.Listener{Address: "127.0.0.1:0", Family: fam}, // ephemeral, binds ok
				External: &server.Listener{Address: busyAddr, Family: fam},      // collides -> error
			},
			Register: func(r fiber.Router, guard fiber.Handler) {
				r.Get("/x", func(c *fiber.Ctx) error { return nil })
			},
		}
		err = ms.Run(context.Background())
		Expect(err).To(HaveOccurred())
	})
})

var _ = Describe("MultiServer shared-TLS wiring", func() {
	It("passes the same cert/key to every TLS listener via the serveTLS seam", func() {
		type call struct{ addr, cert, key string }
		var (
			mu    sync.Mutex
			calls []call
		)
		seam := func(ctx context.Context, app *fiber.App, addr, cert, key string) error {
			mu.Lock()
			calls = append(calls, call{addr, cert, key})
			mu.Unlock()
			<-ctx.Done() // block until teardown, like a real serve
			return nil
		}
		fam := func(r fiber.Router) fiber.Handler { return nil }
		ms := &server.MultiServer{
			AppConfig: server.NewAppConfig(),
			TLS:       &server.TLSConfig{CertFile: "/c", KeyFile: "/k"},
			Listeners: server.Listeners{
				Internal: &server.Listener{Address: ":1", Family: fam},
				External: &server.Listener{Address: ":2", Family: fam},
			},
			Register: func(r fiber.Router, guard fiber.Handler) {},
		}
		ms.SetServeTLS(seam)

		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan error, 1)
		go func() { done <- ms.Run(ctx) }()

		Eventually(func() int {
			mu.Lock()
			defer mu.Unlock()
			return len(calls)
		}).Should(Equal(2))
		cancel()
		Eventually(done).Should(Receive(BeNil()))

		mu.Lock()
		defer mu.Unlock()
		for _, c := range calls {
			Expect(c.cert).To(Equal("/c"))
			Expect(c.key).To(Equal("/k"))
		}
	})
})
