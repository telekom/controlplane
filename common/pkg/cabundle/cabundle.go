// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

// Package cabundle validates and resolves custom CA bundles referenced from ConfigMaps.
package cabundle

import (
	"bytes"
	"crypto/sha256"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"slices"

	corev1 "k8s.io/api/core/v1"
)

const (
	// ManagedBundleName is the name of the ConfigMap holding the managed Zone CA bundle.
	ManagedBundleName = "zone-ca-bundle"
	// ManagedBundleKey is the data key of the managed Zone CA bundle.
	ManagedBundleKey = "ca-bundle.pem"

	// MaxConfigMapDataBytes is the Kubernetes limit for the summed length of all
	// keys and values of a ConfigMap (data and binaryData).
	MaxConfigMapDataBytes = corev1.MaxSecretSize
	// MaxBundleBytes is the largest canonical bundle that fits into the managed
	// ConfigMap under ManagedBundleKey.
	MaxBundleBytes = MaxConfigMapDataBytes - len(ManagedBundleKey)
)

var (
	// ErrInvalidBundle indicates invalid CA bundle content. Not retryable.
	ErrInvalidBundle = errors.New("invalid CA bundle")
	// ErrSourceNotFound indicates a missing ConfigMap or key. Not retryable until the source changes.
	ErrSourceNotFound = errors.New("CA bundle source not found")
	// ErrSourceForbidden indicates the ConfigMap could not be read due to missing permissions.
	ErrSourceForbidden = errors.New("CA bundle source forbidden")
)

// ValidationError describes why a CA bundle is invalid. It matches ErrInvalidBundle.
type ValidationError struct {
	// Reason is a human readable explanation.
	Reason string
}

func (e *ValidationError) Error() string {
	return ErrInvalidBundle.Error() + ": " + e.Reason
}

func (e *ValidationError) Is(target error) bool {
	return target == ErrInvalidBundle
}

func invalidf(format string, a ...any) error {
	return &ValidationError{Reason: fmt.Sprintf(format, a...)}
}

// Canonicalize validates every given PEM set and returns the deduplicated union
// as canonical PEM ordered by the SHA-256 of the DER encoding.
//
// Each set must contain at least one certificate and nothing but CERTIFICATE
// blocks (without headers) and whitespace. Every certificate must be a CA
// (valid basic constraints with IsCA, and if key usage is set it must include
// certificate signing). Validity timestamps are not checked.
// The result must not exceed MaxBundleBytes.
// Without sets, nil is returned, meaning system roots only.
func Canonicalize(pemSets ...[]byte) ([]byte, error) {
	unique := map[[sha256.Size]byte][]byte{}
	for i, set := range pemSets {
		ders, err := parseSet(set)
		if err != nil {
			return nil, fmt.Errorf("bundle %d: %w", i, err)
		}
		for _, der := range ders {
			unique[sha256.Sum256(der)] = der
		}
	}
	if len(unique) == 0 {
		return nil, nil
	}

	keys := make([][sha256.Size]byte, 0, len(unique))
	for k := range unique {
		keys = append(keys, k)
	}
	slices.SortFunc(keys, func(a, b [sha256.Size]byte) int { return bytes.Compare(a[:], b[:]) })

	var out bytes.Buffer
	for _, k := range keys {
		if err := pem.Encode(&out, &pem.Block{Type: "CERTIFICATE", Bytes: unique[k]}); err != nil {
			return nil, fmt.Errorf("encoding certificate: %w", err)
		}
	}
	if out.Len() > MaxBundleBytes {
		return nil, invalidf("canonical bundle has %d bytes, exceeding limit of %d", out.Len(), MaxBundleBytes)
	}
	return out.Bytes(), nil
}

func parseSet(data []byte) ([][]byte, error) {
	var ders [][]byte
	rest := bytes.TrimSpace(data)
	for len(rest) > 0 {
		block, next, err := decodeNext(rest)
		if err != nil {
			return nil, err
		}
		if block.Type != "CERTIFICATE" {
			return nil, invalidf("unsupported PEM block type %q", block.Type)
		}
		if len(block.Headers) > 0 {
			return nil, invalidf("PEM headers are not allowed")
		}
		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return nil, invalidf("certificate %d cannot be parsed: %v", len(ders), err)
		}
		if err := checkCA(cert); err != nil {
			return nil, err
		}
		ders = append(ders, cert.Raw)
		rest = bytes.TrimSpace(next)
	}
	if len(ders) == 0 {
		return nil, invalidf("no certificates found")
	}
	return ders, nil
}

var (
	pemBegin    = []byte("-----BEGIN ")
	pemEnd      = []byte("-----END ")
	pemBoundary = []byte("-----")
)

// decodeNext decodes the PEM block at the very start of data and returns the
// remaining input. Unlike pem.Decode alone it never skips content: the block
// must start at the beginning of data, end with the END line matching its
// type, contain no other BEGIN line and decode as a whole.
func decodeNext(data []byte) (*pem.Block, []byte, error) {
	if !bytes.HasPrefix(data, pemBegin) {
		return nil, nil, invalidf("unexpected content outside of PEM blocks")
	}
	lineEnd := bytes.IndexByte(data, '\n')
	if lineEnd < 0 {
		return nil, nil, invalidf("malformed PEM block")
	}
	typeLine := bytes.TrimRight(data[len(pemBegin):lineEnd], " \t\r")
	typ, ok := bytes.CutSuffix(typeLine, pemBoundary)
	if !ok || len(typ) == 0 {
		return nil, nil, invalidf("malformed PEM BEGIN line")
	}
	endMarker := slices.Concat(pemEnd, typ, pemBoundary)
	bodyEnd := bytes.Index(data[lineEnd:], endMarker)
	if bodyEnd < 0 {
		return nil, nil, invalidf("unmatched PEM BEGIN line for type %q", typ)
	}
	bodyEnd += lineEnd
	if bytes.Contains(data[lineEnd:bodyEnd], pemBegin) {
		return nil, nil, invalidf("nested or unmatched PEM BEGIN line")
	}
	blockEnd := bodyEnd + len(endMarker)
	block, trailing := pem.Decode(data[:blockEnd])
	if block == nil || len(bytes.TrimSpace(trailing)) > 0 {
		return nil, nil, invalidf("malformed PEM block")
	}
	return block, data[blockEnd:], nil
}

func checkCA(cert *x509.Certificate) error {
	if !cert.BasicConstraintsValid || !cert.IsCA {
		return invalidf("certificate %q is not a CA", cert.Subject.String())
	}
	if cert.KeyUsage != 0 && cert.KeyUsage&x509.KeyUsageCertSign == 0 {
		return invalidf("CA certificate %q lacks certificate signing key usage", cert.Subject.String())
	}
	return nil
}
