// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"context"
	"net/url"
	"strings"

	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/telekom/controlplane/common/pkg/config"
	"github.com/telekom/controlplane/common/pkg/controller"
	cerrors "github.com/telekom/controlplane/common/pkg/errors"
	sftpv1 "github.com/telekom/controlplane/sftp/api/v1"
)

// SetupZoneServiceConfigWebhookWithManager registers the webhook for ZoneServiceConfig in the manager.
func SetupZoneServiceConfigWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, &sftpv1.ZoneServiceConfig{}).
		WithValidator(&ZoneServiceConfigCustomValidator{}).
		Complete()
}

// +kubebuilder:webhook:path=/validate-sftp-cp-ei-telekom-de-v1-zoneserviceconfig,mutating=false,failurePolicy=fail,sideEffects=None,groups=sftp.cp.ei.telekom.de,resources=zoneserviceconfigs,verbs=create;update,versions=v1,name=vzoneserviceconfig-v1.kb.io,admissionReviewVersions=v1

var _ admission.Validator[*sftpv1.ZoneServiceConfig] = &ZoneServiceConfigCustomValidator{}

// ZoneServiceConfigCustomValidator validates ZoneServiceConfig resources.
type ZoneServiceConfigCustomValidator struct{}

// ValidateCreate implements admission.Validator for ZoneServiceConfig creates.
func (v *ZoneServiceConfigCustomValidator) ValidateCreate(_ context.Context, zsc *sftpv1.ZoneServiceConfig) (admission.Warnings, error) {
	return v.validateCreateOrUpdate(zsc)
}

// ValidateUpdate implements admission.Validator for ZoneServiceConfig updates.
func (v *ZoneServiceConfigCustomValidator) ValidateUpdate(_ context.Context, _, zsc *sftpv1.ZoneServiceConfig) (admission.Warnings, error) {
	return v.validateCreateOrUpdate(zsc)
}

// ValidateDelete implements admission.Validator for ZoneServiceConfig deletes.
func (v *ZoneServiceConfigCustomValidator) ValidateDelete(_ context.Context, _ *sftpv1.ZoneServiceConfig) (admission.Warnings, error) {
	return nil, nil
}

func (v *ZoneServiceConfigCustomValidator) validateCreateOrUpdate(zsc *sftpv1.ZoneServiceConfig) (admission.Warnings, error) {
	if controller.IsBeingDeleted(zsc) {
		return nil, nil
	}

	valErr := cerrors.NewValidationError(sftpv1.GroupVersion.WithKind("ZoneServiceConfig").GroupKind(), zsc)

	if _, ok := controller.GetEnvironment(zsc); !ok {
		valErr.AddRequiredError(field.NewPath("metadata").Child("labels").Key(config.EnvironmentLabelKey), "must contain an environment label")
	}

	apiPath := field.NewPath("spec").Child("api")
	validateRequiredURL(valErr, apiPath.Child("endpoint"), zsc.Spec.API.Endpoint, "must be a valid SFTP Tardis API base URL")
	validateRequiredURL(valErr, apiPath.Child("issuer"), zsc.Spec.API.Issuer, "must be a valid OAuth2 token endpoint URL")
	validateRequiredString(valErr, apiPath.Child("clientID"), zsc.Spec.API.ClientID, "must not be empty")
	validateRequiredString(valErr, apiPath.Child("clientSecret"), zsc.Spec.API.ClientSecret, "must not be empty")

	return valErr.BuildWarnings(), valErr.BuildError()
}

func validateRequiredString(valErr *cerrors.ValidationError, path *field.Path, value, message string) {
	if strings.TrimSpace(value) == "" {
		valErr.AddRequiredError(path, message)
	}
}

func validateRequiredURL(valErr *cerrors.ValidationError, path *field.Path, value, message string) {
	value = strings.TrimSpace(value)
	if value == "" {
		valErr.AddRequiredError(path, message)
		return
	}

	parsedURL, err := url.Parse(value)
	if err != nil || parsedURL.Scheme == "" || parsedURL.Host == "" {
		valErr.AddInvalidError(path, value, message)
	}
}
