// SPDX-FileCopyrightText: 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package handler

import (
	"context"
	"fmt"
	"strings"

	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
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

	// Check for retained legacy approval references in status. A ProviderApproval
	// that does not carry the scoped "ag-v1-" prefix is a legacy unscoped ref —
	// not a fresh install.
	if listener.Status.ProviderApproval != nil {
		if !strings.HasPrefix(listener.Status.ProviderApproval.Name, "ag-v1-") {
			return false, nil
		}
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

// decideFreshness is the freshness decision for a Listener that is neither v2
// nor migrating. A fresh install enters v2 directly; any other Listener gets
// its migration record now (recordLegacyEvidence), persisted with this
// reconcile's status. advanceMigration continues from it once the dual-gate
// result is available.
func (h *ListenerHandler) decideFreshness(ctx context.Context, listener *spectrev1.Listener) error {
	if listener.Status.AuthorizationPolicyVersion == authorizationPolicyV2 || listener.Status.AuthorizationMigration != nil {
		return nil
	}
	fresh, err := h.isFreshInstall(ctx, listener)
	if err != nil {
		return errors.Wrap(err, "failed to check fresh install")
	}
	if fresh {
		listener.Status.AuthorizationPolicyVersion = authorizationPolicyV2
		return nil
	}
	if _, _, err = h.recordLegacyEvidence(ctx, listener); err != nil {
		return errors.Wrap(err, "failed to record migration")
	}
	return nil
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

	// Step 1: Discover and record legacy evidence. The handler normally recorded
	// it before any child could be removed; this continues from that record.
	mctx, recordedBlocked, err := h.recordLegacyEvidence(ctx, listener)
	if err != nil {
		return false, err
	}
	mctx.intent = intent
	if recordedBlocked {
		return false, nil
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
		// Check if the absence is expected because we have a pending deletion
		// checkpoint for this Approval. After retireLegacyApproval deletes the
		// object, the next reconcile's discovery will find nothing — that is the
		// happy path, not a blocking condition.
		if !hasPendingDeletionFor(migration, "Approval", migration.LegacyApproval) {
			// Unexplained absence — block.
			migration.Phase = MigrationPhaseBlocked
			reason := "Legacy Approval was previously recorded but is now missing or recreated"
			listener.SetCondition(condition.NewNotReadyCondition("LegacyApprovalMigrationBlocked", reason))
			listener.SetCondition(condition.NewBlockedCondition(reason))
			logger.Info("Migration blocked: recorded evidence missing", "approvalRef", migration.LegacyApproval.String())
			return false, nil
		}
		// Expected absence after our own deletion — let the retirement phase handle it.
		logger.V(1).Info("Approval absent but pending deletion checkpoint exists, continuing", "approvalRef", migration.LegacyApproval.String())
	}

	// UID cross-check: catch a legacy Approval that was deleted and recreated
	// with the same name — the new object is not the original consent.
	// Skip if the Approval is absent due to a pending deletion (handled above).
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

	// Step 4: Drain old capture via the shared drain protocol. Step 5.10 of the
	// handler usually drained the prior-policy children already; with nothing
	// left no empty drain is started and retirement follows directly.
	if migration.Phase == MigrationPhaseDraining {
		switch {
		case listener.Status.Draining != nil:
			complete, err := h.continueDrain(ctx, listener)
			if err != nil {
				return false, err
			}
			if !complete {
				return false, nil // drain still in progress
			}
		case !migration.DrainStarted:
			started, err := h.drainCapture(ctx, listener, "legacy migration")
			if err != nil {
				return false, err
			}
			if started {
				migration.DrainStarted = true
				return false, nil // persist drain checkpoint
			}
		}
		// The drain completed (here or in the handler's step 0) or there was
		// nothing to drain. Clear the old applied fingerprint so step 5.9 does not
		// re-trigger a drain.
		if listener.Status.AppliedPlacement != nil {
			listener.Status.AppliedPlacement.Fingerprint = ""
		}
		migration.Phase = MigrationPhaseRetiringRequests
		logger.Info("Old capture drained, advancing to RetiringRequests")
	}

	// Step 5: Retire legacy resources conditionally. Each retirement function
	// receives the current dual-gate result so it can verify authorization is
	// still Granted before issuing NEW deletions (not when observing
	// already-prepared ones).
	if migration.Phase == MigrationPhaseRetiringRequests {
		retired, err := h.retireLegacyRequests(ctx, mctx, migration, dual)
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
		retired, err := h.retireLegacyApproval(ctx, mctx, migration, dual)
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

// recordLegacyEvidence discovers the legacy evidence and, when no migration is
// recorded yet, records one: Recorded with the legacy refs when a legacy
// Approval exists, Blocked otherwise (prior-policy evidence without a legacy
// Approval). It reports whether it just recorded a Blocked migration.
//
// decideFreshness calls it before placement, fingerprint or stale-child
// handling can drain a child, so the not-fresh decision is persisted with that
// reconcile's status and a Listener whose prior-policy children were drained is
// never classified fresh afterwards (isFreshInstall returns false once a record
// exists).
func (h *ListenerHandler) recordLegacyEvidence(ctx context.Context, listener *spectrev1.Listener) (*migrationContext, bool, error) {
	logger := log.FromContext(ctx)

	mctx, err := h.discoverLegacyEvidence(ctx, listener)
	if err != nil {
		return nil, false, errors.Wrap(err, "migration: discover legacy evidence")
	}
	if listener.Status.AuthorizationMigration != nil {
		return mctx, false, nil
	}

	// isFreshInstall is the SOLE freshness decision and runs before this. If we
	// reach here (isFreshInstall returned false) but find no legacy Approval and
	// no migration status, something is inconsistent — block rather than
	// silently granting v2.
	//
	// CRITICAL: Persist a migration record so the next reconcile's isFreshInstall
	// sees AuthorizationMigration != nil and returns false. Without this record,
	// ensureApprovals may overwrite the old unscoped ProviderApproval ref with a
	// scoped "ag-v1-" ref, or the prior-policy children may be drained, causing
	// isFreshInstall to see no legacy evidence and no migration record —
	// misclassifying as fresh and granting v2.
	if mctx.legacyApproval == nil {
		listener.Status.AuthorizationMigration = &spectrev1.AuthorizationMigrationStatus{
			TargetPolicyVersion: authorizationPolicyV2,
			Phase:               MigrationPhaseBlocked,
		}
		listener.SetCondition(condition.NewNotReadyCondition("LegacyApprovalMigrationBlocked",
			"No legacy Approval found but isFreshInstall returned false — cannot determine policy"))
		listener.SetCondition(condition.NewBlockedCondition(
			"No legacy Approval found but migration evidence exists"))
		logger.Info("Migration blocked: no legacy Approval but isFreshInstall was false")
		return mctx, true, nil
	}

	listener.Status.AuthorizationMigration = &spectrev1.AuthorizationMigrationStatus{
		TargetPolicyVersion: authorizationPolicyV2,
		Phase:               MigrationPhaseRecorded,
		LegacyApproval:      mctx.legacyApprovalRef,
	}
	if mctx.legacyRequestRef != nil {
		listener.Status.AuthorizationMigration.LegacyRequests = []ctypes.ObjectRef{*mctx.legacyRequestRef}
	}
	logger.Info("Migration recorded", "phase", MigrationPhaseRecorded)
	return mctx, false, nil
}

// discoverLegacyEvidence locates the legacy Approval and its bound request.
// It validates ownership and binding before returning the evidence.
func (h *ListenerHandler) discoverLegacyEvidence(
	ctx context.Context,
	listener *spectrev1.Listener,
) (*migrationContext, error) {
	c := cclient.ClientFromContextOrDie(ctx)
	mctx := &migrationContext{listener: listener}

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

// PendingDeletion phase constants.
const (
	PendingDeletionPhasePrepared = "DeletePrepared"
	PendingDeletionPhaseObserved = "DeleteObserved"
)

// findPendingDeletion returns the index of a PendingDeletion for the given
// kind+name+namespace, or -1 if not found.
func findPendingDeletion(pds []spectrev1.PendingDeletion, kind, name, ns string) int {
	for i := range pds {
		if pds[i].Kind == kind && pds[i].Name == name && pds[i].Namespace == ns {
			return i
		}
	}
	return -1
}

// hasPendingDeletionFor returns true if the migration has a PendingDeletion
// checkpoint in the DeletePrepared phase for the given kind whose UID matches
// the recorded ObjectRef. This indicates the controller itself deleted the
// resource and is waiting to observe the NotFound confirmation.
func hasPendingDeletionFor(migration *spectrev1.AuthorizationMigrationStatus, kind string, ref *ctypes.ObjectRef) bool {
	if migration.RetirementCheckpoint == nil || ref == nil {
		return false
	}
	idx := findPendingDeletion(migration.RetirementCheckpoint.PendingDeletions, kind, ref.Name, ref.Namespace)
	if idx < 0 {
		return false
	}
	pd := &migration.RetirementCheckpoint.PendingDeletions[idx]
	return pd.Phase == PendingDeletionPhasePrepared && pd.UID == string(ref.UID)
}

// retireLegacyRequests deletes legacy ApprovalRequests using per-resource
// deletion checkpoints. Each request goes through:
//  1. Fresh read + UID check -> record PendingDeletion(DeletePrepared) -> return
//  2. Next reconcile: re-read, if UID/RV match -> delete
//  3. Next reconcile: verify NotFound -> mark DeleteObserved
//
// Returns true when all legacy requests are retired.
func (h *ListenerHandler) retireLegacyRequests(
	ctx context.Context,
	mctx *migrationContext,
	migration *spectrev1.AuthorizationMigrationStatus,
	dual *dualApprovalResult,
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
	cp := migration.RetirementCheckpoint

	for _, ref := range migration.LegacyRequests {
		idx := findPendingDeletion(cp.PendingDeletions, "ApprovalRequest", ref.Name, ref.Namespace)

		// Already observed as deleted.
		if idx >= 0 && cp.PendingDeletions[idx].Phase == PendingDeletionPhaseObserved {
			continue
		}

		// Live read: the pre-delete review must see the current instance.
		ar := &approvalapi.ApprovalRequest{}
		err := h.getLive(ctx, k8stypes.NamespacedName{
			Name:      ref.Name,
			Namespace: ref.Namespace,
		}, ar)

		if err != nil {
			if apierrors.IsNotFound(err) {
				if idx >= 0 && cp.PendingDeletions[idx].Phase == PendingDeletionPhasePrepared {
					// Phase-aware recovery: we prepared deletion and the object
					// is gone — the delete succeeded before we could persist the ack.
					cp.PendingDeletions[idx].Phase = PendingDeletionPhaseObserved
					logger.Info("Recovered deletion: ApprovalRequest gone after DeletePrepared", "name", ref.Name)
					continue
				}
				if idx < 0 {
					// No checkpoint recorded but object is missing — unexplained
					// absence. Block rather than silently proceeding.
					return false, fmt.Errorf("legacy ApprovalRequest %q missing without deletion checkpoint — unexplained absence", ref.Name)
				}
				continue
			}
			return false, errors.Wrapf(err, "fresh read of legacy ApprovalRequest %q", ref.Name)
		}

		// Conditional check: UID must match what we recorded.
		if ref.UID != "" && ar.UID != ref.UID {
			return false, fmt.Errorf("legacy ApprovalRequest %q UID mismatch: recorded %s, found %s — aborting retirement",
				ref.Name, ref.UID, ar.UID)
		}

		if idx < 0 {
			// About to issue a NEW deletion — verify the dual-gate result is
			// still Granted. If authorization has regressed since AwaitingScoped,
			// hold retirement to avoid deleting legacy consent without replacement.
			if dual == nil || dual.outcome != outcomeGranted {
				return false, nil
			}
			// Step 1: Record PendingDeletion checkpoint before any destructive work.
			cp.PendingDeletions = append(cp.PendingDeletions, spectrev1.PendingDeletion{
				Kind:            "ApprovalRequest",
				Name:            ref.Name,
				Namespace:       ref.Namespace,
				UID:             string(ar.UID),
				ResourceVersion: ar.ResourceVersion,
				Phase:           PendingDeletionPhasePrepared,
			})
			cp.LastRetiredUID = string(ar.UID)
			cp.LastRetiredResourceVersion = ar.ResourceVersion
			logger.Info("Recorded PendingDeletion for ApprovalRequest", "name", ref.Name)
			return false, nil // persist checkpoint before deletion
		}

		pd := &cp.PendingDeletions[idx]
		if pd.Phase == PendingDeletionPhasePrepared {
			// Re-verify dual authorization before deleting from a prepared
			// checkpoint. Between prepare and delete, the Listener's approval
			// state might have regressed (e.g., both gates now Pending).
			if dual == nil || dual.outcome != outcomeGranted {
				return false, nil
			}
			// Step 2: UID/RV match confirmed — delete with both preconditions.
			if pd.UID != string(ar.UID) {
				// UID changed since checkpoint — re-evaluate.
				pd.UID = string(ar.UID)
				pd.ResourceVersion = ar.ResourceVersion
				return false, nil // persist updated checkpoint
			}
			if pd.ResourceVersion != ar.ResourceVersion {
				// ResourceVersion changed since checkpoint — re-evaluate.
				logger.Info("ApprovalRequest ResourceVersion changed since checkpoint, re-validating",
					"name", ref.Name, "checkpointRV", pd.ResourceVersion, "currentRV", ar.ResourceVersion)
				pd.ResourceVersion = ar.ResourceVersion
				return false, nil // persist updated checkpoint
			}
			uid := ar.UID
			rv := ar.ResourceVersion
			precond := client.Preconditions(metav1.Preconditions{
				UID:             &uid,
				ResourceVersion: &rv,
			})
			if err := c.Delete(ctx, ar, precond); err != nil && !apierrors.IsNotFound(err) {
				return false, errors.Wrapf(err, "failed to delete legacy ApprovalRequest %q", ref.Name)
			}
			// Don't mark DeleteObserved immediately — verify on next reconcile.
			// The phase-aware recovery (NotFound + DeletePrepared) handles
			// the case where the controller crashes between delete and ack.
			return false, nil
		}
	}

	cp.RequestsRetired = true
	return true, nil
}

// retireLegacyApproval deletes the legacy Approval using the per-resource
// deletion checkpoint pattern. Returns true when the legacy Approval is retired.
func (h *ListenerHandler) retireLegacyApproval(
	ctx context.Context,
	mctx *migrationContext,
	migration *spectrev1.AuthorizationMigrationStatus,
	dual *dualApprovalResult,
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
	cp := migration.RetirementCheckpoint

	idx := findPendingDeletion(cp.PendingDeletions, "Approval", ref.Name, ref.Namespace)

	// Already observed as deleted.
	if idx >= 0 && cp.PendingDeletions[idx].Phase == PendingDeletionPhaseObserved {
		cp.ApprovalRetired = true
		return true, nil
	}

	// Live read: the pre-delete review must see the current instance.
	approval := &approvalapi.Approval{}
	err := h.getLive(ctx, k8stypes.NamespacedName{
		Name:      ref.Name,
		Namespace: ref.Namespace,
	}, approval)
	if err != nil {
		if apierrors.IsNotFound(err) {
			if idx >= 0 && cp.PendingDeletions[idx].Phase == PendingDeletionPhasePrepared {
				// Phase-aware recovery: prepared but gone — delete succeeded before ack.
				cp.PendingDeletions[idx].Phase = PendingDeletionPhaseObserved
				logger.Info("Recovered deletion: Approval gone after DeletePrepared", "name", ref.Name)
			}
			cp.ApprovalRetired = true
			return true, nil
		}
		return false, errors.Wrapf(err, "fresh read of legacy Approval %q", ref.Name)
	}

	// Conditional check: UID must match.
	if ref.UID != "" && approval.UID != ref.UID {
		return false, fmt.Errorf("legacy Approval %q UID mismatch: recorded %s, found %s — aborting retirement",
			ref.Name, ref.UID, approval.UID)
	}

	// Re-check restrictive state before deletion. Between the initial migration
	// check and this retirement step the Approval may have been Rejected or
	// Suspended. Deleting a now-restrictive Approval would silently remove the
	// block signal — instead, block migration so the operator can investigate.
	if blocked, reason := isLegacyBlocked(approval); blocked {
		migration.Phase = MigrationPhaseBlocked
		mctx.listener.SetCondition(condition.NewNotReadyCondition("LegacyApprovalMigrationBlocked", reason))
		mctx.listener.SetCondition(condition.NewBlockedCondition(reason))
		logger.Info("Retirement blocked: Approval became restrictive", "reason", reason)
		return false, nil
	}

	if idx < 0 {
		// About to issue a NEW deletion — verify the dual-gate result is
		// still Granted before deleting the legacy Approval.
		if dual == nil || dual.outcome != outcomeGranted {
			return false, nil
		}
		// Step 1: Record PendingDeletion checkpoint before deletion.
		cp.PendingDeletions = append(cp.PendingDeletions, spectrev1.PendingDeletion{
			Kind:            "Approval",
			Name:            ref.Name,
			Namespace:       ref.Namespace,
			UID:             string(approval.UID),
			ResourceVersion: approval.ResourceVersion,
			Phase:           PendingDeletionPhasePrepared,
		})
		cp.LastRetiredUID = string(approval.UID)
		cp.LastRetiredResourceVersion = approval.ResourceVersion
		logger.Info("Recorded PendingDeletion for Approval", "name", ref.Name)
		return false, nil // persist checkpoint before deletion
	}

	pd := &cp.PendingDeletions[idx]
	if pd.Phase == PendingDeletionPhasePrepared {
		// Re-verify dual authorization before deleting from a prepared
		// checkpoint. Between prepare and delete, the Listener's approval
		// state might have regressed.
		if dual == nil || dual.outcome != outcomeGranted {
			return false, nil
		}
		// Step 2: UID match confirmed — check ResourceVersion for concurrent changes.
		if pd.UID != string(approval.UID) {
			pd.UID = string(approval.UID)
			pd.ResourceVersion = approval.ResourceVersion
			return false, nil // persist updated checkpoint
		}
		if pd.ResourceVersion != approval.ResourceVersion {
			// The Approval was modified since we recorded the checkpoint. Update
			// the checkpoint and re-validate on the next reconcile (the re-check
			// above will catch any new restrictive state).
			logger.Info("Approval ResourceVersion changed since checkpoint, re-validating",
				"name", ref.Name, "checkpointRV", pd.ResourceVersion, "currentRV", approval.ResourceVersion)
			pd.ResourceVersion = approval.ResourceVersion
			return false, nil // persist updated checkpoint
		}
		// Delete with both UID and ResourceVersion preconditions to guard
		// against concurrent modifications between the Get and the Delete.
		uid := approval.UID
		rv := approval.ResourceVersion
		precond := client.Preconditions(metav1.Preconditions{
			UID:             &uid,
			ResourceVersion: &rv,
		})
		if err := c.Delete(ctx, approval, precond); err != nil && !apierrors.IsNotFound(err) {
			return false, errors.Wrapf(err, "failed to delete legacy Approval %q", ref.Name)
		}
		// Don't mark DeleteObserved immediately — verify on next reconcile.
		return false, nil
	}

	cp.ApprovalRetired = true
	return true, nil
}

// dualOutcomeString safely returns the string representation of a dual result.
func dualOutcomeString(dual *dualApprovalResult) string {
	if dual == nil {
		return "nil"
	}
	return dual.outcome.String()
}
