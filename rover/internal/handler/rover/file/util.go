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
	return fileType + "--" + labelutil.NormalizeValue(ownerName)
}

// mapSFTP converts rover-domain public keys to file-domain public keys.
func mapSFTP(in *roverv1.FileSFTP) *filev1.FileSFTP {
	if in == nil || len(in.PublicKeys) == 0 {
		return nil
	}

	out := &filev1.FileSFTP{
		PublicKeys: make([]filev1.SSHPublicKeySpec, len(in.PublicKeys)),
	}

	for i, k := range in.PublicKeys {
		out.PublicKeys[i] = filev1.SSHPublicKeySpec{
			Key: k.Key,
		}
	}
	return out
}
