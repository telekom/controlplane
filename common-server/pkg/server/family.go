// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package server

import (
	"github.com/gofiber/fiber/v2"

	k8s "github.com/telekom/controlplane/common-server/pkg/server/middleware/kubernetes"
	"github.com/telekom/controlplane/common-server/pkg/server/middleware/security"
)

// Family installs its auth / business-context middleware onto router via
// router.Use(...) and returns the per-route guard to attach to each route, or
// nil if the family guards entirely via Use (no per-route guard needed).
type Family func(router fiber.Router) (guard fiber.Handler)

// JWTFamily wraps security.ConfigureSecurity: it installs the JWT +
// business-context middleware and returns the checkAccess handler as the
// per-route guard. The caller assembles the full SecurityOpts (mode, issuers,
// and the server-specific check-access templates) — see
// JWTConfig.ToSecurityOpts.
func JWTFamily(opts security.SecurityOpts) Family {
	return func(router fiber.Router) fiber.Handler {
		return security.ConfigureSecurity(router, opts)
	}
}

// K8sFamily is for pure-k8s servers (file-manager, secret-manager) that never
// read BusinessContext. It installs the Kubernetes-authz middleware and returns
// a nil guard (authorization is done inside the middleware; no per-route guard).
//
// allowOpenAccess permits an empty accessConfig ("any authenticated in-cluster
// SA"): pass true only for an internal listener in a trusted in-cluster zone.
// The issuer guard always applies regardless.
func K8sFamily(opts K8sConfig, allowOpenAccess bool) Family {
	return func(router fiber.Router) fiber.Handler {
		router.Use(buildK8sHandler(opts, allowOpenAccess))
		return nil
	}
}

// K8sFamilyWithAdminContext is for the internal port of JWT-servers (rover,
// discovery, controlplane-api). It installs the Kubernetes-authz middleware
// plus a synthetic admin BusinessContext, so downstream handlers that read
// FromContext / PrefixFromContext keep working for a trusted, allow-listed SA.
// checkAccess is deliberately skipped (nil guard).
//
// This is always an internal trust-zone listener, so an empty accessConfig is
// permitted (allowOpenAccess=true): it means "any authenticated in-cluster SA
// may call". The issuer guard still applies — an unverifiable token is never
// accepted.
func K8sFamilyWithAdminContext(opts K8sConfig) Family {
	return func(router fiber.Router) fiber.Handler {
		router.Use(buildK8sHandler(opts, true))
		router.Use(security.NewSyntheticAdminBusinessContext())
		return nil
	}
}

// buildK8sHandler validates the k8s config fail-closed, then constructs the
// Kubernetes-authz middleware. It panics on misconfiguration so a misconfigured
// port can never boot.
//
// The issuer surface is always closed: NewKubernetesAuthz returns a
// pass-through handler when there are no trusted issuers (WithJWKSetURLs alone
// does NOT add issuers), so we require inCluster or an explicit issuer list.
//
// The accessConfig surface (an empty allow-list authorizes any valid cluster
// SA) is closed only when allowOpenAccess is false. Internal listeners pass
// true: empty accessConfig is an intentional "allow any authenticated
// in-cluster SA". External listeners pass false and must supply a non-empty
// allow-list.
func buildK8sHandler(opts K8sConfig, allowOpenAccess bool) fiber.Handler {
	if !opts.InCluster && len(opts.TrustedIssuers) == 0 {
		panic("k8s security family requires trustedIssuers or inCluster=true; refusing to start fail-open")
	}
	if !allowOpenAccess && len(opts.AccessConfig) == 0 {
		panic("k8s security family requires a non-empty accessConfig; refusing to start with unrestricted access")
	}
	return k8s.NewKubernetesAuthz(opts.ToOptions()...)
}
