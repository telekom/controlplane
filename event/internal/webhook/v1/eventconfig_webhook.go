// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"context"
	"encoding/json"
	"fmt"

	admissionv1 "k8s.io/api/admission/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/pkg/errors"
	"github.com/telekom/controlplane/common/pkg/config"
	"github.com/telekom/controlplane/common/pkg/controller"
	cerrors "github.com/telekom/controlplane/common/pkg/errors"
	eventv1 "github.com/telekom/controlplane/event/api/v1"
	"github.com/telekom/controlplane/event/internal/handler/util"
	secretsapi "github.com/telekom/controlplane/secret-manager/api"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

// nolint:unused
// log is for logging in this package.
var log = logf.Log.WithName("eventconfig-resource")

// SetupEventConfigWebhookWithManager registers the webhook for EventConfig in the manager.
func SetupEventConfigWebhookWithManager(mgr ctrl.Manager, secretManager secretsapi.SecretManager) error {
	return ctrl.NewWebhookManagedBy(mgr).For(&eventv1.EventConfig{}).
		WithValidator(&EventConfigCustomValidator{}).
		WithDefaulter(&EventConfigCustomDefaulter{secretManager}).
		Complete()
}

// +kubebuilder:webhook:path=/mutate-event-cp-ei-telekom-de-v1-eventconfig,mutating=true,failurePolicy=fail,sideEffects=None,groups=event.cp.ei.telekom.de,resources=eventconfigs,verbs=create;update,versions=v1,name=meventconfig-v1.kb.io,admissionReviewVersions=v1

// EventConfigCustomDefaulter struct is responsible for setting default values on the custom resource of the
// Kind EventConfig when those are created or updated.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as it is used only for temporary operations and does not need to be deeply copied.
type EventConfigCustomDefaulter struct {
	secretManager secretsapi.SecretManager
}

var _ webhook.CustomDefaulter = &EventConfigCustomDefaulter{}

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
func secretValueOrGenerate(value string) string {
	if value == "" || value == secretsapi.KeywordRotate {
		return secretsapi.GenerateSecret()
	}
	return value
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
		options = append(options, secretsapi.WithSecretValue(adminSecretPath, secretValueOrGenerate(eventCfg.Spec.Admin.Client.ClientSecret)))
	}

	needsMeshSecret := !secretsapi.IsRef(eventCfg.Spec.Mesh.Client.ClientSecret)
	if needsMeshSecret {
		options = append(options, secretsapi.WithSecretValue(meshSecretPath, secretValueOrGenerate(eventCfg.Spec.Mesh.Client.ClientSecret)))
	}

	if len(options) > 1 {
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
	}

	return nil
}

// Default implements webhook.CustomDefaulter so a webhook will be registered for the Kind EventConfig.
func (d *EventConfigCustomDefaulter) Default(ctx context.Context, obj runtime.Object) (err error) {
	eventCfg, ok := obj.(*eventv1.EventConfig)
	if !ok {
		return fmt.Errorf("expected an EventConfig object but got %T", obj)
	}

	if controller.IsBeingDeleted(eventCfg) {
		return nil
	}

	log.Info("Defaulting for EventConfig", "name", eventCfg.GetName())

	adminClient := &eventCfg.Spec.Admin.Client
	if adminClient.ClientId == "" {
		adminClient.ClientId = util.AdminClientName
	}

	meshClient := &eventCfg.Spec.Mesh.Client
	if meshClient.ClientId == "" {
		meshClient.ClientId = util.MeshClientName
	}

	// On UPDATE, preserve existing secrets when the new value is empty.
	// This prevents accidental secret regeneration when users omit the field.
	if oldCfg, isUpdate := getOldEventConfig(ctx); isUpdate {
		adminClient.ClientSecret = resolveSecretForUpdate(adminClient.ClientSecret, oldCfg.Spec.Admin.Client.ClientSecret)
		meshClient.ClientSecret = resolveSecretForUpdate(meshClient.ClientSecret, oldCfg.Spec.Mesh.Client.ClientSecret)
	}

	if config.FeatureSecretManager.IsEnabled() {
		log.Info("Secret-Manager is enabled, onboarding secrets for EventConfig")

		if d.secretManager == nil {
			return errors.New("Secret-Manager is not configured for EventConfig webhook")
		}

		if err := d.OnboardSecrets(ctx, eventCfg); err != nil {
			return errors.Wrap(err, "failed to onboard secrets")
		}

	} else {
		log.Info("Secret-Manager is disabled, skipping onboarding of secrets for EventConfig")

		if adminClient.ClientSecret == "" || adminClient.ClientSecret == secretsapi.KeywordRotate {
			adminClient.ClientSecret = secretsapi.GenerateSecret()
		}
		if meshClient.ClientSecret == "" || meshClient.ClientSecret == secretsapi.KeywordRotate {
			meshClient.ClientSecret = secretsapi.GenerateSecret()
		}
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
type EventConfigCustomValidator struct {
}

var _ webhook.CustomValidator = &EventConfigCustomValidator{}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type EventConfig.
func (v *EventConfigCustomValidator) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	return v.ValidateCreateOrUpdate(ctx, obj)
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type EventConfig.
func (v *EventConfigCustomValidator) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	return v.ValidateCreateOrUpdate(ctx, newObj)
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type EventConfig.
func (v *EventConfigCustomValidator) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {

	// Could check if there are any Events configured to use this EventConfig and prevent deletion if there are any, but for now we allow deletion without checks.
	return nil, nil
}

func (v *EventConfigCustomValidator) ValidateCreateOrUpdate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	eventCfg, ok := obj.(*eventv1.EventConfig)
	if !ok {
		return nil, fmt.Errorf("expected a EventConfig object for the newObj but got %T", obj)
	}

	if controller.IsBeingDeleted(eventCfg) {
		return nil, nil
	}

	valErr := cerrors.NewValidationError(eventv1.GroupVersion.WithKind("EventConfig").GroupKind(), eventCfg)

	adminClient := eventCfg.Spec.Admin.Client
	if adminClient.Realm.IsEmpty() {
		valErr.AddInvalidError(field.NewPath("spec").Child("admin").Child("admin").Child("realm"), adminClient.Realm, "realm must be specified for admin client")
	}

	meshClient := eventCfg.Spec.Mesh.Client
	if meshClient.Realm.IsEmpty() {
		valErr.AddInvalidError(field.NewPath("spec").Child("mesh").Child("mesh").Child("realm"), meshClient.Realm, "realm must be specified for mesh client")
	}

	return valErr.BuildWarnings(), valErr.BuildError()
}
