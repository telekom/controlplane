// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package identity_client

import (
	"context"

	cclient "github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/types"
	identityv1 "github.com/telekom/controlplane/identity/api/v1"
	organisationv1 "github.com/telekom/controlplane/organization/api/v1"
	"github.com/telekom/controlplane/organization/internal/handler/team/handler"
	"github.com/telekom/controlplane/organization/internal/handler/util"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func MakeClientId(owner *organisationv1.Team) string {
	return owner.Spec.Group + handler.Separator + owner.Spec.Name + handler.Separator + TeamNameSuffix
}

type IdentityClientHandler struct {
}

var _ handler.ObjectHandler = &IdentityClientHandler{}

func (i IdentityClientHandler) CreateOrUpdate(ctx context.Context, owner *organisationv1.Team) error {
	var err error

	identityClient := buildIdentityClientObj(owner)
	k8sClient := cclient.ClientFromContextOrDie(ctx)
	zoneObj, err := util.GetZoneObjWithTeamInfo(ctx)
	if err != nil {
		return err
	}

	mutate := func() error {
		identityClient.Spec.ClientId = MakeClientId(owner)
		identityClient.Spec.ClientSecret = owner.Spec.Secret
		identityClient.Spec.Realm = zoneObj.Status.TeamApiIdentityRealm
		identityClient.SetLabels(owner.GetLabels())

		return nil
	}

	if _, err = k8sClient.CreateOrUpdate(ctx, identityClient, mutate); err != nil {
		return err
	}

	owner.Status.IdentityClientRef = types.ObjectRefFromObject(identityClient)

	return nil
}

func (i IdentityClientHandler) Delete(ctx context.Context, owner *organisationv1.Team) error {
	var err error
	k8sClient := cclient.ClientFromContextOrDie(ctx)
	if owner.Status.IdentityClientRef != nil {
		err = k8sClient.Delete(ctx, &identityv1.Client{
			ObjectMeta: metav1.ObjectMeta{
				Name:      owner.Status.IdentityClientRef.GetName(),
				Namespace: owner.Status.IdentityClientRef.GetNamespace(),
			},
		})
	}
	return err
}

func (i IdentityClientHandler) Identifier() string {
	return "identity client"
}
