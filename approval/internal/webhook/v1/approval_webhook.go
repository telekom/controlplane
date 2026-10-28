// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"context"
	"fmt"
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	approvalv1 "github.com/telekom/controlplane/approval/api/v1"
	approvalhandler "github.com/telekom/controlplane/approval/internal/handler/approval"
)

// log is for logging in this package.
var approvallog = logf.Log.WithName("approval-resource")

// SetupApprovalWebhookWithManager will set up the manager to manage the webhooks
func SetupApprovalWebhookWithManager(mgr ctrl.Manager, operatorServiceAccount string) error {
	return ctrl.NewWebhookManagedBy(mgr, &approvalv1.Approval{}).
		WithDefaulter(&ApprovalCustomDefaulter{}).
		WithValidator(&ApprovalCustomValidator{OperatorServiceAccount: operatorServiceAccount, Reader: mgr.GetAPIReader()}).
		Complete()
}

// +kubebuilder:webhook:path=/mutate-approval-cp-ei-telekom-de-v1-approval,mutating=true,failurePolicy=fail,sideEffects=None,groups=approval.cp.ei.telekom.de,resources=approvals,verbs=create;update,versions=v1,name=mapproval.kb.io,admissionReviewVersions=v1

// ApprovalCustomDefaulter struct is responsible for setting default values on the custom resource of the
// Kind Approval when those are created or updated.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as it is used only for temporary operations and does not need to be deeply copied.
type ApprovalCustomDefaulter struct{}

var _ admission.Defaulter[*approvalv1.Approval] = &ApprovalCustomDefaulter{}

// Default implements webhook.Defaulter so a webhook will be registered for the type
func (a *ApprovalCustomDefaulter) Default(_ context.Context, obj *approvalv1.Approval) error {
	approvallog.Info("default", "name", obj.GetName())
	if obj.Spec.Decisions == nil {
		obj.Spec.Decisions = []approvalv1.Decision{}
	}
	defaultDecisionFields(obj.Spec.Decisions, obj.Spec.State)
	// Trim last: defaulting must run on the items that are kept.
	obj.Spec.Decisions = approvalv1.TrimDecisions(obj.Spec.Decisions)
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
	// OperatorServiceAccount is the full username of the operator's service account
	// (e.g. "system:serviceaccount:system:controller-manager"). Only this identity
	// is permitted to transition an Approval to the Expired state.
	OperatorServiceAccount string
	// Reader is an uncached API reader for the ApprovalRequest a scoped Approval
	// is rebound to.
	Reader client.Reader
}

var _ admission.Validator[*approvalv1.Approval] = &ApprovalCustomValidator{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (a *ApprovalCustomValidator) ValidateCreate(_ context.Context, obj *approvalv1.Approval) (warnings admission.Warnings, err error) {
	approvallog.Info("validate create", "name", obj.Name)

	if obj.Spec.Strategy == approvalv1.ApprovalStrategyAuto && obj.Spec.State != approvalv1.ApprovalStateGranted {
		return warnings, apierrors.NewBadRequest("Auto strategy Approval must be in Granted state")
	}
	return warnings, err
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (a *ApprovalCustomValidator) ValidateUpdate(ctx context.Context, oldObj, newObj *approvalv1.Approval) (warnings admission.Warnings, err error) {
	approvallog.Info("validate update", "name", newObj.Name)

	// Key immutability: reject any non-empty add/remove/change before all other checks.
	err = validateApprovalKeyImmutability(oldObj.Spec.ApprovalKey, newObj.Spec.ApprovalKey)
	if err != nil {
		return warnings, err
	}

	stateChanged := oldObj.Spec.State != newObj.Spec.State

	// Validate FSM transitions on-the-fly using the canonical FSM definitions
	// instead of Status.AvailableTransitions (which may be stale or nil before
	// the controller has reconciled). Auto strategy uses its own FSM.
	// A scoped Approval rebound to another ApprovalRequest, which happens when a
	// grant of a newer request lifts a revocation, is validated against that
	// request instead.
	if stateChanged {
		if isScopedRebind(oldObj, newObj) {
			err = a.validateRebind(ctx, newObj)
		} else {
			err = a.validateStateTransition(ctx, oldObj, newObj)
		}
		if err != nil {
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

	// FourEyes: completing a half-approved Approval (Semigranted -> Granted) needs
	// a second, different person. Any other grant needs one person's granting
	// decision: the one-step renewal (Expired -> Granted) and resume (Suspended ->
	// Granted) the FSM allows, and a rebind, which validateRebind checked against
	// the request it is rebound to instead of the FSM.
	if newObj.Spec.Strategy == approvalv1.ApprovalStrategyFourEyes &&
		stateChanged && newObj.Spec.State == approvalv1.ApprovalStateGranted {
		if oldObj.Spec.State == approvalv1.ApprovalStateSemigranted {
			if distinctErr := validateDistinctDeciders(newObj.Spec.Decisions); distinctErr != nil {
				return warnings, distinctErr
			}
		} else if last := newObj.Spec.Decisions[len(newObj.Spec.Decisions)-1]; strings.TrimSpace(last.Email) == "" ||
			last.ResultingState != approvalv1.ApprovalStateGranted {
			return warnings, apierrors.NewBadRequest(
				"FourEyes strategy requires a granting decision with a non-empty email address")
		}
	}

	return warnings, err
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (a *ApprovalCustomValidator) ValidateDelete(_ context.Context, obj *approvalv1.Approval) (admission.Warnings, error) {
	approvallog.Info("validate delete", "name", obj.Name)

	return nil, nil
}

// validateStateTransition checks FSM validity and operator-only constraints for state changes.
func (a *ApprovalCustomValidator) validateStateTransition(ctx context.Context, oldObj, newObj *approvalv1.Approval) error {
	fsmDef, ok := approvalhandler.ApprovalStrategyFSM[newObj.Spec.Strategy]
	if !ok {
		return apierrors.NewBadRequest("Unknown approval strategy")
	}
	computed := approvalv1.AvailableTransitions(fsmDef.AvailableTransitions(oldObj.Spec.State))
	if len(computed) == 0 || !computed.HasState(newObj.Spec.State) {
		return apierrors.NewBadRequest("Invalid state transition")
	}

	// Transition to Expired is reserved for the operator — no manual expiry.
	if newObj.Spec.State == approvalv1.ApprovalStateExpired {
		return a.validateOperatorOnly(ctx, newObj)
	}
	return nil
}

// isScopedRebind reports whether an update binds a scoped Approval to another
// ApprovalRequest than before.
func isScopedRebind(oldObj, newObj *approvalv1.Approval) bool {
	oldRef, newRef := oldObj.Spec.ApprovedRequest, newObj.Spec.ApprovedRequest
	return newObj.Spec.ApprovalKey != "" && newRef != nil &&
		(oldRef == nil || *oldRef != *newRef)
}

// validateRebind allows a scoped Approval to be granted for the ApprovalRequest
// it is rebound to only when that live request is Granted with the same
// strategy for the same target and gate, and, under FourEyes, by two distinct
// deciders. Whether the request may lift a revocation is decided by the
// ApprovalRequest handler.
func (a *ApprovalCustomValidator) validateRebind(ctx context.Context, newObj *approvalv1.Approval) error {
	ref := newObj.Spec.ApprovedRequest
	if newObj.Spec.State != approvalv1.ApprovalStateGranted {
		return apierrors.NewBadRequest("an Approval rebound to another ApprovalRequest must be Granted")
	}
	if ref.Namespace != newObj.Namespace {
		return apierrors.NewBadRequest("the approved ApprovalRequest must be in the namespace of the Approval")
	}
	if a.Reader == nil {
		return apierrors.NewInternalError(fmt.Errorf("no reader for the approved ApprovalRequest"))
	}
	ar := &approvalv1.ApprovalRequest{}
	if err := a.Reader.Get(ctx, client.ObjectKey{Namespace: ref.Namespace, Name: ref.Name}, ar); err != nil {
		if apierrors.IsNotFound(err) {
			return apierrors.NewBadRequest(fmt.Sprintf("approved ApprovalRequest %s not found", ref.Name))
		}
		return apierrors.NewInternalError(fmt.Errorf("reading approved ApprovalRequest %s: %w", ref.Name, err))
	}
	switch {
	case ar.UID != ref.UID:
		return apierrors.NewBadRequest(fmt.Sprintf("approved ApprovalRequest %s has UID %s, not %s", ref.Name, ar.UID, ref.UID))
	case ar.DeletionTimestamp != nil || ar.Spec.State != approvalv1.ApprovalStateGranted:
		return apierrors.NewBadRequest(fmt.Sprintf("approved ApprovalRequest %s is not granted", ref.Name))
	case !approvalv1.ScopedIdentityMatch(ar.Spec.Target, newObj.Spec.Target, ar.Spec.ApprovalKey, newObj.Spec.ApprovalKey):
		return apierrors.NewBadRequest(fmt.Sprintf("approved ApprovalRequest %s has another scope", ref.Name))
	case ar.Spec.Strategy != newObj.Spec.Strategy:
		return apierrors.NewBadRequest(fmt.Sprintf("approved ApprovalRequest %s has strategy %s, not %s", ref.Name, ar.Spec.Strategy, newObj.Spec.Strategy))
	case ar.Spec.Strategy == approvalv1.ApprovalStrategyFourEyes:
		return validateDistinctDeciders(ar.Spec.Decisions)
	}
	return nil
}

// validateOperatorOnly ensures that only the operator service account may perform the transition.
func (a *ApprovalCustomValidator) validateOperatorOnly(ctx context.Context, obj *approvalv1.Approval) error {
	req, err := admission.RequestFromContext(ctx)
	if err != nil {
		return apierrors.NewInternalError(err)
	}
	if req.UserInfo.Username != a.OperatorServiceAccount {
		return apierrors.NewForbidden(
			approvalv1.GroupVersion.WithResource("approvals").GroupResource(),
			obj.Name,
			fmt.Errorf("only the operator service account may transition an Approval to Expired"),
		)
	}
	return nil
}
