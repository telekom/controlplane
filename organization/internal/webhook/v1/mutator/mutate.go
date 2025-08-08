// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package mutator

import (
	"context"
	"fmt"

	organisationv1 "github.com/telekom/controlplane/organization/api/v1"
	"github.com/telekom/controlplane/organization/internal/secret"
	"k8s.io/apimachinery/pkg/api/errors"
)

func wrapCommunicationError(err error, purposeOfCommunication string) error {
	return errors.NewInternalError(fmt.Errorf("failure during communication with secret-manager when doing '%s': '%w'", purposeOfCommunication, err))
}

func MutateSecret(ctx context.Context, env string, teamObj *organisationv1.Team) error {
	var err error
	var availableSecrets map[string]string

	switch teamObj.Spec.Secret {
	case "":
		availableSecrets, err = secret.GetSecretManager().UpsertTeam(ctx, env, teamObj.GetName())
		if err != nil {
			return wrapCommunicationError(err, "upsert team")
		}

		var ok bool
		teamObj.Spec.Secret, ok = secret.FindSecretId(availableSecrets, secret.ClientSecret)
		if !ok {
			return wrapCommunicationError(fmt.Errorf("client secret ref not found in available secrets from secret-manager"), "searching for client secret ref")
		}
		teamObj.Status.TeamToken, ok = secret.FindSecretId(availableSecrets, secret.TeamToken)
		if !ok {
			return wrapCommunicationError(fmt.Errorf("team token ref not found in available secrets from secret-manager"), "searching for team token ref")
		}
	case secret.KeywordRotate:
		var newId string
		availableSecrets, err = secret.GetSecretManager().UpsertTeam(ctx, env, teamObj.GetName())
		if err != nil {
			return wrapCommunicationError(err, "checking available secrets")
		}

		clientSecretRef, ok := secret.FindSecretId(availableSecrets, secret.ClientSecret)
		if !ok {
			return wrapCommunicationError(fmt.Errorf("client secret ref not found in available secrets from secret-manager"), "searching for client secret ref")
		}
		newId, err = secret.GetSecretManager().Rotate(ctx, clientSecretRef)
		if err != nil {
			return wrapCommunicationError(err, "rotate team secret")
		}
		teamObj.Spec.Secret = newId
	}
	return nil
}
