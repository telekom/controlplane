// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package client

import (
	"net/http"
	"time"

	"github.com/Khan/genqlient/graphql"

	commonclient "github.com/telekom/controlplane/common-server/pkg/client"
	accesstoken "github.com/telekom/controlplane/common-server/pkg/client/token"
	"github.com/telekom/controlplane/common-server/pkg/server/middleware/security"
)

// NewCPAPIClient creates a genqlient GraphQL client that talks to CP API's
// internal (Kubernetes-authz) listener using the projected ServiceAccount token
// at tokenFilePath, and forwards consumer identity via X-Forwarded-* headers
// (extracted from context).
//
// CP API's internal listener builds a synthetic admin BusinessContext from the
// mandatory X-Environment header, so every request carries it.
// caFilePath is the CA bundle for TLS verification; empty means system CAs.
func NewCPAPIClient(endpoint string, token accesstoken.AccessToken, caFilePath string) graphql.Client {
	baseClient := commonclient.NewBaseHttpClient(
		commonclient.WithCaFilepath(caFilePath),
		commonclient.WithClientName("cpapi"),
		commonclient.WithClientTimeout(15*time.Second),
	)

	baseClient.Transport = &tokenTransport{
		token:    token,
		base:     baseClient.Transport,
		decorate: forwardIdentity,
	}
	return graphql.NewClient(endpoint, baseClient)
}

// forwardIdentity copies the consumer identity from the request context onto
// the outgoing headers. X-Environment is required by the internal listener's
// synthetic admin business context; the X-Forwarded-* headers tell CP API on
// whose behalf the facade is acting.
func forwardIdentity(req *http.Request) {
	bCtx, ok := security.FromContext(req.Context())
	if !ok {
		return
	}
	if bCtx.Environment != "" {
		req.Header.Set("X-Environment", bCtx.Environment)
		req.Header.Set("X-Forwarded-Environment", bCtx.Environment)
	}
	if bCtx.Group != "" {
		req.Header.Set("X-Forwarded-Group", bCtx.Group)
	}
	if bCtx.Team != "" {
		req.Header.Set("X-Forwarded-Team", bCtx.Team)
	}
}
