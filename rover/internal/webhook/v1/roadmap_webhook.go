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

// SetupRoadmapWebhookWithManager registers the webhook for Roadmap in the manager.
func SetupRoadmapWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, &roverv1.Roadmap{}).
		WithValidator(&RoadmapCustomValidator{}).
		Complete()
}

// +kubebuilder:webhook:path=/validate-rover-cp-ei-telekom-de-v1-roadmap,mutating=false,failurePolicy=fail,sideEffects=None,groups=rover.cp.ei.telekom.de,resources=roadmaps,verbs=create;update,versions=v1,name=vroadmap-v1.kb.io,admissionReviewVersions=v1

type RoadmapCustomValidator struct{}

var _ admission.Validator[*roverv1.Roadmap] = &RoadmapCustomValidator{}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type Roadmap.
func (v *RoadmapCustomValidator) ValidateCreate(ctx context.Context, roadmap *roverv1.Roadmap) (admission.Warnings, error) {
	return v.ValidateCreateOrUpdate(ctx, roadmap)
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type Roadmap.
func (v *RoadmapCustomValidator) ValidateUpdate(ctx context.Context, _ *roverv1.Roadmap, roadmap *roverv1.Roadmap) (admission.Warnings, error) {
	return v.ValidateCreateOrUpdate(ctx, roadmap)
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type Roadmap.
func (v *RoadmapCustomValidator) ValidateDelete(ctx context.Context, roadmap *roverv1.Roadmap) (admission.Warnings, error) {
	return nil, nil
}

func (v *RoadmapCustomValidator) ValidateCreateOrUpdate(ctx context.Context, roadmap *roverv1.Roadmap) (admission.Warnings, error) {

	if controller.IsBeingDeleted(roadmap) {
		return nil, nil
	}

	valErr := cerrors.NewValidationError(roverv1.GroupVersion.WithKind("Roadmap").GroupKind(), roadmap)

	// Validate environment label is present
	_, ok := controller.GetEnvironment(roadmap)
	if !ok {
		valErr.AddInvalidError(cerrors.MetadataEnvPath, "", "environment label is required")
	}

	// Validate resourceName is not empty
	if roadmap.Spec.ResourceName == "" {
		valErr.AddInvalidError(field.NewPath("spec").Child("resourceName"), roadmap.Spec.ResourceName, "resourceName must not be empty")
	}

	// Validate resourceType is valid enum (API or Event)
	// This should be caught by kubebuilder validation, but we double-check here
	if roadmap.Spec.ResourceType != roverv1.ResourceTypeAPI && roadmap.Spec.ResourceType != roverv1.ResourceTypeEvent {
		valErr.AddInvalidError(field.NewPath("spec").Child("resourceType"), string(roadmap.Spec.ResourceType), "resourceType must be either 'API' or 'Event'")
	}

	// Validate roadmap field (file ID reference) is not empty
	if roadmap.Spec.Roadmap == "" {
		valErr.AddInvalidError(field.NewPath("spec").Child("roadmap"), roadmap.Spec.Roadmap, "roadmap file ID must not be empty")
	}

	// Validate hash field is not empty
	if roadmap.Spec.Hash == "" {
		valErr.AddInvalidError(field.NewPath("spec").Child("hash"), roadmap.Spec.Hash, "hash must not be empty")
	}

	return valErr.BuildWarnings(), valErr.BuildError()
}
