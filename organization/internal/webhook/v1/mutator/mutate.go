// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package mutator

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/go-logr/logr"
	"github.com/telekom/controlplane/organization/internal/index"
	"k8s.io/apimachinery/pkg/util/uuid"
	"sigs.k8s.io/controller-runtime/pkg/client"

	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	organisationv1 "github.com/telekom/controlplane/organization/api/v1"
	"github.com/telekom/controlplane/organization/internal/secret"
	secretsapi "github.com/telekom/controlplane/secret-manager/api"
	"k8s.io/apimachinery/pkg/api/errors"
)

func wrapCommunicationError(err error, purposeOfCommunication string) error {
	return errors.NewInternalError(fmt.Errorf("failure during communication with secret-manager when doing '%s': '%w'", purposeOfCommunication, err))
}

func MutateSecret(ctx context.Context, env string, teamObj *organisationv1.Team, zoneObj *adminv1.Zone) error {
	log := logr.FromContextOrDiscard(ctx).WithName("mutateSecret")
	var availableSecrets map[string]string
	var clientSecret string

	if !secretsapi.IsRef(teamObj.Spec.Secret) {
		if strings.EqualFold(teamObj.Spec.Secret, secret.KeywordRotate) {
			// generate new secret
			clientSecret = string(uuid.NewUUID())
		} else {
			// use provided secret
			clientSecret = teamObj.Spec.Secret
		}
	} else {
		log.V(1).Info("spec.secret is already a reference, nothing to do")
		return nil
	}
	clientSecretValue, teamToken, err := generateNewToken(env, teamObj, zoneObj, clientSecret)
	if err != nil {
		return fmt.Errorf("unable to generate team token: %w", err)
	}
	// Pass both secrets directly in the onboarding request
	availableSecrets, err = secret.GetSecretManager().UpsertTeam(ctx, env, teamObj.GetName(),
		secret.WithSecretValue(secret.ClientSecret, clientSecretValue),
		secret.WithSecretValue(secret.TeamToken, teamToken))
	if err != nil {
		return wrapCommunicationError(err, "upsert team")
	}

	var ok bool
	secretRef, ok := secret.FindSecretId(availableSecrets, secret.ClientSecret)
	if !ok {
		return wrapCommunicationError(fmt.Errorf("client secret ref not found in available secrets from secret-manager"), "searching for client secret ref")
	}
	teamObj.Spec.Secret = secretRef
	// Due to status not being able to be set in the webhook, we will set the team-token in the identity-client handler

	return nil
}

func generateNewToken(env string, teamObj *organisationv1.Team, zoneObj *adminv1.Zone, clientSecret string) (string, string, error) {
	if teamObj == nil {
		return "", "", errors.NewInternalError(fmt.Errorf("teamObj is nil"))
	}

	if zoneObj == nil {
		return "", "", errors.NewInternalError(fmt.Errorf("zoneObj is nil"))
	}

	teamToken, err := organisationv1.EncodeTeamToken(
		organisationv1.TeamToken{
			ClientId:     teamObj.GetName(),
			ClientSecret: clientSecret,
			Environment:  env,
			GeneratedAt:  time.Now().Unix(),
			ServerUrl:    zoneObj.Status.Links.Url,
			TokenUrl:     zoneObj.Status.Links.TeamIssuer + "/protocol/openid-connect/token",
		}, teamObj.Spec.Group, teamObj.Spec.Name)

	return clientSecret, teamToken, err

}

func GetZoneObjWithTeamInfo(ctx context.Context, k8sClient client.Client) (*adminv1.Zone, error) {
	var teamApiZone *adminv1.Zone = nil
	zoneList := &adminv1.ZoneList{}

	if k8sClient == nil {
		return nil, errors.NewInternalError(fmt.Errorf("k8sClient is nil"))
	}

	err := k8sClient.List(ctx, zoneList, client.MatchingFields{index.FieldSpecTeamApis: "true"})
	if err != nil {
		return nil, errors.NewInternalError(err)
	}

	for _, zone := range zoneList.GetItems() {
		zoneObj, ok := zone.(*adminv1.Zone)
		if !ok {
			continue
		}
		if zoneObj.Spec.TeamApis != nil {
			teamApiZone = zoneObj.DeepCopy()
			break
		}
	}

	if teamApiZone == nil {
		return nil, errors.NewInternalError(fmt.Errorf("found no zone with team apis"))
	}

	return teamApiZone, nil
}
