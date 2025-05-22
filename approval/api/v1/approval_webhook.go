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
func (r *Approval) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		Complete()
}

// +kubebuilder:webhook:path=/mutate-approval-cp-ei-telekom-de-v1-approval,mutating=true,failurePolicy=fail,sideEffects=None,groups=approval.cp.ei.telekom.de,resources=approvals,verbs=create;update,versions=v1,name=mapproval.kb.io,admissionReviewVersions=v1

var _ webhook.CustomDefaulter = &Approval{}

// Default implements webhook.Defaulter so a webhook will be registered for the type
func (_ *Approval) Default(_ context.Context, obj runtime.Object) error {
	a := obj.(*Approval)
	approvallog.Info("default", "name", a.GetName())
	return nil
}

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
// NOTE: The 'path' attribute must follow a specific pattern and should not be modified directly here.
// Modifying the path for an invalid path can cause API server errors; failing to locate the webhook.
// +kubebuilder:webhook:path=/validate-approval-cp-ei-telekom-de-v1-approval,mutating=false,failurePolicy=fail,sideEffects=None,groups=approval.cp.ei.telekom.de,resources=approvals,verbs=create;update,versions=v1,name=vapproval.kb.io,admissionReviewVersions=v1

var _ webhook.CustomValidator = &Approval{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (_ *Approval) ValidateCreate(_ context.Context, obj runtime.Object) (warnings admission.Warnings, err error) {
	a := obj.(*Approval)
	approvallog.Info("validate create", "name", a.Name)

	if a.Spec.Strategy == ApprovalStrategyAuto && a.Spec.State != ApprovalStateGranted {
		warnings = append(warnings, "Approval is auto approved and should be granted")
		a.Spec.State = ApprovalStateGranted
	}
	return warnings, err
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (_ *Approval) ValidateUpdate(_ context.Context, _ runtime.Object, newObj runtime.Object) (warnings admission.Warnings, err error) {
	a := newObj.(*Approval)
	approvallog.Info("validate update", "name", a.Name)

	if a.Spec.Strategy == ApprovalStrategyAuto && a.Spec.State != ApprovalStateGranted {
		warnings = append(warnings, "Approval is auto approved and should be granted")
		a.Spec.State = ApprovalStateGranted
	}

	if a.StateChanged() && a.Status.AvailableTransitions != nil {
		if !a.Status.AvailableTransitions.HasState(a.Spec.State) {
			err = apierrors.NewBadRequest("Invalid state transition")
		}
	}
	return warnings, err
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (_ *Approval) ValidateDelete(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	a := obj.(*Approval)
	approvallog.Info("validate delete", "name", a.Name)

	// TODO(user): fill in your validation logic upon object deletion.
	return nil, nil
}
