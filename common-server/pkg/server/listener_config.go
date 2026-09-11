// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package server

import (
	"fmt"
	"net/url"

	k8s "github.com/telekom/controlplane/common-server/pkg/server/middleware/kubernetes"
	"github.com/telekom/controlplane/common-server/pkg/server/middleware/security"
)

// TLSFileConfig points at a PEM cert/key pair on disk.
type TLSFileConfig struct {
	Cert string `mapstructure:"cert"`
	Key  string `mapstructure:"key"`
}

// ToServerTLS maps the file-based TLS config to the MultiServer TLS config.
// A nil receiver (dev plain-HTTP: no tls block) returns nil.
func (c *TLSFileConfig) ToServerTLS() *TLSConfig {
	if c == nil {
		return nil
	}
	return &TLSConfig{CertFile: c.Cert, KeyFile: c.Key}
}

// K8sConfig is the declarative config for the Kubernetes-authz security family.
type K8sConfig struct {
	Audience       string                    `mapstructure:"audience"`
	TrustedIssuers []string                  `mapstructure:"trustedIssuers"`
	JWKSetURLs     []string                  `mapstructure:"jwkSetURLs"`
	AccessConfig   []k8s.ServiceAccessConfig `mapstructure:"accessConfig"`
	InCluster      bool                      `mapstructure:"inCluster"`
}

// ToOptions builds the KubernetesAuthOption slice from the declarative config.
//
//nolint:gocritic // value receiver matches the config-struct convention (JWTConfig.ToSecurityOpts, ListenerConfig.Validate)
func (c K8sConfig) ToOptions() []k8s.KubernetesAuthOption {
	opts := []k8s.KubernetesAuthOption{
		k8s.WithAudience(c.Audience),
		k8s.WithTrustedIssuers(c.TrustedIssuers...),
		k8s.WithJWKSetURLs(c.JWKSetURLs...),
		k8s.WithAccessConfig(c.AccessConfig...),
	}
	if c.InCluster {
		opts = append(opts, k8s.WithInClusterIssuer())
	}
	return opts
}

// ListenerConfig declares a single listener address and exactly one security
// family (JWT or K8s).
type ListenerConfig struct {
	Address string              `mapstructure:"address"`
	JWT     *security.JWTConfig `mapstructure:"jwt,omitempty"`
	K8s     *K8sConfig          `mapstructure:"k8s,omitempty"`
}

// Validate checks the listener config for misconfigurations that would
// otherwise only surface as a panic (or hang) at server-construction time.
// internal marks an in-cluster trust-zone listener: a k8s block there may omit
// accessConfig (open to any authenticated in-cluster SA); an external k8s
// listener must supply an allow-list.
//
// It is pure data validation — it never contacts issuers or the cluster.
func (lc ListenerConfig) Validate(internal bool) error {
	switch {
	case lc.JWT != nil && lc.K8s != nil:
		return fmt.Errorf("listener %q must set exactly one of jwt or k8s, got both", lc.Address)
	case lc.JWT == nil && lc.K8s == nil:
		return fmt.Errorf("listener %q must set exactly one of jwt or k8s, got neither", lc.Address)
	}
	if lc.Address == "" {
		return fmt.Errorf("listener address must not be empty")
	}
	if lc.JWT != nil {
		return validateJWT(lc.Address, lc.JWT)
	}
	return validateK8s(lc.Address, lc.K8s, internal)
}

func validateJWT(addr string, jwt *security.JWTConfig) error {
	switch jwt.Mode {
	case security.ModeJWT:
		if len(jwt.TrustedIssuers) == 0 {
			return fmt.Errorf("listener %q: jwt mode %q requires at least one trustedIssuers entry", addr, jwt.Mode)
		}
		for _, iss := range jwt.TrustedIssuers {
			if err := validateURL(iss); err != nil {
				return fmt.Errorf("listener %q: jwt trustedIssuer %q: %w", addr, iss, err)
			}
		}
	case security.ModeMock:
		// mock mode needs no issuers.
	default:
		return fmt.Errorf("listener %q: unknown jwt mode %q (want %q or %q)", addr, jwt.Mode, security.ModeJWT, security.ModeMock)
	}
	return nil
}

func validateK8s(addr string, k *K8sConfig, internal bool) error {
	// Issuer surface: inCluster auto-discovers the issuer; otherwise an explicit
	// issuer list is required. An empty k8s block (k8s: {}) is allowed — the
	// runtime defaults inCluster=true. So only reject when issuers are set but
	// unparseable, and require an allow-list on external listeners.
	for _, iss := range k.TrustedIssuers {
		if err := validateURL(iss); err != nil {
			return fmt.Errorf("listener %q: k8s trustedIssuer %q: %w", addr, iss, err)
		}
	}
	for _, u := range k.JWKSetURLs {
		if err := validateURL(u); err != nil {
			return fmt.Errorf("listener %q: k8s jwkSetURL %q: %w", addr, u, err)
		}
	}
	if !internal && len(k.AccessConfig) == 0 {
		return fmt.Errorf("listener %q: external k8s listener requires a non-empty accessConfig (an empty allow-list authorizes any cluster SA)", addr)
	}
	seen := map[string]bool{}
	for i := range k.AccessConfig {
		ac := k.AccessConfig[i]
		if ac.ServiceAccountName == "" || ac.Namespace == "" {
			return fmt.Errorf("listener %q: accessConfig[%d] requires service_account_name and namespace", addr, i)
		}
		key := ac.ServiceAccountName + "/" + ac.Namespace
		if seen[key] {
			return fmt.Errorf("listener %q: duplicate accessConfig for %q (later entries silently overwrite earlier ones)", addr, key)
		}
		seen[key] = true
		for _, at := range ac.AllowedAccess {
			if at != k8s.AccessTypeNone && at != k8s.AccessTypeRead && at != k8s.AccessTypeWrite {
				return fmt.Errorf("listener %q: accessConfig[%d] has invalid allowed_access %q", addr, i, at)
			}
		}
	}
	return nil
}

// validateURL rejects a value that is not a parseable absolute URL.
func validateURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("not a valid URL: %w", err)
	}
	if u.Scheme == "" || u.Host == "" {
		return fmt.Errorf("must be an absolute URL (scheme://host)")
	}
	return nil
}
