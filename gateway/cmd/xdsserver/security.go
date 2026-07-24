// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package xdsserver

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/url"
	"os"
	"sync"

	"sigs.k8s.io/yaml"
)

type relayAssignments map[string]map[string]struct{}

func loadRelayAssignments(path string) (relayAssignments, error) {
	contents, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var raw map[string][]string
	if err := yaml.Unmarshal(contents, &raw); err != nil {
		return nil, fmt.Errorf("decoding assignments: %w", err)
	}
	assignments := make(relayAssignments, len(raw))
	for identity, nodeIDs := range raw {
		uri, err := url.Parse(identity)
		if err != nil || uri.Scheme == "" {
			return nil, fmt.Errorf("assignment identity %q is not an absolute URI", identity)
		}
		if len(nodeIDs) == 0 {
			return nil, fmt.Errorf("assignment identity %q has no node IDs", identity)
		}
		assignments[identity] = make(map[string]struct{}, len(nodeIDs))
		for _, nodeID := range nodeIDs {
			if nodeID == "" {
				return nil, fmt.Errorf("assignment identity %q has an empty node ID", identity)
			}
			assignments[identity][nodeID] = struct{}{}
		}
	}
	return assignments, nil
}

type tlsReloader struct {
	certificateFile string
	keyFile         string
	clientCAFile    string
	mu              sync.RWMutex
	current         *tls.Config
}

func newTLSReloader(certificateFile, keyFile, clientCAFile string) (*tlsReloader, error) {
	r := &tlsReloader{certificateFile: certificateFile, keyFile: keyFile, clientCAFile: clientCAFile}
	config, err := r.load()
	if err != nil {
		return nil, err
	}
	r.current = config
	return r, nil
}

func (r *tlsReloader) getConfigForClient(*tls.ClientHelloInfo) (*tls.Config, error) {
	if config, err := r.load(); err == nil {
		r.mu.Lock()
		r.current = config
		r.mu.Unlock()
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.current, nil
}

func (r *tlsReloader) load() (*tls.Config, error) {
	certificate, err := tls.LoadX509KeyPair(r.certificateFile, r.keyFile)
	if err != nil {
		return nil, err
	}
	caPEM, err := os.ReadFile(r.clientCAFile)
	if err != nil {
		return nil, err
	}
	clientCAs := x509.NewCertPool()
	if !clientCAs.AppendCertsFromPEM(caPEM) {
		return nil, fmt.Errorf("client CA file contains no certificates")
	}
	return &tls.Config{
		MinVersion:   tls.VersionTLS12,
		Certificates: []tls.Certificate{certificate},
		ClientCAs:    clientCAs,
		ClientAuth:   tls.RequireAndVerifyClientCert,
		NextProtos:   []string{"h2"},
	}, nil
}
