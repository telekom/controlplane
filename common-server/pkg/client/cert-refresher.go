// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package client

import (
	"bytes"
	"context"
	"crypto/x509"
	"os"
	"time"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	"github.com/telekom/controlplane/common-server/pkg/util"
)

// CertRefresher is a utility to refresh the CA certificates from a file periodically.
// It is useful for applications that need to maintain a valid certificate pool
type CertRefresher struct {
	Pool     *x509.CertPool
	filepath string
	lastCert []byte
	internal time.Duration
}

func NewCertRefresher(filepath string) *CertRefresher {
	return &CertRefresher{
		filepath: filepath,
		internal: 30 * time.Second,
	}
}

func (c *CertRefresher) Start(ctx context.Context) (err error) {
	c.Pool, err = util.GetCert(c.filepath)
	if err != nil {
		return errors.Wrap(err, "failed to start cert refresher")
	}
	go c.Watch(ctx)

	return nil
}

func (c *CertRefresher) Watch(ctx context.Context) {
	ticker := time.NewTicker(c.internal)
	defer ticker.Stop()

	log := logr.FromContextOrDiscard(ctx)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			caCert, err := os.ReadFile(c.filepath)
			if err != nil {
				log.Error(err, "failed to read cert file")
				continue
			}
			if bytes.Equal(caCert, c.lastCert) {
				log.V(1).Info("cert not changed")
				continue
			}

			if !c.Pool.AppendCertsFromPEM(caCert) {
				log.Info("failed to append certs from PEM")
				continue
			}
			c.lastCert = caCert
			log.V(1).Info("cert updated")
		}
	}
}
