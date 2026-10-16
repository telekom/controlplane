// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

// Package tlsclient builds HTTP clients that trust custom CA bundles in
// addition to the system roots, optionally authenticated via OAuth2, and
// caches them per owner and configuration.
package tlsclient

import (
	"context"
	"crypto/x509"
	"errors"
	"fmt"
)

// CAResolver returns the validated canonical PEM bundle to trust in addition
// to the system roots. An empty result means system roots only.
type CAResolver interface {
	Resolve(ctx context.Context) ([]byte, error)
}

var (
	// ErrInvalidCA indicates CA bytes that could not be added to a pool.
	ErrInvalidCA = errors.New("invalid CA bundle")
	// ErrFactoryClosed is returned when acquiring from a closed factory.
	ErrFactoryClosed = errors.New("client factory is closed")
)

// systemCertPool is a test seam.
var systemCertPool = x509.SystemCertPool

// NewCertPool returns the system roots extended by caPEM.
// It fails if the system roots cannot be loaded or caPEM contains no certificate.
func NewCertPool(caPEM []byte) (*x509.CertPool, error) {
	pool, err := systemCertPool()
	if err != nil {
		return nil, fmt.Errorf("loading system cert pool: %w", err)
	}
	if pool == nil {
		return nil, errors.New("loading system cert pool: no pool available")
	}
	if len(caPEM) == 0 {
		return pool, nil
	}
	if !pool.AppendCertsFromPEM(caPEM) {
		return nil, fmt.Errorf("%w: no certificates could be added", ErrInvalidCA)
	}
	return pool, nil
}
