// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/telekom/controlplane/common/pkg/controller"
	cerrors "github.com/telekom/controlplane/common/pkg/errors"
	roverv1 "github.com/telekom/controlplane/rover/api/v1"
)

// SetupFileSpecificationWebhookWithManager registers the webhook for FileSpecification in the manager.
func SetupFileSpecificationWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, &roverv1.FileSpecification{}).
		WithValidator(&FileSpecificationCustomValidator{client: mgr.GetClient()}).
		Complete()
}

// +kubebuilder:webhook:path=/validate-rover-cp-ei-telekom-de-v1-filespecification,mutating=false,failurePolicy=fail,sideEffects=None,groups=rover.cp.ei.telekom.de,resources=filespecifications,verbs=create;update,versions=v1,name=vfilespecification-v1.kb.io,admissionReviewVersions=v1

type FileSpecificationCustomValidator struct {
	client client.Client
}

var _ admission.Validator[*roverv1.FileSpecification] = &FileSpecificationCustomValidator{}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type FileSpecification.
func (v *FileSpecificationCustomValidator) ValidateCreate(ctx context.Context, filespecification *roverv1.FileSpecification) (admission.Warnings, error) {
	return v.ValidateCreateOrUpdate(ctx, filespecification)
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type FileSpecification.
func (v *FileSpecificationCustomValidator) ValidateUpdate(ctx context.Context, _ *roverv1.FileSpecification, filespecification *roverv1.FileSpecification) (admission.Warnings, error) {
	return v.ValidateCreateOrUpdate(ctx, filespecification)
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type FileSpecification.
func (v *FileSpecificationCustomValidator) ValidateDelete(ctx context.Context, filespecification *roverv1.FileSpecification) (admission.Warnings, error) {
	return nil, nil
}

func (v *FileSpecificationCustomValidator) ValidateCreateOrUpdate(ctx context.Context, filespecification *roverv1.FileSpecification) (admission.Warnings, error) {

	if controller.IsBeingDeleted(filespecification) {
		return nil, nil
	}

	valErr := cerrors.NewValidationError(roverv1.GroupVersion.WithKind("FileSpecification").GroupKind(), filespecification)

	// storageType, when set, must be a supported backend (currently only "sftp").
	// The file type identifier lives in metadata.name (no spec.type field in the
	// internal CRD, per spec_dcp); the client-side name==type rule is enforced by
	// rover-server / roverctl.
	if st := filespecification.Spec.StorageType; st != "" && st != roverv1.FileStorageTypeSFTP {
		valErr.AddInvalidError(
			field.NewPath("spec").Child("storageType"),
			string(st),
			fmt.Sprintf("spec.storageType must be %q", roverv1.FileStorageTypeSFTP),
		)
	}

	return valErr.BuildWarnings(), valErr.BuildError()
}
