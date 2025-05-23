// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package identity_client

import (
	"context"
	"fmt"
	"time"

	cclient "github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/types"
	"github.com/telekom/controlplane/common/pkg/util/contextutil"
	identityv1 "github.com/telekom/controlplane/identity/api/v1"
	organisationv1 "github.com/telekom/controlplane/organization/api/v1"
	"github.com/telekom/controlplane/organization/internal/handler/team/handler"
	"github.com/telekom/controlplane/organization/internal/handler/util"
	"github.com/telekom/controlplane/organization/internal/secret"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type IdentityClientHandler struct {
}

var _ handler.ObjectHandler = &IdentityClientHandler{}

func (i IdentityClientHandler) CreateOrUpdate(ctx context.Context, owner *organisationv1.Team) error {
	var err error

	identityClient := buildIdentityClientObj(owner)
	k8sClient := cclient.ClientFromContextOrDie(ctx)
	env := contextutil.EnvFromContextOrDie(ctx)
	zoneObj, err := util.GetZoneObjWithTeamInfo(ctx)
	if err != nil {
		return err
	}

	mutate := func() error {
		var teamToken, teamTokenRef string
		var clientSecret string
		var decodedToken token
		identityClient.Spec.ClientId = identityClient.GetName()
		identityClient.Spec.ClientSecret = owner.Spec.Secret
		identityClient.Spec.Realm = zoneObj.Status.TeamApiIdentityRealm
		identityClient.SetLabels(owner.GetLabels())

		clientSecret, err = secret.GetSecretManager().Get(ctx, owner.Spec.Secret)
		if err != nil {
			return err
		}

		if owner.Status.TeamToken != "" {
			teamTokenRef = owner.Status.TeamToken
			teamToken, err = secret.GetSecretManager().Get(ctx, teamTokenRef)
			if err != nil {
				return err
			}
		}

		decodedToken, err = decodeToken(teamToken)
		if decodedToken.ClientSecret != clientSecret || err != nil { // if err != nil, we need to create a new token, since it is the first time a token is generated (secret manager does not know the token format)
			teamToken, err = buildToken(
				identityClient.Spec.ClientId,
				clientSecret,
				env,
				time.Now())
			if err != nil {
				return err
			}
			availableSecrets, err := secret.GetSecretManager().UpsertTeam(ctx, env, owner.GetName())
			if err != nil {
				return err
			}
			teamTokenId, ok := secret.FindSecretId(availableSecrets, secret.TeamToken)
			if !ok {
				return fmt.Errorf("team token not found in available secrets from secret-manager")
			}

			teamTokenRef, err = secret.GetSecretManager().Set(ctx, teamTokenId, teamToken)
			if err != nil {
				return err
			}
		}

		owner.Status.TeamToken = teamTokenRef

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
