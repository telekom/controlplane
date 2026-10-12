// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package client

import (
	"bytes"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"time"
)

const (
	// MaxProviderCABundleBytes bounds the selected PEM input.
	MaxProviderCABundleBytes = 256 * 1024
	// MaxProviderCABundleCerts bounds the number of certificates in a bundle.
	MaxProviderCABundleCerts = 64
	// DefaultProviderIdleConnTimeout bounds idle connections of retired clients.
	DefaultProviderIdleConnTimeout = 90 * time.Second
)

// ErrRedirectNotAllowed is returned when a credential-bearing client is redirected.
var ErrRedirectNotAllowed = errors.New("redirects are not allowed")

// ProviderCABundle is an immutable, validated set of provider trust anchors.
type ProviderCABundle struct {
	certs      []*x509.Certificate
	exactLeafs map[string]struct{}
	digest     string
	pem        []byte
}

// ParseProviderCABundle strictly parses PEM CERTIFICATE blocks.
// It rejects empty input, malformed DER, non-certificate blocks, PEM headers and garbage,
// and inputs exceeding MaxProviderCABundleBytes or MaxProviderCABundleCerts.
// CA anchors need valid CA basic constraints and CertSign when KeyUsage is present.
// Non-CA anchors must be validly self-signed; they only authenticate themselves.
func ParseProviderCABundle(data []byte) (*ProviderCABundle, error) {
	if len(data) > MaxProviderCABundleBytes {
		return nil, fmt.Errorf("CA bundle exceeds %d bytes", MaxProviderCABundleBytes)
	}
	rest := data
	seen := map[string]struct{}{}
	b := &ProviderCABundle{exactLeafs: map[string]struct{}{}}
	blocks := 0
	for {
		rest = bytes.TrimLeft(rest, " \t\r\n")
		if len(rest) == 0 {
			break
		}
		der, next, err := nextCertificateBlock(rest)
		if err != nil {
			return nil, err
		}
		rest = next
		blocks++
		if blocks > MaxProviderCABundleCerts {
			return nil, fmt.Errorf("CA bundle exceeds %d certificates", MaxProviderCABundleCerts)
		}
		cert, err := x509.ParseCertificate(der)
		if err != nil {
			return nil, fmt.Errorf("parsing CA certificate %d: %w", blocks, err)
		}
		exactLeaf, err := validateAnchor(cert)
		if err != nil {
			return nil, fmt.Errorf("CA certificate %d: %w", blocks, err)
		}
		key := string(cert.Raw)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		if exactLeaf {
			b.exactLeafs[key] = struct{}{}
		}
		b.certs = append(b.certs, cert)
	}
	if len(b.certs) == 0 {
		return nil, errors.New("CA bundle contains no certificates")
	}
	slices.SortFunc(b.certs, func(x, y *x509.Certificate) int { return bytes.Compare(x.Raw, y.Raw) })
	var canon, out bytes.Buffer
	for _, c := range b.certs {
		canon.WriteString(strconv.Itoa(len(c.Raw)) + ":")
		canon.Write(c.Raw)
		if err := pem.Encode(&out, &pem.Block{Type: "CERTIFICATE", Bytes: c.Raw}); err != nil {
			return nil, fmt.Errorf("encoding canonical PEM: %w", err)
		}
	}
	sum := sha256.Sum256(canon.Bytes())
	b.digest = hex.EncodeToString(sum[:])
	b.pem = out.Bytes()
	return b, nil
}

var (
	pemCertBegin = []byte("-----BEGIN CERTIFICATE-----")
	pemCertEnd   = []byte("-----END CERTIFICATE-----")
	pemBoundary  = []byte("-----")
)

// nextCertificateBlock frames exactly one headerless CERTIFICATE block at the start of in
// and returns its DER and the remaining input. Unlike pem.Decode it never skips ahead:
// the block must start at in[0], contain only base64 and whitespace, and be closed by a
// matching END line followed by whitespace or end of input.
func nextCertificateBlock(in []byte) (der, rest []byte, err error) {
	if !bytes.HasPrefix(in, pemBoundary) {
		return nil, nil, errors.New("CA bundle contains data outside PEM blocks")
	}
	if !bytes.HasPrefix(in, pemCertBegin) {
		return nil, nil, errors.New("CA bundle contains a non-CERTIFICATE or malformed PEM block")
	}
	body := in[len(pemCertBegin):]
	switch {
	case bytes.HasPrefix(body, []byte("\r\n")):
		body = body[2:]
	case bytes.HasPrefix(body, []byte("\n")):
		body = body[1:]
	default:
		return nil, nil, errors.New("CA bundle contains a malformed PEM BEGIN line")
	}
	end := bytes.Index(body, pemBoundary)
	if end < 0 || !bytes.HasPrefix(body[end:], pemCertEnd) {
		return nil, nil, errors.New("CA bundle contains an unterminated or nested PEM block")
	}
	if end > 0 && body[end-1] != '\n' {
		return nil, nil, errors.New("CA bundle contains a malformed PEM END line")
	}
	rest = body[end+len(pemCertEnd):]
	if len(rest) > 0 && !bytes.ContainsAny(rest[:1], " \t\r\n") {
		return nil, nil, errors.New("CA bundle contains data after a PEM END line")
	}
	b64 := bytes.Map(func(r rune) rune {
		if r == ' ' || r == '\t' || r == '\r' || r == '\n' {
			return -1
		}
		return r
	}, body[:end])
	der = make([]byte, base64.StdEncoding.DecodedLen(len(b64)))
	n, err := base64.StdEncoding.Strict().Decode(der, b64)
	if err != nil || n == 0 {
		return nil, nil, errors.New("CA bundle contains a malformed PEM body")
	}
	return der[:n], rest, nil
}

func validateAnchor(cert *x509.Certificate) (exactLeaf bool, err error) {
	if cert.BasicConstraintsValid && cert.IsCA {
		if cert.KeyUsage != 0 && cert.KeyUsage&x509.KeyUsageCertSign == 0 {
			return false, errors.New("CA certificate lacks CertSign key usage")
		}
		return false, nil
	}
	if !bytes.Equal(cert.RawIssuer, cert.RawSubject) {
		return false, errors.New("non-CA certificate is not self-signed")
	}
	if err := cert.CheckSignatureFrom(selfAsCA(cert)); err != nil {
		return false, fmt.Errorf("invalid self-signature: %w", err)
	}
	return true, nil
}

// selfAsCA lets CheckSignatureFrom verify a non-CA self-signature without trusting it as an issuer.
func selfAsCA(cert *x509.Certificate) *x509.Certificate {
	c := *cert
	c.BasicConstraintsValid = true
	c.IsCA = true
	c.KeyUsage = 0
	return &c
}

// MergeProviderCABundles returns the deduplicated union of bundles in canonical order.
// The union is re-validated, so the size and certificate limits apply to the result too.
func MergeProviderCABundles(bundles ...*ProviderCABundle) (*ProviderCABundle, error) {
	seen := map[string]struct{}{}
	var out bytes.Buffer
	for _, b := range bundles {
		if b == nil {
			return nil, errors.New("merging CA bundles: bundle is nil")
		}
		for _, c := range b.certs {
			if _, ok := seen[string(c.Raw)]; ok {
				continue
			}
			seen[string(c.Raw)] = struct{}{}
			if err := pem.Encode(&out, &pem.Block{Type: "CERTIFICATE", Bytes: c.Raw}); err != nil {
				return nil, fmt.Errorf("encoding CA bundle union: %w", err)
			}
		}
	}
	return ParseProviderCABundle(out.Bytes())
}

// Digest is the SHA-256 of the sorted, deduplicated certificate DER.
func (b *ProviderCABundle) Digest() string { return b.digest }

// CanonicalPEM returns the deduplicated, sorted PEM representation.
func (b *ProviderCABundle) CanonicalPEM() []byte { return bytes.Clone(b.pem) }

// Len returns the number of distinct certificates.
func (b *ProviderCABundle) Len() int { return len(b.certs) }

// SystemCertPool is replaceable for tests.
var SystemCertPool = x509.SystemCertPool

// RootPool returns a fresh pool of system roots plus the bundle. A nil bundle returns system roots.
// Failure to obtain system roots is an error.
func (b *ProviderCABundle) RootPool() (*x509.CertPool, error) {
	pool, err := SystemCertPool()
	if err != nil {
		return nil, fmt.Errorf("loading system roots: %w", err)
	}
	if pool == nil {
		return nil, errors.New("loading system roots: empty pool")
	}
	if b == nil {
		return pool, nil
	}
	for _, c := range b.certs {
		pool.AddCert(c)
	}
	return pool, nil
}

// TLSConfig returns a fresh TLS client configuration derived from base (may be nil).
// Standard verification stays enabled; a non-CA anchor authenticates only an identical leaf.
func (b *ProviderCABundle) TLSConfig(base *tls.Config) (*tls.Config, error) {
	cfg := &tls.Config{MinVersion: tls.VersionTLS12}
	if base != nil {
		cfg = base.Clone()
	}
	if cfg.InsecureSkipVerify {
		return nil, errors.New("InsecureSkipVerify is not allowed for provider TLS")
	}
	if b == nil {
		return cfg, nil
	}
	pool, err := b.RootPool()
	if err != nil {
		return nil, err
	}
	cfg.RootCAs = pool
	exact := b.exactLeafs
	// VerifyConnection also runs on resumed sessions, unlike VerifyPeerCertificate.
	cfg.VerifyConnection = func(cs tls.ConnectionState) error {
		return verifyExactLeafAnchors(exact, cs.VerifiedChains)
	}
	return cfg, nil
}

func verifyExactLeafAnchors(exact map[string]struct{}, chains [][]*x509.Certificate) error {
	for _, chain := range chains {
		if len(chain) == 0 {
			continue
		}
		root := chain[len(chain)-1]
		if _, isExact := exact[string(root.Raw)]; isExact && len(chain) > 1 {
			continue
		}
		return nil
	}
	return errors.New("certificate signed by a non-CA provider anchor")
}

// NewProviderTransport clones base (or http.DefaultTransport) and applies bundle trust.
func NewProviderTransport(base *http.Transport, b *ProviderCABundle) (*http.Transport, error) {
	if base == nil {
		dt, ok := http.DefaultTransport.(*http.Transport)
		if !ok {
			return nil, errors.New("default transport is not *http.Transport")
		}
		base = dt
	}
	t := base.Clone()
	if t.IdleConnTimeout <= 0 {
		t.IdleConnTimeout = DefaultProviderIdleConnTimeout
	}
	cfg, err := b.TLSConfig(t.TLSClientConfig)
	if err != nil {
		return nil, err
	}
	t.TLSClientConfig = cfg
	return t, nil
}

// DenyRedirects is an http.Client CheckRedirect that rejects all redirects.
func DenyRedirects(*http.Request, []*http.Request) error { return ErrRedirectNotAllowed }
