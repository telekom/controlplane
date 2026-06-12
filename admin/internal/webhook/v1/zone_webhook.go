// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/pkg/errors"
	admissionv1 "k8s.io/api/admission/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	"github.com/telekom/controlplane/common/pkg/config"
	"github.com/telekom/controlplane/common/pkg/controller"
	secretsapi "github.com/telekom/controlplane/secret-manager/api"
)

var zonelog = logf.Log.WithName("zone-resource")

// SetupZoneWebhookWithManager registers the webhook for Zone in the manager.
func SetupZoneWebhookWithManager(mgr ctrl.Manager, secretManager secretsapi.SecretManager) error {
	return ctrl.NewWebhookManagedBy(mgr, &adminv1.Zone{}).
		WithDefaulter(&ZoneCustomDefaulter{
			secretManager: secretManager,
		}).
		Complete()
}

// +kubebuilder:webhook:path=/mutate-admin-cp-ei-telekom-de-v1-zone,mutating=true,failurePolicy=fail,sideEffects=None,groups=admin.cp.ei.telekom.de,resources=zones,verbs=create;update,versions=v1,name=mzone-v1.kb.io,admissionReviewVersions=v1

// ZoneCustomDefaulter struct is responsible for setting default values on the custom resource of the
// Kind Zone when those are created or updated.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as it is used only for temporary operations and does not need to be deeply copied.
type ZoneCustomDefaulter struct {
	secretManager secretsapi.SecretManager
}

var _ admission.Defaulter[*adminv1.Zone] = &ZoneCustomDefaulter{}

// getOldZone extracts the old Zone from the admission request context.
// Returns the old object and true if this is an UPDATE operation with a valid old object.
// Returns nil and false for CREATE operations or if the context does not contain an admission request.
func getOldZone(ctx context.Context) (*adminv1.Zone, bool) {
	req, err := admission.RequestFromContext(ctx)
	if err != nil || req.Operation != admissionv1.Update {
		return nil, false
	}
	oldObj := &adminv1.Zone{}
	if err := json.Unmarshal(req.OldObject.Raw, oldObj); err != nil {
		zonelog.Error(err, "failed to unmarshal old Zone from admission request")
		return nil, false
	}
	return oldObj, true
}

// resolveSecretForUpdate returns the old secret value if the new value is empty and
// this is an update with an existing secret. Otherwise returns the new value unchanged.
func resolveSecretForUpdate(newSecret, oldSecret string) string {
	if newSecret == "" && oldSecret != "" {
		return oldSecret
	}
	return newSecret
}

// resolveOptionalSecretForUpdate handles *string pointer secrets.
// Preserves the old value when the new pointer is nil or points to an empty string.
func resolveOptionalSecretForUpdate(newSecret, oldSecret *string) *string {
	if newSecret == nil && oldSecret != nil {
		return oldSecret
	}
	if newSecret != nil && *newSecret == "" && oldSecret != nil {
		return oldSecret
	}
	return newSecret
}

// secretValueOrGenerate returns a generated secret if the value is empty or the rotate keyword.
// Otherwise it returns the user-provided value as-is (for upload to secret manager).
func secretValueOrGenerate(value string) (string, error) {
	if value == "" || value == secretsapi.KeywordRotate {
		return secretsapi.GenerateSecret()
	}
	return value, nil
}

// OnboardSecrets uploads zone secrets to the secret-manager and replaces clear-text values with refs.
func (d *ZoneCustomDefaulter) OnboardSecrets(ctx context.Context, zone *adminv1.Zone) error {
	envName, ok := controller.GetEnvironment(zone)
	if !ok {
		return fmt.Errorf("environment label is required")
	}

	zoneName := zone.Name

	idpPasswordPath := fmt.Sprintf("zones/%s/admin/identityProvider/password", zoneName)
	redisPasswordPath := fmt.Sprintf("zones/%s/admin/redis/password", zoneName)
	gatewaySecretPath := fmt.Sprintf("zones/%s/admin/gateway/clientSecret", zoneName)

	options := []secretsapi.OnboardingOption{
		secretsapi.WithMergeStrategy(),
	}

	// IDP admin password
	needsIdpPassword := !secretsapi.IsRef(zone.Spec.IdentityProvider.Admin.Password)
	if needsIdpPassword {
		secretValue, err := secretValueOrGenerate(zone.Spec.IdentityProvider.Admin.Password)
		if err != nil {
			return errors.Wrap(err, "failed to determine IDP admin password value")
		}
		options = append(options, secretsapi.WithSecretValue(idpPasswordPath, secretValue))
	}

	// Redis password
	needsRedisPassword := !secretsapi.IsRef(zone.Spec.Redis.Password)
	if needsRedisPassword {
		secretValue, err := secretValueOrGenerate(zone.Spec.Redis.Password)
		if err != nil {
			return errors.Wrap(err, "failed to determine Redis password value")
		}
		options = append(options, secretsapi.WithSecretValue(redisPasswordPath, secretValue))
	}

	// Gateway client secret
	gatewayAdminClientSecret := zone.Spec.Gateway.Admin.ClientSecret
	needsGatewaySecret := gatewayAdminClientSecret == nil || !secretsapi.IsRef(*gatewayAdminClientSecret)
	if needsGatewaySecret {
		if gatewayAdminClientSecret == nil {
			// Initialize pointer to avoid nil dereference in secretValueOrGenerate
			gatewayAdminClientSecret = new(string)
		}
		secretValue, err := secretValueOrGenerate(*gatewayAdminClientSecret)
		if err != nil {
			return errors.Wrap(err, "failed to determine gateway client secret value")
		}
		options = append(options, secretsapi.WithSecretValue(gatewaySecretPath, secretValue))
	}

	// Nothing to onboard (only merge-strategy option present)
	if len(options) <= 1 {
		return nil
	}

	availableSecrets, err := d.secretManager.UpsertEnvironment(ctx, envName, options...)
	if err != nil {
		return errors.Wrap(err, "failed to onboard secrets for Zone")
	}
	zonelog.Info("Successfully onboarded secrets for Zone", "environment", envName, "secrets", len(availableSecrets))

	if needsIdpPassword {
		ref, found := secretsapi.FindSecretId(availableSecrets, idpPasswordPath)
		if !found {
			return fmt.Errorf("IDP admin password reference not found in onboarding response")
		}
		zone.Spec.IdentityProvider.Admin.Password = ref
		zonelog.Info("Onboarded IDP admin password for Zone", "secretId", idpPasswordPath)
	}

	if needsRedisPassword {
		ref, found := secretsapi.FindSecretId(availableSecrets, redisPasswordPath)
		if !found {
			return fmt.Errorf("Redis password reference not found in onboarding response")
		}
		zone.Spec.Redis.Password = ref
		zonelog.Info("Onboarded Redis password for Zone", "secretId", redisPasswordPath)
	}

	if needsGatewaySecret {
		ref, found := secretsapi.FindSecretId(availableSecrets, gatewaySecretPath)
		if !found {
			return fmt.Errorf("gateway client secret reference not found in onboarding response")
		}
		zone.Spec.Gateway.Admin.ClientSecret = &ref
		zonelog.Info("Onboarded gateway client secret for Zone", "secretId", gatewaySecretPath)
	}

	return nil
}

// Default implements webhook.CustomDefaulter so a webhook will be registered for the Kind Zone.
func (d *ZoneCustomDefaulter) Default(ctx context.Context, zone *adminv1.Zone) error {
	if controller.IsBeingDeleted(zone) {
		return nil
	}

	zonelog.Info("Defaulting for Zone", "name", zone.GetName())

	// On UPDATE: preserve existing secrets when the new value is empty.
	// This prevents accidental secret regeneration when users omit the field.
	if oldZone, isUpdate := getOldZone(ctx); isUpdate {
		zone.Spec.IdentityProvider.Admin.Password = resolveSecretForUpdate(
			zone.Spec.IdentityProvider.Admin.Password,
			oldZone.Spec.IdentityProvider.Admin.Password,
		)
		zone.Spec.Redis.Password = resolveSecretForUpdate(
			zone.Spec.Redis.Password,
			oldZone.Spec.Redis.Password,
		)
		zone.Spec.Gateway.Admin.ClientSecret = resolveOptionalSecretForUpdate(
			zone.Spec.Gateway.Admin.ClientSecret,
			oldZone.Spec.Gateway.Admin.ClientSecret,
		)
	}

	if config.FeatureSecretManager.IsEnabled() {
		zonelog.Info("Secret-Manager is enabled, onboarding secrets for Zone")

		if d.secretManager == nil {
			return errors.New("Secret-Manager is not configured for Zone webhook")
		}

		if onboardErr := d.OnboardSecrets(ctx, zone); onboardErr != nil {
			return errors.Wrap(onboardErr, "failed to onboard secrets")
		}
		return nil
	}

	zonelog.Info("Secret-Manager is disabled, generating secrets inline for Zone")

	// Generate IDP admin password if empty or rotate
	if zone.Spec.IdentityProvider.Admin.Password == "" ||
		zone.Spec.IdentityProvider.Admin.Password == secretsapi.KeywordRotate {
		secret, err := secretsapi.GenerateSecret()
		if err != nil {
			return errors.Wrap(err, "failed to generate IDP admin password")
		}
		zone.Spec.IdentityProvider.Admin.Password = secret
	}

	// Generate Redis password if empty or rotate
	if zone.Spec.Redis.Password == "" || zone.Spec.Redis.Password == secretsapi.KeywordRotate {
		secret, err := secretsapi.GenerateSecret()
		if err != nil {
			return errors.Wrap(err, "failed to generate Redis password")
		}
		zone.Spec.Redis.Password = secret
	}

	// Generate gateway client secret only when non-nil and empty/rotate
	if zone.Spec.Gateway.Admin.ClientSecret != nil {
		cs := *zone.Spec.Gateway.Admin.ClientSecret
		if cs == "" || cs == secretsapi.KeywordRotate {
			secret, err := secretsapi.GenerateSecret()
			if err != nil {
				return errors.Wrap(err, "failed to generate gateway client secret")
			}
			zone.Spec.Gateway.Admin.ClientSecret = &secret
		}
	}

	return nil
}
