// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package mutator

import (
	"context"
	"fmt"
	"strings"

	"github.com/go-logr/logr"
	applicationv1 "github.com/telekom/controlplane/application/api/v1"
	"github.com/telekom/controlplane/application/internal/secret"
	secretsapi "github.com/telekom/controlplane/secret-manager/api"
	"k8s.io/apimachinery/pkg/api/errors"
)

func wrapCommunicationError(err error, purposeOfCommunication string) error {
	return errors.NewInternalError(fmt.Errorf("failure during communication with secret-manager when doing '%s': '%w'", purposeOfCommunication, err))
}

// MutateSecret intercepts Application secret values and replaces them with
// secret-manager references. If the secret is already a reference, this is a no-op.
// If the secret is "rotate" or empty, a new secret is generated.
func MutateSecret(ctx context.Context, env string, app *applicationv1.Application) error {
	log := logr.FromContextOrDiscard(ctx)

	if secretsapi.IsRef(app.Spec.Secret) {
		log.V(1).Info("spec.secret is already a reference, nothing to do")
		return nil
	}

	var clientSecret string
	if strings.EqualFold(app.Spec.Secret, secret.KeywordRotate) || app.Spec.Secret == "" {
		generatedSecret, err := secretsapi.GenerateSecret()
		if err != nil {
			return errors.NewInternalError(fmt.Errorf("failed to generate secret: %w", err))
		}
		clientSecret = generatedSecret
	} else {
		clientSecret = app.Spec.Secret
	}

	availableSecrets, err := secret.GetSecretManager().UpsertApplication(ctx, env, app.Spec.Team, app.GetName(),
		secret.WithSecretValue(secret.ClientSecret, clientSecret))
	if err != nil {
		return wrapCommunicationError(err, "upsert application")
	}

	log.V(1).Info("upserted application secrets in secret-manager", "availableSecrets", availableSecrets)

	clientSecretRef, ok := secret.FindSecretId(availableSecrets, secret.ClientSecret)
	if !ok {
		return wrapCommunicationError(fmt.Errorf("client secret ref not found in available secrets from secret-manager"), "searching for client secret ref")
	}

	app.Spec.Secret = clientSecretRef
	return nil
}
