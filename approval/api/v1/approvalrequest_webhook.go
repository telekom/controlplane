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
func (ar *ApprovalRequest) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(ar).
		Complete()
}

// +kubebuilder:webhook:path=/mutate-approval-cp-ei-telekom-de-v1-approvalrequest,mutating=true,failurePolicy=fail,sideEffects=None,groups=approval.cp.ei.telekom.de,resources=approvalrequests,verbs=create;update,versions=v1,name=mapprovalrequest.kb.io,admissionReviewVersions=v1

var _ webhook.CustomDefaulter = &ApprovalRequest{}

// Default implements webhook.Defaulter so a webhook will be registered for the type
func (ar *ApprovalRequest) Default(_ context.Context, obj runtime.Object) error {
	arObj := obj.(*ApprovalRequest)
	approvalrequestlog.Info("default", "name", arObj.Name)

	if arObj.Spec.Strategy == "" {
		arObj.Spec.Strategy = ApprovalStrategySimple
	}
	if arObj.Spec.Strategy == ApprovalStrategyAuto {
		arObj.Spec.State = ApprovalStateGranted
	}
	return nil
}

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
// NOTE: The 'path' attribute must follow a specific pattern and should not be modified directly here.
// Modifying the path for an invalid path can cause API server errors; failing to locate the webhook.
// +kubebuilder:webhook:path=/validate-approval-cp-ei-telekom-de-v1-approvalrequest,mutating=false,failurePolicy=fail,sideEffects=None,groups=approval.cp.ei.telekom.de,resources=approvalrequests,verbs=create;update,versions=v1,name=vapprovalrequest.kb.io,admissionReviewVersions=v1

var _ webhook.CustomValidator = &ApprovalRequest{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (ar *ApprovalRequest) ValidateCreate(_ context.Context, obj runtime.Object) (warnings admission.Warnings, err error) {
	arObj := obj.(*ApprovalRequest)
	approvalrequestlog.Info("validate create", "name", arObj.Name)

	if arObj.Spec.Strategy == ApprovalStrategyAuto && arObj.Spec.State != ApprovalStateGranted {
		warnings = append(warnings, "Request is auto approved and should be granted")
		arObj.Spec.State = ApprovalStateGranted
	}

	return warnings, err
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (ar *ApprovalRequest) ValidateUpdate(_ context.Context, _ runtime.Object, newObj runtime.Object) (warnings admission.Warnings, err error) {
	arObj := newObj.(*ApprovalRequest)
	approvalrequestlog.Info("validate update", "name", arObj.Name)

	if arObj.Spec.Strategy == ApprovalStrategyAuto && arObj.Spec.State != ApprovalStateGranted {
		warnings = append(warnings, "Request is auto approved and should be granted")
		arObj.Spec.State = ApprovalStateGranted
	}

	if arObj.StateChanged() && arObj.Status.AvailableTransitions != nil {
		if !arObj.Status.AvailableTransitions.HasState(arObj.Spec.State) {
			err = apierrors.NewBadRequest("Invalid state transition")
		}
	}

	return warnings, err
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (ar *ApprovalRequest) ValidateDelete(_ context.Context, delObj runtime.Object) (admission.Warnings, error) {
	arObj := delObj.(*ApprovalRequest)
	approvalrequestlog.Info("validate delete", "name", arObj.Name)
	return nil, nil
}
