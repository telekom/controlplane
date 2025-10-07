// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"context"

	approvalv1 "github.com/telekom/controlplane/approval/api/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// log is for logging in this package.
var approvalrequestlog = logf.Log.WithName("approvalrequest-resource")

// SetupApprovalRequestWebhookWithManager will setup the manager to manage the webhooks
func SetupApprovalRequestWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(&approvalv1.ApprovalRequest{}).
		WithDefaulter(&ApprovalRequestCustomDefaulter{}).
		WithValidator(&ApprovalRequestCustomValidator{}).
		Complete()
}

// +kubebuilder:webhook:path=/mutate-approval-cp-ei-telekom-de-v1-approvalrequest,mutating=true,failurePolicy=fail,sideEffects=None,groups=approval.cp.ei.telekom.de,resources=approvalrequests,verbs=create;update,versions=v1,name=mapprovalrequest.kb.io,admissionReviewVersions=v1

// ApprovalRequestCustomDefaulter struct is responsible for setting default values on the custom resource of the
// Kind ApprovalRequest when those are created or updated.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as it is used only for temporary operations and does not need to be deeply copied.
type ApprovalRequestCustomDefaulter struct{}

var _ webhook.CustomDefaulter = &ApprovalRequestCustomDefaulter{}

// Default implements webhook.Defaulter so a webhook will be registered for the type
func (ar *ApprovalRequestCustomDefaulter) Default(_ context.Context, obj runtime.Object) error {
	arObj := obj.(*approvalv1.ApprovalRequest)
	approvalrequestlog.Info("default", "name", arObj.Name)

	if arObj.Spec.Strategy == "" {
		arObj.Spec.Strategy = approvalv1.ApprovalStrategySimple
	}
	if arObj.Spec.Strategy == approvalv1.ApprovalStrategyAuto {
		arObj.Spec.State = approvalv1.ApprovalStateGranted
	}
	return nil
}

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
// NOTE: The 'path' attribute must follow a specific pattern and should not be modified directly here.
// Modifying the path for an invalid path can cause API server errors; failing to locate the webhook.
// +kubebuilder:webhook:path=/validate-approval-cp-ei-telekom-de-v1-approvalrequest,mutating=false,failurePolicy=fail,sideEffects=None,groups=approval.cp.ei.telekom.de,resources=approvalrequests,verbs=create;update,versions=v1,name=vapprovalrequest.kb.io,admissionReviewVersions=v1

// ApprovalRequestCustomValidator struct is responsible for validating the ApprovalRequest resource
// when it is created, updated, or deleted.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as this struct is used only for temporary operations and does not need to be deeply copied.
type ApprovalRequestCustomValidator struct{}

var _ webhook.CustomValidator = &ApprovalRequestCustomValidator{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (ar *ApprovalRequestCustomValidator) ValidateCreate(_ context.Context, obj runtime.Object) (warnings admission.Warnings, err error) {
	arObj := obj.(*approvalv1.ApprovalRequest)
	approvalrequestlog.Info("validate create", "name", arObj.Name)

	if arObj.Spec.Strategy == approvalv1.ApprovalStrategyAuto && arObj.Spec.State != approvalv1.ApprovalStateGranted {
		warnings = append(warnings, "Request is auto approved and should be granted")
		arObj.Spec.State = approvalv1.ApprovalStateGranted
	}

	return warnings, err
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (ar *ApprovalRequestCustomValidator) ValidateUpdate(_ context.Context, _ runtime.Object, newObj runtime.Object) (warnings admission.Warnings, err error) {
	arObj := newObj.(*approvalv1.ApprovalRequest)
	approvalrequestlog.Info("validate update", "name", arObj.Name)

	if arObj.Spec.Strategy == approvalv1.ApprovalStrategyAuto && arObj.Spec.State != approvalv1.ApprovalStateGranted {
		warnings = append(warnings, "Request is auto approved and should be granted")
		arObj.Spec.State = approvalv1.ApprovalStateGranted
	}

	if arObj.StateChanged() && arObj.Status.AvailableTransitions != nil {
		if !arObj.Status.AvailableTransitions.HasState(arObj.Spec.State) {
			err = apierrors.NewBadRequest("Invalid state transition")
		}
	}

	return warnings, err
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (ar *ApprovalRequestCustomValidator) ValidateDelete(_ context.Context, delObj runtime.Object) (admission.Warnings, error) {
	arObj := delObj.(*approvalv1.ApprovalRequest)
	approvalrequestlog.Info("validate delete", "name", arObj.Name)
	return nil, nil
}
