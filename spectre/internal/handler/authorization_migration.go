// SPDX-FileCopyrightText: 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package handler

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/log"

	approvalapi "github.com/telekom/controlplane/approval/api/v1"
	cclient "github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/condition"
	ctypes "github.com/telekom/controlplane/common/pkg/types"
	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
	pubsubv1 "github.com/telekom/controlplane/pubsub/api/v1"
	spectrev1 "github.com/telekom/controlplane/spectre/api/v1"
)

// Migration phase constants match the AuthorizationMigrationStatus.Phase enum.
const (
	MigrationPhaseRecorded         = "Recorded"
	MigrationPhaseBlocked          = "Blocked"
	MigrationPhaseAwaitingScoped   = "AwaitingScoped"
	MigrationPhaseDraining         = "Draining"
	MigrationPhaseRetiringRequests = "RetiringRequests"
	MigrationPhaseRetiringApproval = "RetiringApproval"
)

// authorizationPolicyV2 is the marker written to ListenerStatus when migration
// is complete (or when a fresh install enters v2 directly).
const authorizationPolicyV2 = "v2"

// migrationContext holds the resolved legacy evidence for the current reconcile.
type migrationContext struct {
	listener *spectrev1.Listener
	intent   *authorizationIntent

	// Resolved legacy evidence (nil when not found).
	legacyApproval    *approvalapi.Approval
	legacyApprovalRef *ctypes.ObjectRef
	legacyRequest     *approvalapi.ApprovalRequest
	legacyRequestRef  *ctypes.ObjectRef
}

// isFreshInstall returns true when the Listener can enter v2 directly:
//   - AuthorizationPolicyVersion is already "v2", OR
//   - No legacy Approval exists AND no recorded migration status
func (h *ListenerHandler) isFreshInstall(ctx context.Context, listener *spectrev1.Listener) (bool, error) {
	// Already migrated.
	if listener.Status.AuthorizationPolicyVersion == authorizationPolicyV2 {
		return true, nil
	}

	// Recorded migration in progress.
	if listener.Status.AuthorizationMigration != nil {
		return false, nil
	}

	// Check for legacy Approval by name convention.
	legacyName := approvalapi.ApprovalName("Listener", listener.Name)
	c := cclient.ClientFromContextOrDie(ctx)

	approval := &approvalapi.Approval{}
	err := c.Get(ctx, k8stypes.NamespacedName{
		Name:      legacyName,
		Namespace: listener.Namespace,
	}, approval)

	if err != nil {
		if apierrors.IsNotFound(err) {
			// No legacy approval by name — check for old unlabelled children.
			return h.noOldUnlabelledChildren(ctx, c, listener)
		}
		return false, errors.Wrapf(err, "failed to check legacy Approval %q", legacyName)
	}

	// Legacy approval exists — not a fresh install.
	return false, nil
}

// noOldUnlabelledChildren returns true only when no owner-labelled
// RouteListeners or Subscribers exist that lack the authorization fingerprint
// label. Such children are prior-policy artifacts that need migration/drain.
func (h *ListenerHandler) noOldUnlabelledChildren(
	ctx context.Context,
	c cclient.JanitorClient,
	listener *spectrev1.Listener,
) (bool, error) {
	rlList := &gatewayv1.RouteListenerList{}
	if err := c.List(ctx, rlList, cclient.OwnedByLabel(listener)...); err != nil {
		return false, errors.Wrap(err, "failed to list owned RouteListeners for fresh-install check")
	}
	for i := range rlList.Items {
		if _, ok := rlList.Items[i].Labels[AuthorizationFingerprintLabelKey]; !ok {
			return false, nil // old unlabelled child exists
		}
	}

	subList := &pubsubv1.SubscriberList{}
	if err := c.List(ctx, subList, cclient.OwnedByLabel(listener)...); err != nil {
		return false, errors.Wrap(err, "failed to list owned Subscribers for fresh-install check")
	}
	for i := range subList.Items {
		if _, ok := subList.Items[i].Labels[AuthorizationFingerprintLabelKey]; !ok {
			return false, nil // old unlabelled child exists
		}
	}

	return true, nil
}

// advanceMigration drives the six-step migration protocol. It returns:
//   - true  if migration is complete (v2 ready, proceed to provisioning)
//   - false if migration is in progress (do not provision)
//   - error on unrecoverable failure
//
// Called from the handler before provisioning. Task 10 will wire this call.
func (h *ListenerHandler) advanceMigration(
	ctx context.Context,
	listener *spectrev1.Listener,
	intent *authorizationIntent,
	dual *dualApprovalResult,
) (bool, error) {
	logger := log.FromContext(ctx)

	// Already migrated — nothing to do.
	if listener.Status.AuthorizationPolicyVersion == authorizationPolicyV2 {
		return true, nil
	}

	// Step 1: Discover and record legacy evidence.
	mctx, err := h.discoverLegacyEvidence(ctx, listener, intent)
	if err != nil {
		return false, errors.Wrap(err, "migration: discover legacy evidence")
	}

	// No legacy evidence and no recorded migration — enter v2 directly.
	if mctx.legacyApproval == nil && listener.Status.AuthorizationMigration == nil {
		logger.Info("No legacy evidence found, entering v2 directly")
		listener.Status.AuthorizationPolicyVersion = authorizationPolicyV2
		return true, nil
	}

	// Ensure migration status is initialized.
	if listener.Status.AuthorizationMigration == nil {
		listener.Status.AuthorizationMigration = &spectrev1.AuthorizationMigrationStatus{
			TargetPolicyVersion: authorizationPolicyV2,
			Phase:               MigrationPhaseRecorded,
		}
		// Record legacy refs.
		if mctx.legacyApprovalRef != nil {
			listener.Status.AuthorizationMigration.LegacyApproval = mctx.legacyApprovalRef
		}
		if mctx.legacyRequestRef != nil {
			listener.Status.AuthorizationMigration.LegacyRequests = []ctypes.ObjectRef{*mctx.legacyRequestRef}
		}
		logger.Info("Migration recorded", "phase", MigrationPhaseRecorded)
	}

	migration := listener.Status.AuthorizationMigration

	// Step 2: Check for restrictive or unresolved decisions.
	if mctx.legacyApproval != nil {
		blocked, reason := isLegacyBlocked(mctx.legacyApproval)
		if blocked {
			migration.Phase = MigrationPhaseBlocked
			listener.SetCondition(condition.NewNotReadyCondition("LegacyApprovalMigrationBlocked", reason))
			listener.SetCondition(condition.NewBlockedCondition(reason))
			logger.Info("Migration blocked by legacy decision", "reason", reason)
			return false, nil
		}
	}

	// Validate that recorded legacy evidence still exists and matches.
	if migration.LegacyApproval != nil && mctx.legacyApproval == nil {
		// Evidence was recorded but is now missing — block.
		migration.Phase = MigrationPhaseBlocked
		reason := "Legacy Approval was previously recorded but is now missing or recreated"
		listener.SetCondition(condition.NewNotReadyCondition("LegacyApprovalMigrationBlocked", reason))
		listener.SetCondition(condition.NewBlockedCondition(reason))
		logger.Info("Migration blocked: recorded evidence missing", "approvalRef", migration.LegacyApproval.String())
		return false, nil
	}

	// UID cross-check: catch a legacy Approval that was deleted and recreated
	// with the same name — the new object is not the original consent.
	if migration.LegacyApproval != nil && mctx.legacyApprovalRef != nil &&
		migration.LegacyApproval.UID != mctx.legacyApprovalRef.UID {
		migration.Phase = MigrationPhaseBlocked
		reason := fmt.Sprintf("Legacy Approval %q UID changed: recorded %s, found %s — recreated evidence is not consent",
			migration.LegacyApproval.Name, migration.LegacyApproval.UID, mctx.legacyApprovalRef.UID)
		listener.SetCondition(condition.NewNotReadyCondition("LegacyApprovalMigrationBlocked", reason))
		listener.SetCondition(condition.NewBlockedCondition(reason))
		return false, nil
	}

	// Step 3: Advance to AwaitingScoped — the new scoped requests are created
	// by the normal ensureApprovals path (dual-gate). The migration only needs
	// to verify the dual result converges.
	if migration.Phase == MigrationPhaseRecorded {
		migration.Phase = MigrationPhaseAwaitingScoped
		logger.Info("Migration advancing to AwaitingScoped")
	}

	if migration.Phase == MigrationPhaseBlocked {
		// Re-evaluate: the blocking condition may have cleared.
		if mctx.legacyApproval != nil {
			blocked, _ := isLegacyBlocked(mctx.legacyApproval)
			if blocked {
				return false, nil
			}
		}
		migration.Phase = MigrationPhaseAwaitingScoped
		logger.Info("Migration unblocked, advancing to AwaitingScoped")
	}

	// Check if the dual-gate result is granted.
	if migration.Phase == MigrationPhaseAwaitingScoped {
		if dual == nil || dual.outcome != outcomeGranted {
			logger.Info("Migration waiting for scoped approvals to be granted",
				"dualOutcome", dualOutcomeString(dual))
			return false, nil
		}
		// Both scoped gates granted — proceed to drain.
		migration.Phase = MigrationPhaseDraining
		logger.Info("Scoped approvals granted, advancing to Draining")
	}

	// Step 4: Drain old capture (stub — Task 10 implements the full protocol).
	if migration.Phase == MigrationPhaseDraining {
		// Stub: mark as draining and proceed immediately.
		// Task 10 will implement: wait for fingerprint change, drain old registrations.
		migration.Phase = MigrationPhaseRetiringRequests
		logger.Info("Drain stub complete, advancing to RetiringRequests")
	}

	// Step 5: Retire legacy resources conditionally.
	if migration.Phase == MigrationPhaseRetiringRequests {
		retired, err := h.retireLegacyRequests(ctx, mctx, migration)
		if err != nil {
			return false, errors.Wrap(err, "migration: retire legacy requests")
		}
		if !retired {
			return false, nil
		}
		migration.Phase = MigrationPhaseRetiringApproval
		logger.Info("Legacy requests retired, advancing to RetiringApproval")
	}

	if migration.Phase == MigrationPhaseRetiringApproval {
		retired, err := h.retireLegacyApproval(ctx, mctx, migration)
		if err != nil {
			return false, errors.Wrap(err, "migration: retire legacy approval")
		}
		if !retired {
			return false, nil
		}
	}

	// Step 6: Complete migration.
	listener.Status.AuthorizationPolicyVersion = authorizationPolicyV2
	listener.Status.AuthorizationMigration = nil
	logger.Info("Migration complete, policy version set to v2")
	return true, nil
}

// discoverLegacyEvidence locates the legacy Approval and its bound request.
// It validates ownership and binding before returning the evidence.
func (h *ListenerHandler) discoverLegacyEvidence(
	ctx context.Context,
	listener *spectrev1.Listener,
	intent *authorizationIntent,
) (*migrationContext, error) {
	c := cclient.ClientFromContextOrDie(ctx)
	mctx := &migrationContext{
		listener: listener,
		intent:   intent,
	}

	legacyName := approvalapi.ApprovalName("Listener", listener.Name)
	approval := &approvalapi.Approval{}
	err := c.Get(ctx, k8stypes.NamespacedName{
		Name:      legacyName,
		Namespace: listener.Namespace,
	}, approval)

	if err != nil {
		if apierrors.IsNotFound(err) {
			return mctx, nil
		}
		return nil, errors.Wrapf(err, "failed to get legacy Approval %q", legacyName)
	}

	// Validate: same namespace.
	if approval.Namespace != listener.Namespace {
		return mctx, nil // foreign object
	}

	// Validate: target references this Listener.
	if approval.Spec.Target.Name != listener.Name ||
		approval.Spec.Target.Kind != "Listener" ||
		(approval.Spec.Target.UID != "" && approval.Spec.Target.UID != listener.UID) {
		return mctx, nil // foreign object
	}

	// Validate: controller owner reference.
	ownerValid := false
	for _, ref := range approval.GetOwnerReferences() {
		if ref.UID == listener.UID && ref.Controller != nil && *ref.Controller {
			ownerValid = true
			break
		}
	}
	if !ownerValid {
		return mctx, nil // not owned by this Listener
	}

	// Validate: empty approvalKey (legacy unscoped).
	if approval.Spec.ApprovalKey != "" {
		return mctx, nil // scoped approval, not legacy
	}

	mctx.legacyApproval = approval
	mctx.legacyApprovalRef = ctypes.ObjectRefFromObject(approval)

	// Discover the bound ApprovalRequest.
	if approval.Spec.ApprovedRequest != nil {
		ar := &approvalapi.ApprovalRequest{}
		err := c.Get(ctx, k8stypes.NamespacedName{
			Name:      approval.Spec.ApprovedRequest.Name,
			Namespace: approval.Spec.ApprovedRequest.Namespace,
		}, ar)
		if err != nil {
			if !apierrors.IsNotFound(err) {
				return nil, errors.Wrapf(err, "failed to get legacy ApprovalRequest %q",
					approval.Spec.ApprovedRequest.Name)
			}
			// Request is gone — still record the Approval.
		} else {
			mctx.legacyRequest = ar
			mctx.legacyRequestRef = ctypes.ObjectRefFromObject(ar)
		}
	}

	return mctx, nil
}

// isLegacyBlocked returns true if the legacy Approval's state blocks migration.
// Rejected, Suspended, and Pending states block. Expired blocks only if the
// last state was Suspended (expired-from-granted may proceed).
func isLegacyBlocked(approval *approvalapi.Approval) (bool, string) {
	switch approval.Spec.State {
	case approvalapi.ApprovalStateRejected:
		return true, fmt.Sprintf("Legacy Approval %q is Rejected — migration blocked", approval.Name)
	case approvalapi.ApprovalStateSuspended:
		return true, fmt.Sprintf("Legacy Approval %q is Suspended — migration blocked", approval.Name)
	case approvalapi.ApprovalStatePending:
		return true, fmt.Sprintf("Legacy Approval %q is Pending — migration blocked", approval.Name)
	case approvalapi.ApprovalStateExpired:
		// Expired-from-Suspended blocks; expired-from-Granted may proceed.
		if approval.Status.LastState == approvalapi.ApprovalStateSuspended {
			return true, fmt.Sprintf("Legacy Approval %q expired from Suspended — migration blocked", approval.Name)
		}
		return false, ""
	case approvalapi.ApprovalStateGranted:
		return false, ""
	default:
		// Unknown state — block conservatively.
		return true, fmt.Sprintf("Legacy Approval %q has unknown state %q — migration blocked",
			approval.Name, approval.Spec.State)
	}
}

// retireLegacyRequests deletes legacy ApprovalRequests with conditional checks.
// Returns true when all legacy requests are retired.
func (h *ListenerHandler) retireLegacyRequests(
	ctx context.Context,
	mctx *migrationContext,
	migration *spectrev1.AuthorizationMigrationStatus,
) (bool, error) {
	logger := log.FromContext(ctx)

	// Already retired in a previous reconcile.
	if migration.RetirementCheckpoint != nil && migration.RetirementCheckpoint.RequestsRetired {
		return true, nil
	}

	if migration.RetirementCheckpoint == nil {
		migration.RetirementCheckpoint = &spectrev1.MigrationRetirementCheckpoint{}
	}

	c := cclient.ClientFromContextOrDie(ctx)

	// Retire each recorded legacy request.
	for _, ref := range migration.LegacyRequests {
		ar := &approvalapi.ApprovalRequest{}
		err := c.Get(ctx, k8stypes.NamespacedName{
			Name:      ref.Name,
			Namespace: ref.Namespace,
		}, ar)
		if err != nil {
			if apierrors.IsNotFound(err) {
				// Already gone.
				continue
			}
			return false, errors.Wrapf(err, "fresh read of legacy ApprovalRequest %q", ref.Name)
		}

		// Conditional check: UID must match what we recorded.
		if ref.UID != "" && ar.UID != ref.UID {
			return false, fmt.Errorf("legacy ApprovalRequest %q UID mismatch: recorded %s, found %s — aborting retirement",
				ref.Name, ref.UID, ar.UID)
		}

		// Record checkpoint before deletion.
		migration.RetirementCheckpoint.LastRetiredUID = string(ar.UID)
		migration.RetirementCheckpoint.LastRetiredResourceVersion = ar.ResourceVersion

		if err := c.Delete(ctx, ar); err != nil && !apierrors.IsNotFound(err) {
			return false, errors.Wrapf(err, "failed to delete legacy ApprovalRequest %q", ref.Name)
		}
		logger.Info("Retired legacy ApprovalRequest", "name", ref.Name)
	}

	migration.RetirementCheckpoint.RequestsRetired = true
	return true, nil
}

// retireLegacyApproval deletes the legacy Approval with conditional checks.
// Returns true when the legacy Approval is retired.
func (h *ListenerHandler) retireLegacyApproval(
	ctx context.Context,
	mctx *migrationContext,
	migration *spectrev1.AuthorizationMigrationStatus,
) (bool, error) {
	logger := log.FromContext(ctx)

	// Already retired.
	if migration.RetirementCheckpoint != nil && migration.RetirementCheckpoint.ApprovalRetired {
		return true, nil
	}

	if migration.RetirementCheckpoint == nil {
		migration.RetirementCheckpoint = &spectrev1.MigrationRetirementCheckpoint{}
	}

	if migration.LegacyApproval == nil {
		// No legacy approval to retire.
		migration.RetirementCheckpoint.ApprovalRetired = true
		return true, nil
	}

	ref := migration.LegacyApproval
	c := cclient.ClientFromContextOrDie(ctx)
	approval := &approvalapi.Approval{}
	err := c.Get(ctx, k8stypes.NamespacedName{
		Name:      ref.Name,
		Namespace: ref.Namespace,
	}, approval)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// Already gone.
			migration.RetirementCheckpoint.ApprovalRetired = true
			return true, nil
		}
		return false, errors.Wrapf(err, "fresh read of legacy Approval %q", ref.Name)
	}

	// Conditional check: UID must match.
	if ref.UID != "" && approval.UID != ref.UID {
		return false, fmt.Errorf("legacy Approval %q UID mismatch: recorded %s, found %s — aborting retirement",
			ref.Name, ref.UID, approval.UID)
	}

	// Record checkpoint before deletion.
	migration.RetirementCheckpoint.LastRetiredUID = string(approval.UID)
	migration.RetirementCheckpoint.LastRetiredResourceVersion = approval.ResourceVersion

	if err := c.Delete(ctx, approval); err != nil && !apierrors.IsNotFound(err) {
		return false, errors.Wrapf(err, "failed to delete legacy Approval %q", ref.Name)
	}

	logger.Info("Retired legacy Approval", "name", ref.Name)
	migration.RetirementCheckpoint.ApprovalRetired = true
	return true, nil
}

// dualOutcomeString safely returns the string representation of a dual result.
func dualOutcomeString(dual *dualApprovalResult) string {
	if dual == nil {
		return "nil"
	}
	return dual.outcome.String()
}
