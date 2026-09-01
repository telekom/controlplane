// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package file

import (
	"github.com/telekom/controlplane/common/pkg/util/labelutil"
	filev1 "github.com/telekom/controlplane/file/api/v1"
	roverv1 "github.com/telekom/controlplane/rover/api/v1"
)

// MakeName generates a deterministic resource name for a file exposure or
// subscription: "<fileType>--<owner>" (spec_dcp naming), normalized.
func MakeName(fileType, ownerName string) string {
	return roverv1.MakeFileTypeName(fileType) + "--" + labelutil.NormalizeValue(ownerName)
}

// mapPublicKeys converts rover-domain public keys to file-domain public keys.
func mapPublicKeys(in []roverv1.PublicKey) []filev1.SSHPublicKeySpec {
	if len(in) == 0 {
		return nil
	}
	out := make([]filev1.SSHPublicKeySpec, len(in))
	for i, k := range in {
		out[i] = filev1.SSHPublicKeySpec{Key: k.Key}
	}
	return out
}
