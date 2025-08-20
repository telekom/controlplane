// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package client

import "time"

type Options struct {
	// ClientName is used for metrics.
	ClientName string
	// ReplacePattern is used to obfuscate parts of the URL-path
	// used in metrics collections.
	ReplacePattern string
	// SkipTlsVerify skips TLS verification.
	SkipTlsVerify bool
	// CaFilepath is the path to the CA certificate file.
	// If empty, the system's default CA certificates are used.
	// If SkipTlsVerify is true, this option is ignored.
	CaFilepath string
	// ClientTimeout is the timeout for HTTP requests.
	ClientTimeout time.Duration
}

type Option func(*Options)

func WithClientName(name string) Option {
	return func(o *Options) {
		o.ClientName = name
	}
}

func WithReplacePattern(pattern string) Option {
	return func(o *Options) {
		o.ReplacePattern = pattern
	}
}

func WithSkipTlsVerify(skip bool) Option {
	return func(o *Options) {
		o.SkipTlsVerify = skip
	}
}

func WithCaFilepath(caFilepath string) Option {
	return func(o *Options) {
		o.CaFilepath = caFilepath
	}
}

func WithClientTimeout(timeout time.Duration) Option {
	return func(o *Options) {
		o.ClientTimeout = timeout
	}
}
