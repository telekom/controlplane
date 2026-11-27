// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package files

import (
	accesstoken "github.com/telekom/controlplane/common-server/pkg/client/token"
	"github.com/telekom/controlplane/file-manager/api"
)

// NewFileManagerClient builds the file-manager client used by the download
// proxy. The service-account token is read from tokenFilePath and sent to
// file-manager as bearer token; inbound client credentials are never forwarded.
func NewFileManagerClient(baseURL, tokenFilePath string, opts ...api.Option) api.DownloadApi {
	base := []api.Option{
		api.WithURL(baseURL),
		api.WithAccessToken(accesstoken.NewAccessToken(tokenFilePath)),
	}
	return api.New(append(base, opts...)...)
}
