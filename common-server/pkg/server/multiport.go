// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package server

import (
	"context"
	"fmt"
	"sync"

	"github.com/gofiber/fiber/v2"

	"github.com/telekom/controlplane/common-server/pkg/server/middleware/security"
	"github.com/telekom/controlplane/common-server/pkg/server/serve"
)

// TLSConfig is the shared cert/key used by every TLS listener.
type TLSConfig struct {
	CertFile string
	KeyFile  string
}

// Listener binds one address to one security family. Each listener becomes its
// own *fiber.App with its own middleware tree, so credentials are structurally
// isolated across ports.
type Listener struct {
	Address string
	Family  Family
}

// Listeners holds the (at most two) listeners a server runs, keyed by role.
// A nil field means that role is not served. Internal is the in-cluster trust
// zone (typically K8s auth); External faces outside (typically JWT).
type Listeners struct {
	Internal *Listener
	External *Listener
}

// present returns the non-nil listeners in Internal-then-External order.
func (l Listeners) present() []*Listener {
	out := make([]*Listener, 0, 2)
	if l.Internal != nil {
		out = append(out, l.Internal)
	}
	if l.External != nil {
		out = append(out, l.External)
	}
	return out
}

// RegisterFunc registers the identical route set onto one app's router,
// attaching guard to each route (guard may be nil).
type RegisterFunc func(router fiber.Router, guard fiber.Handler)

// MultiServer serves the same route set on N ports, one *fiber.App per port,
// each running its own independently-chosen security family. All TLS listeners
// share one cert (TLS != nil); TLS == nil serves plain HTTP (dev).
type MultiServer struct {
	AppConfig AppConfig
	TLS       *TLSConfig
	Listeners Listeners
	Register  RegisterFunc

	// serveTLS is an injectable seam (defaults to serve.ServeTLS) so tests can
	// assert the cert/key passed to every TLS listener without real TLS.
	serveTLS func(ctx context.Context, app *fiber.App, addr, cert, key string) error
}

// SetServeTLS overrides the TLS serving function. It exists so tests can assert
// the cert/key passed to every TLS listener without real TLS. Production code
// leaves it unset (defaults to serve.ServeTLS).
func (m *MultiServer) SetServeTLS(fn func(ctx context.Context, app *fiber.App, addr, cert, key string) error) {
	m.serveTLS = fn
}

// Run builds one app per listener, installs probes + the listener's family +
// the shared route set, and serves each app in its own goroutine. If any
// listener's serve returns an error, the shared context is cancelled so all
// sibling apps shut down and the process can exit (no half-up state).
func (m *MultiServer) Run(ctx context.Context) error {
	listeners := m.Listeners.present()
	if len(listeners) == 0 {
		return fmt.Errorf("MultiServer requires at least one listener")
	}
	serveTLSFn := m.serveTLS
	if serveTLSFn == nil {
		serveTLSFn = serve.ServeTLS
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	apps := make([]*fiber.App, len(listeners))
	for i, l := range listeners {
		app := NewAppWithConfig(m.AppConfig)

		// Probes are unauthenticated and identical on every port.
		NewProbesController().Register(app, ControllerOpts{})

		// The family installs its middleware and returns the per-route guard.
		guard := l.Family(app)

		// The same route set on every port.
		m.Register(app, guard)

		apps[i] = app
	}

	var wg sync.WaitGroup
	errCh := make(chan error, len(listeners))
	for i, l := range listeners {
		wg.Add(1)
		go func(app *fiber.App, addr string) {
			defer wg.Done()
			var err error
			if m.TLS != nil {
				err = serveTLSFn(ctx, app, addr, m.TLS.CertFile, m.TLS.KeyFile)
			} else {
				err = app.Listen(addr)
			}
			if err != nil {
				errCh <- fmt.Errorf("listener %q: %w", addr, err)
				cancel() // bring down siblings
			}
		}(apps[i], l.Address)
	}

	// Shut apps down when the context is cancelled (by an error above or by the
	// caller), then wait for the serve goroutines to unwind.
	go func() {
		<-ctx.Done()
		for _, app := range apps {
			_ = app.Shutdown()
		}
	}()

	wg.Wait()
	close(errCh)
	if err := <-errCh; err != nil {
		return err
	}
	return nil
}

// Guarded prepends guard to h when guard is non-nil, so a route registration
// works uniformly whether or not the family uses a per-route guard. Servers use
// it in their RegisterFunc: router.Add(method, path, Guarded(guard, h)...).
func Guarded(guard fiber.Handler, h fiber.Handler) []fiber.Handler {
	if guard == nil {
		return []fiber.Handler{h}
	}
	return []fiber.Handler{guard, h}
}

// FamilyFromListenerConfig selects the family from the single present family
// block on lc, failing closed if zero or both blocks are set.
//
// Options tune k8s listeners (they are ignored for a jwt block):
//   - WithAdminContext installs a synthetic admin BusinessContext (JWT-servers'
//     internal port: rover, discovery need it; pure-k8s servers like
//     secret-manager do not). It implies WithInternal.
//   - WithInternal marks an in-cluster trust-zone listener, permitting an empty
//     accessConfig ("any authenticated in-cluster SA"). External listeners omit
//     it and must supply an allow-list.
//
// jwtOpts turns a listener's jwt config into the full SecurityOpts, letting a
// JWT-server inject its server-specific check-access templates. It may be nil
// for pure-k8s servers that never declare a jwt block; a jwt block with a nil
// jwtOpts falls back to JWTConfig.ToSecurityOpts (no templates).
func FamilyFromListenerConfig(lc ListenerConfig, jwtOpts func(security.JWTConfig) security.SecurityOpts, opts ...FamilyOption) (Family, error) {
	fo := familyOpts{}
	for _, o := range opts {
		o(&fo)
	}
	// admin-context is only ever installed on an internal listener.
	internal := fo.internal || fo.adminContext

	if err := lc.Validate(internal); err != nil {
		return nil, err
	}
	if lc.JWT != nil {
		opts := lc.JWT.ToSecurityOpts()
		if jwtOpts != nil {
			opts = jwtOpts(*lc.JWT)
		}
		return JWTFamily(opts), nil
	}
	// lc.K8s != nil (Validate guarantees exactly one).
	k8sCfg := *lc.K8s
	// k8s: {} means "use the local cluster": with no explicit issuer, default
	// inCluster=true so WithInClusterIssuer auto-discovers the cluster issuer.
	if !k8sCfg.InCluster && len(k8sCfg.TrustedIssuers) == 0 {
		k8sCfg.InCluster = true
	}
	if fo.adminContext {
		// admin-context is only ever installed on an internal listener, which
		// permits open access inside buildK8sHandler.
		return K8sFamilyWithAdminContext(k8sCfg), nil
	}
	return K8sFamily(k8sCfg, internal), nil
}

// familyOpts holds the tunables for FamilyFromListenerConfig.
type familyOpts struct {
	adminContext bool
	internal     bool
}

// FamilyOption tunes how a k8s listener's family is built.
type FamilyOption func(*familyOpts)

// WithInternal marks the listener as an in-cluster trust-zone listener,
// permitting an empty k8s accessConfig ("any authenticated in-cluster SA").
// External listeners omit it and must supply an allow-list.
func WithInternal() FamilyOption {
	return func(o *familyOpts) { o.internal = true }
}

// WithAdminContext installs a synthetic admin BusinessContext on a k8s
// listener, for the internal port of a JWT-server whose handlers read
// BusinessContext. It implies WithInternal.
func WithAdminContext() FamilyOption {
	return func(o *familyOpts) { o.adminContext = true }
}
