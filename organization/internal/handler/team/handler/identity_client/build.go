// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package identity_client

import (
	identityv1 "github.com/telekom/controlplane/identity/api/v1"
	organisationv1 "github.com/telekom/controlplane/organization/api/v1"
	"github.com/telekom/controlplane/organization/internal/handler/team/handler"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const TeamNameSuffix = "team-user"

func buildIdentityClientObj(owner *organisationv1.Team) *identityv1.Client {
	name := owner.Spec.Group + handler.Separator + owner.Spec.Name + handler.Separator + TeamNameSuffix
	return &identityv1.Client{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: owner.Status.Namespace,
		},
	}
}
