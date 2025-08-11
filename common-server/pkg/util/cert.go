// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package util

import (
	"crypto/x509"
	"fmt"
	"os"

	"github.com/pkg/errors"
)

// GetCert creates a new x509.CertPool and adds the system's default CA certificates
// and the CA certificate from the specified file.
func GetCert(filepath string) (*x509.CertPool, error) {
	caCert, err := os.ReadFile(filepath)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read CA certificate")
	}

	caCertPool, err := x509.SystemCertPool()
	if err != nil {
		fmt.Println("ℹ️\tInfo: Using empty cert pool, system cert pool not available")
		caCertPool = x509.NewCertPool()
	}
	if !caCertPool.AppendCertsFromPEM(caCert) {
		return nil, errors.New("failed to append CA certificate to pool")
	}
	return caCertPool, nil
}
