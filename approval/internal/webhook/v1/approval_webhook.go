// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"context"

	approvalv1 "github.com/telekom/controlplane/approval/api/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// log is for logging in this package.
var approvallog = logf.Log.WithName("approval-resource")

// SetupApprovalWebhookWithManager will set up the manager to manage the webhooks
func SetupApprovalWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, &approvalv1.Approval{}).
		WithDefaulter(&ApprovalCustomDefaulter{}).
		WithValidator(&ApprovalCustomValidator{}).
		Complete()
}

// +kubebuilder:webhook:path=/mutate-approval-cp-ei-telekom-de-v1-approval,mutating=true,failurePolicy=fail,sideEffects=None,groups=approval.cp.ei.telekom.de,resources=approvals,verbs=create;update,versions=v1,name=mapproval.kb.io,admissionReviewVersions=v1

// ApprovalCustomDefaulter struct is responsible for setting default values on the custom resource of the
// Kind Approval when those are created or updated.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as it is used only for temporary operations and does not need to be deeply copied.
type ApprovalCustomDefaulter struct {
}

var _ admission.Defaulter[*approvalv1.Approval] = &ApprovalCustomDefaulter{}

// Default implements webhook.Defaulter so a webhook will be registered for the type
func (a *ApprovalCustomDefaulter) Default(_ context.Context, obj *approvalv1.Approval) error {
	approvallog.Info("default", "name", obj.GetName())
	defaultDecisionFields(obj.Spec.Decisions, obj.Spec.State)
	return nil
}

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
// NOTE: The 'path' attribute must follow a specific pattern and should not be modified directly here.
// Modifying the path for an invalid path can cause API server errors; failing to locate the webhook.
// +kubebuilder:webhook:path=/validate-approval-cp-ei-telekom-de-v1-approval,mutating=false,failurePolicy=fail,sideEffects=None,groups=approval.cp.ei.telekom.de,resources=approvals,verbs=create;update,versions=v1,name=vapproval.kb.io,admissionReviewVersions=v1

// ApprovalCustomValidator struct is responsible for validating the Approval resource
// when it is created, updated, or deleted.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as this struct is used only for temporary operations and does not need to be deeply copied.
type ApprovalCustomValidator struct {
}

var _ admission.Validator[*approvalv1.Approval] = &ApprovalCustomValidator{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (a *ApprovalCustomValidator) ValidateCreate(_ context.Context, obj *approvalv1.Approval) (warnings admission.Warnings, err error) {
	approvallog.Info("validate create", "name", obj.Name)

	if obj.Spec.Strategy == approvalv1.ApprovalStrategyAuto && obj.Spec.State != approvalv1.ApprovalStateGranted {
		warnings = append(warnings, "Approval is auto approved and should be granted")
		obj.Spec.State = approvalv1.ApprovalStateGranted
	}
	return warnings, err
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (a *ApprovalCustomValidator) ValidateUpdate(_ context.Context, oldObj *approvalv1.Approval, newObj *approvalv1.Approval) (warnings admission.Warnings, err error) {
	approvallog.Info("validate update", "name", newObj.Name)

	if newObj.Spec.Strategy == approvalv1.ApprovalStrategyAuto && newObj.Spec.State != approvalv1.ApprovalStateGranted {
		warnings = append(warnings, "Approval is auto approved and should be granted")
		newObj.Spec.State = approvalv1.ApprovalStateGranted
	}

	stateChanged := oldObj.Spec.State != newObj.Spec.State

	if stateChanged && newObj.Status.AvailableTransitions != nil {
		if !newObj.Status.AvailableTransitions.HasState(newObj.Spec.State) {
			err = apierrors.NewBadRequest("Invalid state transition")
			return warnings, err
		}
	}

	// Enforce at least one decision for any non-Auto state change
	if newObj.Spec.Strategy != approvalv1.ApprovalStrategyAuto && stateChanged {
		if len(newObj.Spec.Decisions) == 0 {
			err = apierrors.NewBadRequest("at least one decision is required when changing state")
			return warnings, err
		}
	}

	// Enforce distinct deciders for FourEyes strategy
	if newObj.Spec.Strategy == approvalv1.ApprovalStrategyFourEyes {
		if newObj.Spec.State == approvalv1.ApprovalStateGranted &&
			oldObj.Spec.State == approvalv1.ApprovalStateSemigranted {
			if err := validateDistinctDeciders(newObj.Spec.Decisions); err != nil {
				return warnings, err
			}
		}
	}

	return warnings, err
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (a *ApprovalCustomValidator) ValidateDelete(_ context.Context, obj *approvalv1.Approval) (admission.Warnings, error) {
	approvallog.Info("validate delete", "name", obj.Name)

	return nil, nil
}
