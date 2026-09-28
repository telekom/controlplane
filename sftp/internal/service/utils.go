// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"fmt"
	"net/url"
	"strings"
)

func parseBaseURL(rawBaseURL string) (*url.URL, error) {
	baseURL, err := url.Parse(strings.TrimRight(rawBaseURL, "/"))
	if err != nil {
		return nil, fmt.Errorf("parsing SFTP Tardis base URL: %w", err)
	}
	if baseURL.Scheme == "" || baseURL.Host == "" {
		return nil, fmt.Errorf("SFTP Tardis base URL must include scheme and host")
	}
	return baseURL, nil
}
