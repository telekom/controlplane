// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package util

import apiapi "github.com/telekom/controlplane/api/api/v1"

func HasM2MRemote(remoteApiSub *apiapi.RemoteApiSubscription) bool {
	if remoteApiSub.Spec.Security == nil {
		return false
	}

	return remoteApiSub.Spec.Security.M2M != nil
}

func HasM2MClientRemote(remoteApiSub *apiapi.RemoteApiSubscription) bool {
	if !HasM2MRemote(remoteApiSub) {
		return false
	}

	return remoteApiSub.Spec.Security.M2M.Client != nil
}
