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

func SetupApiChangelogWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, &roverv1.ApiChangelog{}).
		WithValidator(&ApiChangelogCustomValidator{}).
		Complete()
}

// +kubebuilder:webhook:path=/validate-rover-cp-ei-telekom-de-v1-apichangelog,mutating=false,failurePolicy=fail,sideEffects=None,groups=rover.cp.ei.telekom.de,resources=apichangelogs,verbs=create;update,versions=v1,name=vapichangelog-v1.kb.io,admissionReviewVersions=v1

type ApiChangelogCustomValidator struct{}

var _ admission.Validator[*roverv1.ApiChangelog] = &ApiChangelogCustomValidator{}

func (v *ApiChangelogCustomValidator) ValidateCreate(ctx context.Context, changelog *roverv1.ApiChangelog) (admission.Warnings, error) {
	return v.ValidateCreateOrUpdate(ctx, changelog)
}

func (v *ApiChangelogCustomValidator) ValidateUpdate(ctx context.Context, _ *roverv1.ApiChangelog, changelog *roverv1.ApiChangelog) (admission.Warnings, error) {
	return v.ValidateCreateOrUpdate(ctx, changelog)
}

func (v *ApiChangelogCustomValidator) ValidateDelete(ctx context.Context, changelog *roverv1.ApiChangelog) (admission.Warnings, error) {
	return nil, nil
}

func (v *ApiChangelogCustomValidator) ValidateCreateOrUpdate(ctx context.Context, changelog *roverv1.ApiChangelog) (admission.Warnings, error) {
	if controller.IsBeingDeleted(changelog) {
		return nil, nil
	}

	valErr := cerrors.NewValidationError(roverv1.GroupVersion.WithKind("ApiChangelog").GroupKind(), changelog)

	_, ok := controller.GetEnvironment(changelog)
	if !ok {
		valErr.AddInvalidError(cerrors.MetadataEnvPath, "", "environment label is required")
	}

	if changelog.Spec.SpecificationRef.Name == "" {
		valErr.AddInvalidError(field.NewPath("spec").Child("specificationRef").Child("name"), "", "specificationRef.name is required")
	}

	if changelog.Spec.Contents == "" {
		valErr.AddInvalidError(field.NewPath("spec").Child("contents"), "", "contents file reference is required")
	}

	if changelog.Spec.Hash == "" {
		valErr.AddInvalidError(field.NewPath("spec").Child("hash"), "", "hash is required")
	}

	return valErr.BuildWarnings(), valErr.BuildError()
}
