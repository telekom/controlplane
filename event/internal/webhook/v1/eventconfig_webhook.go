// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/pkg/errors"
	admissionv1 "k8s.io/api/admission/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	"github.com/telekom/controlplane/common/pkg/config"
	"github.com/telekom/controlplane/common/pkg/controller"
	cerrors "github.com/telekom/controlplane/common/pkg/errors"
	eventv1 "github.com/telekom/controlplane/event/api/v1"
	"github.com/telekom/controlplane/event/internal/handler/util"
	secretsapi "github.com/telekom/controlplane/secret-manager/api"
)

var log = logf.Log.WithName("eventconfig-resource")

// SetupEventConfigWebhookWithManager registers the webhook for EventConfig in the manager.
func SetupEventConfigWebhookWithManager(mgr ctrl.Manager, secretManager secretsapi.SecretManager) error {
	return ctrl.NewWebhookManagedBy(mgr, &eventv1.EventConfig{}).
		WithValidator(&EventConfigCustomValidator{}).
		WithDefaulter(&EventConfigCustomDefaulter{
			reader:        mgr.GetClient(),
			secretManager: secretManager,
		}).
		Complete()
}

// +kubebuilder:rbac:groups=admin.cp.ei.telekom.de,resources=zones,verbs=get
// +kubebuilder:rbac:groups=admin.cp.ei.telekom.de,resources=zones/status,verbs=get
// +kubebuilder:webhook:path=/mutate-event-cp-ei-telekom-de-v1-eventconfig,mutating=true,failurePolicy=fail,sideEffects=None,groups=event.cp.ei.telekom.de,resources=eventconfigs,verbs=create;update,versions=v1,name=meventconfig-v1.kb.io,admissionReviewVersions=v1

// EventConfigCustomDefaulter struct is responsible for setting default values on the custom resource of the
// Kind EventConfig when those are created or updated.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as it is used only for temporary operations and does not need to be deeply copied.
type EventConfigCustomDefaulter struct {
	reader        client.Reader
	secretManager secretsapi.SecretManager
}

var _ admission.Defaulter[*eventv1.EventConfig] = &EventConfigCustomDefaulter{}

// getOldEventConfig extracts the old EventConfig from the admission request context.
// Returns the old object and true if this is an UPDATE operation with a valid old object.
// Returns nil and false for CREATE operations or if the context does not contain an admission request
// (e.g. in unit tests without injected context).
func getOldEventConfig(ctx context.Context) (*eventv1.EventConfig, bool) {
	req, err := admission.RequestFromContext(ctx)
	if err != nil || req.Operation != admissionv1.Update {
		return nil, false
	}
	oldObj := &eventv1.EventConfig{}
	if err := json.Unmarshal(req.OldObject.Raw, oldObj); err != nil {
		log.Error(err, "failed to unmarshal old EventConfig from admission request")
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

// secretValueOrGenerate returns a generated secret if the value is empty or the rotate keyword.
// Otherwise it returns the user-provided value as-is (for upload to secret manager).
func secretValueOrGenerate(value string) (string, error) {
	if value == "" || value == secretsapi.KeywordRotate {
		return secretsapi.GenerateSecret()
	}
	return value, nil
}

func (d *EventConfigCustomDefaulter) OnboardSecrets(ctx context.Context, eventCfg *eventv1.EventConfig) (err error) {
	envName, ok := controller.GetEnvironment(eventCfg)
	if !ok {
		return apierrors.NewBadRequest("environment label is required")
	}

	zoneName := eventCfg.Spec.Zone.Name

	adminSecretPath := fmt.Sprintf("zones/%s/event/admin/clientSecret", zoneName)
	meshSecretPath := fmt.Sprintf("zones/%s/event/mesh/clientSecret", zoneName)

	options := []secretsapi.OnboardingOption{
		secretsapi.WithMergeStrategy(), // Preserve existing secrets not in the request
	}

	needsAdminSecret := !secretsapi.IsRef(eventCfg.Spec.Admin.Client.ClientSecret)
	if needsAdminSecret {
		var secretValue string
		secretValue, err = secretValueOrGenerate(eventCfg.Spec.Admin.Client.ClientSecret)
		if err != nil {
			return errors.Wrap(err, "failed to determine admin client secret value")
		}
		options = append(options, secretsapi.WithSecretValue(adminSecretPath, secretValue))
	}

	needsMeshSecret := !secretsapi.IsRef(eventCfg.Spec.Mesh.Client.ClientSecret)
	if needsMeshSecret {
		var secretValue string
		secretValue, err = secretValueOrGenerate(eventCfg.Spec.Mesh.Client.ClientSecret)
		if err != nil {
			return errors.Wrap(err, "failed to determine mesh client secret value")
		}
		options = append(options, secretsapi.WithSecretValue(meshSecretPath, secretValue))
	}

	if len(options) <= 1 {
		return nil
	}

	availableSecrets, err := d.secretManager.UpsertEnvironment(ctx, envName, options...)
	if err != nil {
		return errors.Wrap(err, "failed to onboard secrets for EventConfig")
	}
	log.Info("Successfully onboarded secrets for EventConfig", "environment", envName, "secrets", len(availableSecrets))

	if needsAdminSecret {
		ref, found := secretsapi.FindSecretId(availableSecrets, adminSecretPath)
		if !found {
			return fmt.Errorf("admin client secret reference not found in onboarding response")
		}
		eventCfg.Spec.Admin.Client.ClientSecret = ref
		log.Info("Onboarded admin client secret for EventConfig", "secretId", adminSecretPath)
	}

	if needsMeshSecret {
		ref, found := secretsapi.FindSecretId(availableSecrets, meshSecretPath)
		if !found {
			return fmt.Errorf("mesh client secret reference not found in onboarding response")
		}
		eventCfg.Spec.Mesh.Client.ClientSecret = ref
		log.Info("Onboarded mesh client secret for EventConfig", "secretId", meshSecretPath)
	}

	return nil
}

// Default implements webhook.CustomDefaulter so a webhook will be registered for the Kind EventConfig.
func (d *EventConfigCustomDefaulter) Default(ctx context.Context, eventCfg *eventv1.EventConfig) (err error) {
	if controller.IsBeingDeleted(eventCfg) {
		return nil
	}

	log.Info("Defaulting for EventConfig", "name", eventCfg.GetName())

	// Initialize Mesh with full-mesh defaults if not provided.
	if eventCfg.Spec.Mesh == nil {
		eventCfg.Spec.Mesh = &eventv1.MeshConfig{
			FullMesh: true,
		}
	}

	adminClient := &eventCfg.Spec.Admin.Client
	if adminClient.ClientId == "" {
		adminClient.ClientId = util.AdminClientName
	}

	meshClient := &eventCfg.Spec.Mesh.Client
	if meshClient.ClientId == "" {
		meshClient.ClientId = util.MeshClientName
	}

	// Resolve realm references from the Zone if not explicitly specified.
	if err := d.defaultRealmsFromZone(ctx, eventCfg, adminClient, meshClient); err != nil {
		return err
	}

	// On UPDATE, preserve existing secrets when the new value is empty.
	// This prevents accidental secret regeneration when users omit the field.
	if oldCfg, isUpdate := getOldEventConfig(ctx); isUpdate {
		adminClient.ClientSecret = resolveSecretForUpdate(adminClient.ClientSecret, oldCfg.Spec.Admin.Client.ClientSecret)
		if oldCfg.Spec.Mesh != nil {
			meshClient.ClientSecret = resolveSecretForUpdate(meshClient.ClientSecret, oldCfg.Spec.Mesh.Client.ClientSecret)
		}
	}

	if config.FeatureSecretManager.IsEnabled() {
		log.Info("Secret-Manager is enabled, onboarding secrets for EventConfig")

		if d.secretManager == nil {
			return errors.New("Secret-Manager is not configured for EventConfig webhook")
		}

		if onboardErr := d.OnboardSecrets(ctx, eventCfg); onboardErr != nil {
			return errors.Wrap(onboardErr, "failed to onboard secrets")
		}
		return nil
	}

	log.Info("Secret-Manager is disabled, skipping onboarding of secrets for EventConfig")
	return d.generateLocalSecrets(adminClient, meshClient)
}

// defaultRealmsFromZone resolves realm references from the Zone if not explicitly specified.
func (d *EventConfigCustomDefaulter) defaultRealmsFromZone(ctx context.Context, eventCfg *eventv1.EventConfig, adminClient, meshClient *eventv1.ClientConfig) error {
	if !adminClient.Realm.IsEmpty() && !meshClient.Realm.IsEmpty() {
		return nil
	}

	zone := &adminv1.Zone{}
	if err := d.reader.Get(ctx, eventCfg.Spec.Zone.K8s(), zone); err != nil {
		return errors.Wrapf(err, "failed to get Zone %q for realm defaulting", eventCfg.Spec.Zone.String())
	}
	if adminClient.Realm.IsEmpty() && zone.Status.InternalIdentityRealm != nil {
		adminClient.Realm = *zone.Status.InternalIdentityRealm
		log.Info("Defaulted admin client realm from zone", "realm", adminClient.Realm.String())
	}
	if meshClient.Realm.IsEmpty() && zone.Status.IdentityRealm != nil {
		meshClient.Realm = *zone.Status.IdentityRealm
		log.Info("Defaulted mesh client realm from zone", "realm", meshClient.Realm.String())
	}
	return nil
}

// generateLocalSecrets generates secrets locally when the Secret-Manager is disabled.
func (d *EventConfigCustomDefaulter) generateLocalSecrets(adminClient, meshClient *eventv1.ClientConfig) error {
	if adminClient.ClientSecret == "" || adminClient.ClientSecret == secretsapi.KeywordRotate {
		secret, err := secretsapi.GenerateSecret()
		if err != nil {
			return errors.Wrap(err, "failed to generate admin client secret")
		}
		adminClient.ClientSecret = secret
	}
	if meshClient.ClientSecret == "" || meshClient.ClientSecret == secretsapi.KeywordRotate {
		secret, err := secretsapi.GenerateSecret()
		if err != nil {
			return errors.Wrap(err, "failed to generate mesh client secret")
		}
		meshClient.ClientSecret = secret
	}
	return nil
}

// NOTE: If you want to customise the 'path', use the flags '--defaulting-path' or '--validation-path'.
// +kubebuilder:webhook:path=/validate-event-cp-ei-telekom-de-v1-eventconfig,mutating=false,failurePolicy=fail,sideEffects=None,groups=event.cp.ei.telekom.de,resources=eventconfigs,verbs=create;update,versions=v1,name=veventconfig-v1.kb.io,admissionReviewVersions=v1

// EventConfigCustomValidator struct is responsible for validating the EventConfig resource
// when it is created, updated, or deleted.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as this struct is used only for temporary operations and does not need to be deeply copied.
type EventConfigCustomValidator struct{}

var _ admission.Validator[*eventv1.EventConfig] = &EventConfigCustomValidator{}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type EventConfig.
func (v *EventConfigCustomValidator) ValidateCreate(ctx context.Context, obj *eventv1.EventConfig) (admission.Warnings, error) {
	return v.ValidateCreateOrUpdate(ctx, obj)
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type EventConfig.
func (v *EventConfigCustomValidator) ValidateUpdate(ctx context.Context, oldObj, newObj *eventv1.EventConfig) (admission.Warnings, error) {
	return v.ValidateCreateOrUpdate(ctx, newObj)
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type EventConfig.
func (v *EventConfigCustomValidator) ValidateDelete(ctx context.Context, obj *eventv1.EventConfig) (admission.Warnings, error) {
	// Could check if there are any Events configured to use this EventConfig and prevent deletion if there are any, but for now we allow deletion without checks.
	return nil, nil
}

func (v *EventConfigCustomValidator) ValidateCreateOrUpdate(ctx context.Context, eventCfg *eventv1.EventConfig) (admission.Warnings, error) {
	if controller.IsBeingDeleted(eventCfg) {
		return nil, nil
	}

	valErr := cerrors.NewValidationError(eventv1.GroupVersion.WithKind("EventConfig").GroupKind(), eventCfg)

	// Realm fields are optional: when empty, the handler resolves them
	// from the Zone's identity realms (InternalIdentityRealm for admin, IdentityRealm for mesh).
	// However, if a realm is partially specified (only name or only namespace), that is invalid.
	adminClient := eventCfg.Spec.Admin.Client
	if !adminClient.Realm.IsEmpty() && (adminClient.Realm.Name == "" || adminClient.Realm.Namespace == "") {
		valErr.AddInvalidError(field.NewPath("spec").Child("admin").Child("client").Child("realm"), adminClient.Realm, "realm must have both name and namespace if specified")
	}

	if eventCfg.Spec.Mesh != nil {
		meshClient := eventCfg.Spec.Mesh.Client
		if !meshClient.Realm.IsEmpty() && (meshClient.Realm.Name == "" || meshClient.Realm.Namespace == "") {
			valErr.AddInvalidError(field.NewPath("spec").Child("mesh").Child("client").Child("realm"), meshClient.Realm, "realm must have both name and namespace if specified")
		}
	}

	return valErr.BuildWarnings(), valErr.BuildError()
}
