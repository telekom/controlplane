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
var approvallog = logf.Log.WithName("approval-resource")

// SetupWebhookWithManager will setup the manager to manage the webhooks
func (a *Approval) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(a).
		WithDefaulter(a).
		WithValidator(a).
		Complete()
}

// +kubebuilder:webhook:path=/mutate-approval-cp-ei-telekom-de-v1-approval,mutating=true,failurePolicy=fail,sideEffects=None,groups=approval.cp.ei.telekom.de,resources=approvals,verbs=create;update,versions=v1,name=mapproval.kb.io,admissionReviewVersions=v1

var _ webhook.CustomDefaulter = &Approval{}

// Default implements webhook.Defaulter so a webhook will be registered for the type
func (a *Approval) Default(_ context.Context, obj runtime.Object) error {
	aObj := obj.(*Approval)
	approvallog.Info("default", "name", aObj.GetName())
	return nil
}

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
// NOTE: The 'path' attribute must follow a specific pattern and should not be modified directly here.
// Modifying the path for an invalid path can cause API server errors; failing to locate the webhook.
// +kubebuilder:webhook:path=/validate-approval-cp-ei-telekom-de-v1-approval,mutating=false,failurePolicy=fail,sideEffects=None,groups=approval.cp.ei.telekom.de,resources=approvals,verbs=create;update,versions=v1,name=vapproval.kb.io,admissionReviewVersions=v1

var _ webhook.CustomValidator = &Approval{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (a *Approval) ValidateCreate(_ context.Context, obj runtime.Object) (warnings admission.Warnings, err error) {
	aObj := obj.(*Approval)
	approvallog.Info("validate create", "name", aObj.Name)

	if aObj.Spec.Strategy == ApprovalStrategyAuto && aObj.Spec.State != ApprovalStateGranted {
		warnings = append(warnings, "Approval is auto approved and should be granted")
		aObj.Spec.State = ApprovalStateGranted
	}
	return warnings, err
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (a *Approval) ValidateUpdate(_ context.Context, _ runtime.Object, newObj runtime.Object) (warnings admission.Warnings, err error) {
	aObj := newObj.(*Approval)
	approvallog.Info("validate update", "name", aObj.Name)

	if aObj.Spec.Strategy == ApprovalStrategyAuto && aObj.Spec.State != ApprovalStateGranted {
		warnings = append(warnings, "Approval is auto approved and should be granted")
		aObj.Spec.State = ApprovalStateGranted
	}

	if aObj.StateChanged() && aObj.Status.AvailableTransitions != nil {
		if !aObj.Status.AvailableTransitions.HasState(aObj.Spec.State) {
			err = apierrors.NewBadRequest("Invalid state transition")
		}
	}
	return warnings, err
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (a *Approval) ValidateDelete(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	aObj := obj.(*Approval)
	approvallog.Info("validate delete", "name", aObj.Name)

	// TODO(user): fill in your validation logic upon object deletion.
	return nil, nil
}
