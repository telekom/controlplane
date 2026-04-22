// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package mutator

import (
	"context"
	"fmt"
	"strings"

	"github.com/go-logr/logr"
	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	applicationv1 "github.com/telekom/controlplane/application/api/v1"
	"github.com/telekom/controlplane/application/internal/secret"
	secretsapi "github.com/telekom/controlplane/secret-manager/api"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func wrapCommunicationError(err error, purposeOfCommunication string) error {
	return errors.NewInternalError(fmt.Errorf("failure during communication with secret-manager when doing '%s': '%w'", purposeOfCommunication, err))
}

// isRotationInProgress checks whether a secret rotation is currently in progress
// by inspecting the SecretRotation condition on the Application status.
// The condition is set to InProgress by the handler when it starts processing a rotation.
func isRotationInProgress(app *applicationv1.Application) bool {
	cond := meta.FindStatusCondition(app.Status.Conditions, secret.SecretRotationConditionType)
	return cond != nil && cond.Reason == secret.SecretRotationReasonInProgress
}

// MutateSecret intercepts Application secret values and replaces them with
// secret-manager references. If the secret is already a reference, this is a no-op.
// If the secret is "rotate" or empty, a new secret is generated.
// On rotation, the current secret is preserved as rotatedClientSecret in the secret-manager
// and a new clientSecret is generated.
func MutateSecret(ctx context.Context, env string, app *applicationv1.Application, reader client.Reader) error {
	log := logr.FromContextOrDiscard(ctx)

	if secretsapi.IsRef(app.Spec.Secret) {
		log.V(1).Info("spec.secret is already a reference, nothing to do")
		return nil
	}

	isRotation := strings.EqualFold(app.Spec.Secret, secret.KeywordRotate)

	// Guard: deny rotation if one is already in progress
	if isRotation && isRotationInProgress(app) {
		return errors.NewForbidden(
			schema.GroupResource{Group: "application.cp.ei.telekom.de", Resource: "applications"},
			app.GetName(),
			fmt.Errorf("a secret rotation is already in progress, please wait for it to complete"),
		)
	}

	// Determine if the rotation should be graceful (zone has SecretRotation feature enabled)
	isGracefulRotation := false
	if isRotation {
		zone := &adminv1.Zone{}
		if err := reader.Get(ctx, app.Spec.Zone.K8s(), zone); err != nil {
			return errors.NewInternalError(fmt.Errorf("failed to fetch zone %s to check secret rotation feature: %w", app.Spec.Zone.Name, err))
		}
		isGracefulRotation = zone.IsFeatureEnabled(adminv1.FeatureSecretRotation)
		if !isGracefulRotation {
			log.Info("zone does not have SecretRotation feature enabled, performing non-graceful rotation (no grace period)",
				"zone", app.Spec.Zone.Name)
		}
	}

	var clientSecret string
	if isRotation || app.Spec.Secret == "" {
		generatedSecret, err := secretsapi.GenerateSecret()
		if err != nil {
			return errors.NewInternalError(fmt.Errorf("failed to generate secret: %w", err))
		}
		clientSecret = generatedSecret
	} else {
		clientSecret = app.Spec.Secret
	}

	// Build the list of secrets to upsert
	upsertOpts := []secretsapi.OnboardingOption{
		secret.WithSecretValue(secret.ClientSecret, clientSecret),
	}

	// On graceful rotation, retrieve the current secret value and store it as rotatedClientSecret
	if isGracefulRotation && app.Status.ClientSecret != "" {
		currentSecretValue, err := secret.GetSecretManager().Get(ctx, app.Status.ClientSecret)
		if err != nil {
			return wrapCommunicationError(err, "retrieving current client secret for rotation")
		}
		upsertOpts = append(upsertOpts,
			secret.WithSecretValue(secret.RotatedClientSecret, currentSecretValue),
		)
	}

	availableSecrets, err := secret.GetSecretManager().UpsertApplication(ctx, env, app.Spec.Team, app.GetName(),
		upsertOpts...)
	if err != nil {
		return wrapCommunicationError(err, "upsert application")
	}

	log.V(1).Info("upserted application secrets in secret-manager", "availableSecrets", availableSecrets)

	clientSecretRef, ok := secret.FindSecretId(availableSecrets, secret.ClientSecret)
	if !ok {
		return wrapCommunicationError(fmt.Errorf("client secret ref not found in available secrets from secret-manager"), "searching for client secret ref")
	}

	app.Spec.Secret = clientSecretRef

	// On graceful rotation, also set the rotated secret ref.
	// The handler will detect spec.rotatedSecret being set and manage the SecretRotation condition lifecycle.
	if isGracefulRotation {
		rotatedSecretRef, ok := secret.FindSecretId(availableSecrets, secret.RotatedClientSecret)
		if !ok {
			return wrapCommunicationError(fmt.Errorf("rotated client secret ref not found in available secrets from secret-manager"), "searching for rotated client secret ref")
		}
		app.Spec.RotatedSecret = rotatedSecretRef

		annotations := app.GetAnnotations()
		if annotations == nil {
			annotations = make(map[string]string)
		}
		annotations[secret.AnnotationGracefulRotation] = "true"
		app.SetAnnotations(annotations)

		log.Info("graceful secret rotation initiated",
			"application", app.GetName(),
			"zone", app.Spec.Zone.Name,
		)
	} else {
		log.Info("non-graceful secret rotation - old secret will be immediately replaced",
			"application", app.GetName(),
			"zone", app.Spec.Zone.Name,
		)
	}

	return nil
}
