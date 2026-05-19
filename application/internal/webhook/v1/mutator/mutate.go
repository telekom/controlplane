// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package mutator

import (
	"context"
	"fmt"
	"strings"

	"github.com/go-logr/logr"
	admissionv1 "k8s.io/api/admission/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	applicationv1 "github.com/telekom/controlplane/application/api/v1"
	"github.com/telekom/controlplane/application/internal/secret"
	"github.com/telekom/controlplane/common/pkg/util/contextutil"
	secretsapi "github.com/telekom/controlplane/secret-manager/api"
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

// resolveRotationOpts checks whether the rotation should be graceful (zone has SecretRotation feature enabled)
// and, if so, retrieves the current secret value to store as the rotated secret.
// Returns the additional onboarding options and whether the rotation is graceful.
func resolveRotationOpts(ctx context.Context, app *applicationv1.Application, reader client.Reader) ([]secretsapi.OnboardingOption, bool, error) {
	log := logr.FromContextOrDiscard(ctx)
	eventRecorder := contextutil.EventRecorderFromContextOrDiscard(ctx)

	zone := &adminv1.Zone{}
	if err := reader.Get(ctx, app.Spec.Zone.K8s(), zone); err != nil {
		return nil, false, errors.NewInternalError(fmt.Errorf("failed to fetch zone %s to check secret rotation feature: %w", app.Spec.Zone.Name, err))
	}

	isGraceful := zone.IsFeatureEnabled(adminv1.FeatureSecretRotation)
	if !isGraceful {
		log.Info("zone does not have SecretRotation feature enabled, performing non-graceful rotation (no grace period)",
			"zone", app.Spec.Zone.Name)
		eventRecorder.Eventf(app, nil, "Normal", "NonGracefulRotation", "StartNonGracefulRotation", "zone %q does not have SecretRotation feature enabled, no grace period", app.Spec.Zone.Name)
		return nil, false, nil
	}

	eventRecorder.Eventf(app, nil, "Normal", "GracefulRotation", "StartGracefulRotation", "zone %q has SecretRotation feature enabled, with grace period", app.Spec.Zone.Name)

	if app.Status.ClientSecret == "" {
		return nil, true, nil
	}

	currentSecretValue, err := secret.GetSecretManager().Get(ctx, app.Status.ClientSecret)
	if err != nil {
		return nil, true, wrapCommunicationError(err, "retrieving current client secret for rotation")
	}

	return []secretsapi.OnboardingOption{
		secret.WithSecretValue(secret.RotatedClientSecret, currentSecretValue),
	}, true, nil
}

// MutateSecret intercepts Application secret values and replaces them with
// secret-manager references. If the secret is already a reference, this is a no-op.
// If the secret is "rotate" or empty, a new secret is generated.
// On rotation, the current secret is preserved as rotatedClientSecret in the secret-manager
// and a new clientSecret is generated.
func MutateSecret(ctx context.Context, env string, app *applicationv1.Application, reader client.Reader) error {
	log := logr.FromContextOrDiscard(ctx)
	eventRecorder := contextutil.EventRecorderFromContextOrDiscard(ctx)

	req, err := admission.RequestFromContext(ctx)
	if err != nil {
		return errors.NewInternalError(fmt.Errorf("failed to get admission request from context: %w", err))
	}

	if secretsapi.IsRef(app.Spec.Secret) {
		log.V(1).Info("spec.secret is already a reference, nothing to do")
		eventRecorder.Eventf(app, nil, "Normal", "SecretMutationSkipped", "SkipMutation", "spec.secret is already a reference, skipping mutation")
		return nil
	}

	// Determine if this is a rotation request. This is called from an admission webhook, so the
	// admission request operation is available: a rotate keyword on anything other than Create is
	// treated as a rotation request, while Create is the initial secret creation.
	isRotation := strings.EqualFold(app.Spec.Secret, secret.KeywordRotate) && req.Operation != admissionv1.Create

	// Guard: deny rotation if one is already in progress
	if isRotation && isRotationInProgress(app) {
		eventRecorder.Eventf(app, nil, "Warning", "SecretRotationBlocked", "BlockRotation", "a secret rotation is already in progress, blocking new rotation request")

		return errors.NewForbidden(
			schema.GroupResource{Group: "application.cp.ei.telekom.de", Resource: "applications"},
			app.GetName(),
			fmt.Errorf("a secret rotation is already in progress, please wait for it to complete"),
		)
	}

	eventRecorder.Eventf(app, nil, "Normal", "SecretMutationStarted", "StartMutation", "starting mutation of application secret")

	// Resolve rotation options (graceful rotation check + current secret retrieval)
	var rotationOpts []secretsapi.OnboardingOption
	var isGracefulRotation bool
	if isRotation {
		var rotErr error
		rotationOpts, isGracefulRotation, rotErr = resolveRotationOpts(ctx, app, reader)
		if rotErr != nil {
			return rotErr
		}
	}

	var clientSecret string
	// On initial creation, or when the secret value is explicitly set to "rotate", generate a new secret value.
	if strings.EqualFold(app.Spec.Secret, secret.KeywordRotate) || app.Spec.Secret == "" {
		clientSecret, err = secretsapi.GenerateSecret()
		if err != nil {
			return errors.NewInternalError(fmt.Errorf("failed to generate secret: %w", err))
		}
		eventRecorder.Eventf(app, nil, "Normal", "SecretGenerated", "GenerateSecret", "generated new client secret for application")
	} else {
		clientSecret = app.Spec.Secret
	}

	// Build the list of secrets to upsert
	upsertOpts := make([]secretsapi.OnboardingOption, 0, 1+len(rotationOpts))
	upsertOpts = append(upsertOpts, secret.WithSecretValue(secret.ClientSecret, clientSecret))
	upsertOpts = append(upsertOpts, rotationOpts...)

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
	} else if isRotation {
		log.Info("non-graceful secret rotation - old secret will be immediately replaced",
			"application", app.GetName(),
			"zone", app.Spec.Zone.Name,
		)
	}

	return nil
}
