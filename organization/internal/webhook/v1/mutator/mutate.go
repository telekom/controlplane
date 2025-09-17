// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package mutator

import (
	"context"
	"fmt"
	"time"

	"github.com/telekom/controlplane/organization/internal/index"
	"k8s.io/apimachinery/pkg/util/uuid"
	"sigs.k8s.io/controller-runtime/pkg/client"

	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	organisationv1 "github.com/telekom/controlplane/organization/api/v1"
	"github.com/telekom/controlplane/organization/internal/secret"
	"k8s.io/apimachinery/pkg/api/errors"
)

func wrapCommunicationError(err error, target, purposeOfCommunication string) error {
	return errors.NewInternalError(fmt.Errorf("failure during communication with %s when doing '%s': '%w'", target, purposeOfCommunication, err))
}

func MutateSecret(ctx context.Context, client client.Client, env string, teamObj *organisationv1.Team) error {
	var err error
	var availableSecrets map[string]string

	switch teamObj.Spec.Secret {
	case "":
		// ToDo: Try to get Zone somehow
		zone, err := GetZoneObjWithTeamInfo(ctx, client)
		if err != nil {
			return wrapCommunicationError(err, "admin domain", "get zone")
		}

		clientSecretValue, teamToken, err := generateSecretAndToken(env, teamObj, zone)
		if err != nil {
			return fmt.Errorf("unable to generate team token: %w", err)
		}
		// Pass both secrets directly in the onboarding request
		availableSecrets, err = secret.GetSecretManager().UpsertTeam(ctx, env, teamObj.GetName(),
			secret.WithSecretValue(secret.ClientSecret, clientSecretValue),
			secret.WithSecretValue(secret.TeamToken, teamToken))
		if err != nil {
			return wrapCommunicationError(err, "secret-manager", "upsert team")
		}

		var ok bool
		teamObj.Spec.Secret, ok = secret.FindSecretId(availableSecrets, secret.ClientSecret)
		if !ok {
			return wrapCommunicationError(fmt.Errorf("client secret ref not found in available secrets from secret-manager"), "secret-manager", "searching for client secret ref")
		}
		teamObj.Status.TeamToken, ok = secret.FindSecretId(availableSecrets, secret.TeamToken) //ToDo: Check if this even works with the status
		if !ok {
			return wrapCommunicationError(fmt.Errorf("team token ref not found in available secrets from secret-manager"), "secret-manager", "searching for team token ref")
		}
	case secret.KeywordRotate:
		//ToDo: I don't have the new secret, since the rotation never left the secret-manager.
		var newId string
		availableSecrets, err = secret.GetSecretManager().UpsertTeam(ctx, env, teamObj.GetName())
		if err != nil {
			return wrapCommunicationError(err, "secret-manager", "checking available secrets")
		}

		clientSecretRef, ok := secret.FindSecretId(availableSecrets, secret.ClientSecret)
		if !ok {
			return wrapCommunicationError(fmt.Errorf("client secret ref not found in available secrets from secret-manager"), "secret-manager", "searching for client secret ref")
		}
		newId, err = secret.GetSecretManager().Rotate(ctx, clientSecretRef)
		if err != nil {
			return wrapCommunicationError(err, "secret-manager", "rotate team secret")
		}
		teamObj.Spec.Secret = newId
	}
	return nil
}

func generateSecretAndToken(env string, teamObj *organisationv1.Team, zoneObj *adminv1.Zone) (string, string, error) {
	clientSecretValue := string(uuid.NewUUID())

	teamToken, err := organisationv1.EncodeTeamToken(
		organisationv1.TeamToken{
			ClientId:     teamObj.GetName(),
			ClientSecret: clientSecretValue,
			Environment:  env,
			GeneratedAt:  time.Now().Unix(),
			ServerUrl:    zoneObj.Spec.Gateway.Url,
			TokenUrl:     zoneObj.Spec.IdentityProvider.Url,
		}, teamObj.Spec.Group, teamObj.Spec.Name)

	return clientSecretValue, teamToken, err

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
		if zone.(*adminv1.Zone).Spec.TeamApis != nil {
			teamApiZone = zone.(*adminv1.Zone).DeepCopy()
			break
		}
	}

	if teamApiZone == nil {
		return nil, errors.NewInternalError(fmt.Errorf("found no zone with team apis"))
	}

	return teamApiZone, nil
}
