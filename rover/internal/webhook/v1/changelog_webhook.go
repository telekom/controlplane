// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"context"

	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/telekom/controlplane/common/pkg/controller"
	cerrors "github.com/telekom/controlplane/common/pkg/errors"
	roverv1 "github.com/telekom/controlplane/rover/api/v1"
)

func SetupChangelogWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, &roverv1.Changelog{}).
		WithValidator(&ChangelogCustomValidator{}).
		Complete()
}

// +kubebuilder:webhook:path=/validate-rover-cp-ei-telekom-de-v1-changelog,mutating=false,failurePolicy=fail,sideEffects=None,groups=rover.cp.ei.telekom.de,resources=changelogs,verbs=create;update,versions=v1,name=vchangelog-v1.kb.io,admissionReviewVersions=v1

type ChangelogCustomValidator struct{}

var _ admission.Validator[*roverv1.Changelog] = &ChangelogCustomValidator{}

func (v *ChangelogCustomValidator) ValidateCreate(ctx context.Context, changelog *roverv1.Changelog) (admission.Warnings, error) {
	return v.ValidateCreateOrUpdate(ctx, changelog)
}

func (v *ChangelogCustomValidator) ValidateUpdate(ctx context.Context, _ *roverv1.Changelog, changelog *roverv1.Changelog) (admission.Warnings, error) {
	return v.ValidateCreateOrUpdate(ctx, changelog)
}

func (v *ChangelogCustomValidator) ValidateDelete(ctx context.Context, changelog *roverv1.Changelog) (admission.Warnings, error) {
	return nil, nil
}

func (v *ChangelogCustomValidator) ValidateCreateOrUpdate(ctx context.Context, changelog *roverv1.Changelog) (admission.Warnings, error) {
	if controller.IsBeingDeleted(changelog) {
		return nil, nil
	}

	valErr := cerrors.NewValidationError(roverv1.GroupVersion.WithKind("Changelog").GroupKind(), changelog)

	_, ok := controller.GetEnvironment(changelog)
	if !ok {
		valErr.AddInvalidError(cerrors.MetadataEnvPath, "", "environment label is required")
	}

	if changelog.Spec.ResourceName == "" {
		valErr.AddInvalidError(field.NewPath("spec").Child("resourceName"), "", "resourceName is required")
	}

	if changelog.Spec.ResourceType != roverv1.ResourceTypeAPI && changelog.Spec.ResourceType != roverv1.ResourceTypeEvent {
		valErr.AddInvalidError(field.NewPath("spec").Child("resourceType"), string(changelog.Spec.ResourceType), "resourceType must be either 'API' or 'Event'")
	}

	if changelog.Spec.Changelog == "" {
		valErr.AddInvalidError(field.NewPath("spec").Child("changelog"), "", "changelog file reference is required")
	}

	if changelog.Spec.Hash == "" {
		valErr.AddInvalidError(field.NewPath("spec").Child("hash"), "", "hash is required")
	}

	return valErr.BuildWarnings(), valErr.BuildError()
}
