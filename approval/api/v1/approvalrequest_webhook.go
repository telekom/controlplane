// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"context"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// log is for logging in this package.
var approvalrequestlog = logf.Log.WithName("approvalrequest-resource")

// SetupWebhookWithManager will setup the manager to manage the webhooks
func (r *ApprovalRequest) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		Complete()
}

// +kubebuilder:webhook:path=/mutate-approval-cp-ei-telekom-de-v1-approvalrequest,mutating=true,failurePolicy=fail,sideEffects=None,groups=approval.cp.ei.telekom.de,resources=approvalrequests,verbs=create;update,versions=v1,name=mapprovalrequest.kb.io,admissionReviewVersions=v1

var _ webhook.CustomDefaulter = &ApprovalRequest{}

// Default implements webhook.Defaulter so a webhook will be registered for the type
func (_ *ApprovalRequest) Default(_ context.Context, obj runtime.Object) error {
	ar := obj.(*ApprovalRequest)
	approvalrequestlog.Info("default", "name", ar.Name)

	if ar.Spec.Strategy == "" {
		ar.Spec.Strategy = ApprovalStrategySimple
	}
	if ar.Spec.Strategy == ApprovalStrategyAuto {
		ar.Spec.State = ApprovalStateGranted
	}
	return nil
}

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
// NOTE: The 'path' attribute must follow a specific pattern and should not be modified directly here.
// Modifying the path for an invalid path can cause API server errors; failing to locate the webhook.
// +kubebuilder:webhook:path=/validate-approval-cp-ei-telekom-de-v1-approvalrequest,mutating=false,failurePolicy=fail,sideEffects=None,groups=approval.cp.ei.telekom.de,resources=approvalrequests,verbs=create;update,versions=v1,name=vapprovalrequest.kb.io,admissionReviewVersions=v1

var _ webhook.CustomValidator = &ApprovalRequest{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (_ *ApprovalRequest) ValidateCreate(ctx context.Context, obj runtime.Object) (warnings admission.Warnings, err error) {
	ar := obj.(*ApprovalRequest)
	approvalrequestlog.Info("validate create", "name", ar.Name)

	if ar.Spec.Strategy == ApprovalStrategyAuto && ar.Spec.State != ApprovalStateGranted {
		warnings = append(warnings, "Request is auto approved and should be granted")
		ar.Spec.State = ApprovalStateGranted
	}

	return warnings, err
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (_ *ApprovalRequest) ValidateUpdate(_ context.Context, _ runtime.Object, newObj runtime.Object) (warnings admission.Warnings, err error) {
	ar := newObj.(*ApprovalRequest)
	approvalrequestlog.Info("validate update", "name", ar.Name)

	if ar.Spec.Strategy == ApprovalStrategyAuto && ar.Spec.State != ApprovalStateGranted {
		warnings = append(warnings, "Request is auto approved and should be granted")
		ar.Spec.State = ApprovalStateGranted
	}

	if ar.StateChanged() && ar.Status.AvailableTransitions != nil {
		if !ar.Status.AvailableTransitions.HasState(ar.Spec.State) {
			err = apierrors.NewBadRequest("Invalid state transition")
		}
	}

	return warnings, err
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (r *ApprovalRequest) ValidateDelete(_ context.Context, _ runtime.Object) (admission.Warnings, error) {
	approvalrequestlog.Info("validate delete", "name", r.Name)
	return nil, nil
}
