// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package identity_client

import (
	identityv1 "github.com/telekom/controlplane/identity/api/v1"
	organisationv1 "github.com/telekom/controlplane/organization/api/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const TeamNameSuffix = "team-user"

func buildIdentityClientObj(owner *organisationv1.Team) *identityv1.Client {
	return &identityv1.Client{
		ObjectMeta: metav1.ObjectMeta{
			Name:      MakeClientId(owner),
			Namespace: owner.Status.Namespace,
		},
	}
}
