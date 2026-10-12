// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

// Package tlsutil creates throwaway CAs and TLS servers for tests.
package tlsutil

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"time"
)

// CA is a test certificate authority.
type CA struct {
	Cert *x509.Certificate
	Key  *ecdsa.PrivateKey
	PEM  string
}

// NewCA creates a self-signed CA certificate.
func NewCA(name string) (*CA, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(time.Now().UnixNano()),
		Subject:               pkix.Name{CommonName: name},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return nil, err
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, err
	}
	return &CA{Cert: cert, Key: key, PEM: string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))}, nil
}

// ServerCertificate issues a TLS server certificate for 127.0.0.1 and localhost.
func (ca *CA) ServerCertificate() (tls.Certificate, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return tls.Certificate{}, err
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject:      pkix.Name{CommonName: "localhost"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:     []string{"localhost"},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, ca.Cert, &key.PublicKey, ca.Key)
	if err != nil {
		return tls.Certificate{}, err
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}, nil
}

// NewTLSServer starts an HTTPS server whose certificate is issued by ca.
func NewTLSServer(ca *CA, h http.Handler) (*httptest.Server, error) {
	cert, err := ca.ServerCertificate()
	if err != nil {
		return nil, err
	}
	srv := httptest.NewUnstartedServer(h)
	srv.TLS = &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS13}
	srv.StartTLS()
	return srv, nil
}

// RotatingServer is an HTTPS server whose certificate can be replaced while it keeps its URL.
type RotatingServer struct {
	*httptest.Server
	cert atomic.Pointer[tls.Certificate]
}

// NewRotatingTLSServer starts an HTTPS server with a certificate issued by ca.
func NewRotatingTLSServer(ca *CA, h http.Handler) (*RotatingServer, error) {
	s := &RotatingServer{}
	if err := s.Rotate(ca); err != nil {
		return nil, err
	}
	s.Server = httptest.NewUnstartedServer(h)
	s.TLS = &tls.Config{
		MinVersion: tls.VersionTLS13,
		// GetConfigForClient wins over the default certificate StartTLS adds, also without SNI.
		GetConfigForClient: func(*tls.ClientHelloInfo) (*tls.Config, error) {
			return &tls.Config{MinVersion: tls.VersionTLS13, Certificates: []tls.Certificate{*s.cert.Load()}}, nil
		},
	}
	s.StartTLS()
	return s, nil
}

// Rotate serves a new certificate issued by ca for subsequent handshakes.
func (s *RotatingServer) Rotate(ca *CA) error {
	cert, err := ca.ServerCertificate()
	if err != nil {
		return err
	}
	s.cert.Store(&cert)
	if s.Server != nil {
		s.CloseClientConnections()
	}
	return nil
}
