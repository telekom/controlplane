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
		WithValidator(&ZoneCustomValidator{}).
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

	redisPasswordPath := fmt.Sprintf("zones/%s/admin/redis/password", zoneName)

	options := []secretsapi.OnboardingOption{
		secretsapi.WithMergeStrategy(),
	}

	idpPasswordPaths := make(map[int]string)
	for i := range zone.Spec.IdentityProviders {
		identityProvider := &zone.Spec.IdentityProviders[i]
		if secretsapi.IsRef(identityProvider.Admin.Password) {
			continue
		}
		secretValue, err := secretValueOrGenerate(identityProvider.Admin.Password)
		if err != nil {
			return errors.Wrap(err, "failed to determine IDP admin password value")
		}
		idpPasswordPath := fmt.Sprintf("zones/%s/admin/identityProviders/%s/password", zoneName, identityProvider.Name)
		idpPasswordPaths[i] = idpPasswordPath
		options = append(options, secretsapi.WithSecretValue(idpPasswordPath, secretValue))
	}

	// Redis password
	needsRedisPassword := zone.Spec.Redis != nil && !secretsapi.IsRef(zone.Spec.Redis.Password)
	if needsRedisPassword {
		secretValue, err := secretValueOrGenerate(zone.Spec.Redis.Password)
		if err != nil {
			return errors.Wrap(err, "failed to determine Redis password value")
		}
		options = append(options, secretsapi.WithSecretValue(redisPasswordPath, secretValue))
	}

	gatewaySecretPaths := make(map[int]string)
	for i := range zone.Spec.Gateways {
		gateway := &zone.Spec.Gateways[i]
		gatewayAdminClientSecret := gateway.Admin.ClientSecret
		if gatewayAdminClientSecret != nil && secretsapi.IsRef(*gatewayAdminClientSecret) {
			continue
		}
		if gatewayAdminClientSecret == nil {
			gatewayAdminClientSecret = new(string)
		}
		secretValue, err := secretValueOrGenerate(*gatewayAdminClientSecret)
		if err != nil {
			return errors.Wrap(err, "failed to determine gateway client secret value")
		}
		gatewaySecretPath := fmt.Sprintf("zones/%s/admin/gateways/%s/clientSecret", zoneName, gateway.Name)
		gatewaySecretPaths[i] = gatewaySecretPath
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

	for i, idpPasswordPath := range idpPasswordPaths {
		ref, found := secretsapi.FindSecretId(availableSecrets, idpPasswordPath)
		if !found {
			return fmt.Errorf("IDP admin password reference not found in onboarding response")
		}
		zone.Spec.IdentityProviders[i].Admin.Password = ref
		zonelog.Info("Onboarded IDP admin password for Zone", "secretId", idpPasswordPath)
	}

	if needsRedisPassword {
		ref, found := secretsapi.FindSecretId(availableSecrets, redisPasswordPath)
		if !found {
			return fmt.Errorf("redis password reference not found in onboarding response")
		}
		zone.Spec.Redis.Password = ref
		zonelog.Info("Onboarded Redis password for Zone", "secretId", redisPasswordPath)
	}

	for i, gatewaySecretPath := range gatewaySecretPaths {
		ref, found := secretsapi.FindSecretId(availableSecrets, gatewaySecretPath)
		if !found {
			return fmt.Errorf("gateway client secret reference not found in onboarding response")
		}
		zone.Spec.Gateways[i].Admin.ClientSecret = &ref
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
		oldIdentityProviders := make(map[string]adminv1.IdentityProviderConfig, len(oldZone.Spec.IdentityProviders))
		for _, identityProvider := range oldZone.Spec.IdentityProviders {
			oldIdentityProviders[identityProvider.Name] = identityProvider
		}
		for i := range zone.Spec.IdentityProviders {
			oldIdentityProvider, found := oldIdentityProviders[zone.Spec.IdentityProviders[i].Name]
			if found {
				zone.Spec.IdentityProviders[i].Admin.Password = resolveSecretForUpdate(
					zone.Spec.IdentityProviders[i].Admin.Password,
					oldIdentityProvider.Admin.Password,
				)
			}
		}
		if zone.Spec.Redis != nil && oldZone.Spec.Redis != nil {
			zone.Spec.Redis.Password = resolveSecretForUpdate(zone.Spec.Redis.Password, oldZone.Spec.Redis.Password)
		}
		oldGateways := make(map[string]adminv1.GatewayConfig, len(oldZone.Spec.Gateways))
		for _, gateway := range oldZone.Spec.Gateways {
			oldGateways[gateway.Name] = gateway
		}
		for i := range zone.Spec.Gateways {
			oldGateway, found := oldGateways[zone.Spec.Gateways[i].Name]
			if found {
				zone.Spec.Gateways[i].Admin.ClientSecret = resolveOptionalSecretForUpdate(
					zone.Spec.Gateways[i].Admin.ClientSecret,
					oldGateway.Admin.ClientSecret,
				)
			}
		}
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

	for i := range zone.Spec.IdentityProviders {
		password := zone.Spec.IdentityProviders[i].Admin.Password
		if password == "" || password == secretsapi.KeywordRotate {
			secret, err := secretsapi.GenerateSecret()
			if err != nil {
				return errors.Wrap(err, "failed to generate IDP admin password")
			}
			zone.Spec.IdentityProviders[i].Admin.Password = secret
		}
	}

	// Generate Redis password if empty or rotate
	if zone.Spec.Redis != nil && (zone.Spec.Redis.Password == "" || zone.Spec.Redis.Password == secretsapi.KeywordRotate) {
		secret, err := secretsapi.GenerateSecret()
		if err != nil {
			return errors.Wrap(err, "failed to generate Redis password")
		}
		zone.Spec.Redis.Password = secret
	}

	for i := range zone.Spec.Gateways {
		clientSecret := zone.Spec.Gateways[i].Admin.ClientSecret
		if clientSecret == nil {
			continue
		}
		cs := *clientSecret
		if cs == "" || cs == secretsapi.KeywordRotate {
			secret, err := secretsapi.GenerateSecret()
			if err != nil {
				return errors.Wrap(err, "failed to generate gateway client secret")
			}
			zone.Spec.Gateways[i].Admin.ClientSecret = &secret
		}
	}

	return nil
}
