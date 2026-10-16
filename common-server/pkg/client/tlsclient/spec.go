// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package tlsclient

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"time"
)

// DefaultTimeout bounds every HTTP request, including token requests,
// when a Profile does not set a timeout.
const DefaultTimeout = 10 * time.Second

// TransportKind selects the base transport.
type TransportKind int

const (
	// TransportRaw uses a zero http.Transport (no proxy from environment).
	TransportRaw TransportKind = iota
	// TransportDefault clones http.DefaultTransport (proxy from environment, default dialer settings).
	TransportDefault
)

// Profile holds immutable, caller specific HTTP settings.
// Profiles are registered once per Factory and referenced by name from a Spec.
type Profile struct {
	// Name identifies the profile within a factory.
	Name string
	// Transport selects the base transport.
	Transport TransportKind
	// Timeout bounds each HTTP request, including token requests. Zero means DefaultTimeout.
	Timeout time.Duration
	// MaxIdleConnsPerHost is applied to the transport if > 0.
	MaxIdleConnsPerHost int
	// MinTLSVersion is applied if != 0.
	MinTLSVersion uint16
	// MetricsClientName enables the metrics wrapper with this client name if not empty.
	MetricsClientName string
	// MetricsReplacePatterns are used to redact metric paths.
	MetricsReplacePatterns []string
	// ClassifyTokenError converts token refresh and re-authentication errors
	// of AuthPassword. Nil wraps the error unchanged.
	ClassifyTokenError func(error) error
}

func (p Profile) timeout() time.Duration {
	if p.Timeout == 0 {
		return DefaultTimeout
	}
	return p.Timeout
}

func (p Profile) validate() error {
	if p.Name == "" {
		return errors.New("profile name is required")
	}
	if p.Timeout < 0 {
		return fmt.Errorf("profile %q: timeout must not be negative", p.Name)
	}
	if p.Transport != TransportRaw && p.Transport != TransportDefault {
		return fmt.Errorf("profile %q: unknown transport kind %d", p.Name, p.Transport)
	}
	for _, pattern := range p.MetricsReplacePatterns {
		if _, err := regexp.Compile(pattern); err != nil {
			return fmt.Errorf("profile %q: invalid metrics replace pattern: %w", p.Name, err)
		}
	}
	return nil
}

// AuthMode selects how requests are authenticated.
type AuthMode string

const (
	AuthNone              AuthMode = "none"
	AuthClientCredentials AuthMode = "client-credentials"
	AuthPassword          AuthMode = "password"
)

// Auth holds OAuth2 settings. TokenURL is required unless Mode is AuthNone.
type Auth struct {
	Mode         AuthMode
	TokenURL     string
	ClientID     string
	ClientSecret string
	Username     string
	Password     string
}

// Owner identifies the resource a cached client belongs to.
// Kind should include the API group, e.g. "gateway.cp.ei.telekom.de/Gateway".
type Owner struct {
	Kind      string
	Namespace string
	Name      string
	UID       string
}

func (o Owner) validate() error {
	if o.Kind == "" || o.Namespace == "" || o.UID == "" {
		return errors.New("owner kind, namespace and uid are required")
	}
	return nil
}

// Spec describes a client.
type Spec struct {
	// Owner scopes cached clients; ignored by New.
	Owner Owner
	// Profile names a registered Profile; ignored by New.
	Profile string
	// APIURL is the base URL of the API the client is used for.
	APIURL string
	Auth   Auth
	// CA resolves additional trusted CAs. Nil means system roots only.
	CA CAResolver
}

func (s Spec) validate() error {
	if err := validateURL("api url", s.APIURL); err != nil {
		return err
	}
	switch s.Auth.Mode {
	case AuthNone:
		return nil
	case AuthClientCredentials, AuthPassword:
		return validateURL("token url", s.Auth.TokenURL)
	default:
		return fmt.Errorf("unknown auth mode %q", s.Auth.Mode)
	}
}

func validateURL(name, raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("parsing %s: %w", name, err)
	}
	if u.Scheme == "" || u.Host == "" {
		return fmt.Errorf("%s %q must be absolute", name, raw)
	}
	return nil
}

// fingerprint covers every input that influences the built client.
func fingerprint(profile string, s Spec, caPEM []byte) string {
	h := sha256.New()
	ca := sha256.Sum256(caPEM)
	for _, field := range []string{
		profile, s.APIURL, string(s.Auth.Mode), s.Auth.TokenURL, s.Auth.ClientID,
		s.Auth.ClientSecret, s.Auth.Username, s.Auth.Password, string(ca[:]),
	} {
		var n [8]byte
		binary.BigEndian.PutUint64(n[:], uint64(len(field)))
		h.Write(n[:])
		h.Write([]byte(field))
	}
	return hex.EncodeToString(h.Sum(nil))
}
