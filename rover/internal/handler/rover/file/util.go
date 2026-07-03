// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package file

import (
	filev1 "github.com/telekom/controlplane/file/api/v1"
	roverv1 "github.com/telekom/controlplane/rover/api/v1"
)

// MakeName generates a deterministic resource name for a file exposure or
// subscription: "<fileType>--<owner>" (spec_dcp naming), normalized.
func MakeName(fileType, ownerName string) string {
	return filev1.MakeFileTypeName(fileType) + "--" + ownerName
}

// mapPublicKeys converts rover-domain public keys to file-domain public keys.
func mapPublicKeys(in []roverv1.PublicKey) []filev1.PublicKey {
	if len(in) == 0 {
		return nil
	}
	out := make([]filev1.PublicKey, len(in))
	for i, k := range in {
		out[i] = filev1.PublicKey{Label: k.Label, Key: k.Key}
	}
	return out
}
