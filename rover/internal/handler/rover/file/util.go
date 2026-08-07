// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package file

import (
	"github.com/telekom/controlplane/common/pkg/util/labelutil"
	filev1 "github.com/telekom/controlplane/file/api/v1"
	roverv1 "github.com/telekom/controlplane/rover/api/v1"
)

func FileTypeRefName(fileType string) string {
	return roverv1.MakeFileTypeName(fileType)
}

func MakeName(fileType, ownerName string) string {
	return FileTypeRefName(fileType) + "--" + labelutil.NormalizeValue(ownerName)
}

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
