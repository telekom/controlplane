// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package model

// FileSFTP represents the fileSFTP config from the file domain
type FileSFTP struct {
	PublicKeys []SSHPublicKeySpec `json:"publicKeys,omitempty"`
}

type SSHPublicKeySpec struct {
	Key string `json:"key"`
}
