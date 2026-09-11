// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package runtime

import (
	"context"
	"fmt"
	"math/rand/v2"
	"time"

	"entgo.io/ent/privacy"
	"github.com/go-logr/logr"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/telekom/controlplane/projector/internal/metrics"
)

// ReadOnlyReconciler is a minimal reconciler that only reads from Kubernetes
// and delegates all database operations to a SyncProcessor. It never writes
// back to the Kubernetes API server (no finalizers, no status updates, no
// conditions).
//
// Cross-cutting concerns handled here:
//   - privacy.DecisionContext wrapping (centralized, not per-handler)
//   - Error classification and requeue policy
//   - Delete cache lookup
type ReadOnlyReconciler[T client.Object] struct {
	client      client.Reader
	processor   SyncProcessor[T]
	deleteCache DeleteCacheReader
	policy      ErrorPolicy
	newObj      func() T
	moduleName  string

	// randFloat64 returns a random float64 in [0.0, 1.0). Exposed for
	// deterministic testing; defaults to rand.Float64.
	randFloat64 func() float64
}

// DeleteCacheReader is the subset of DeleteCache that the reconciler needs.
// This avoids importing the infrastructure package directly.
type DeleteCacheReader interface {
	LoadAndDelete(key client.ObjectKey) client.Object
}

// NewReadOnlyReconciler creates a ReadOnlyReconciler wired to the given
// SyncProcessor and DeleteCache. The ErrorPolicy controls requeue behavior
// for each error class and periodic resync.
func NewReadOnlyReconciler[T client.Object](
	reader client.Reader,
	processor SyncProcessor[T],
	deleteCache DeleteCacheReader,
	moduleName string,
	newObj func() T,
	policy ErrorPolicy,
) *ReadOnlyReconciler[T] {
	return &ReadOnlyReconciler[T]{
		client:      reader,
		processor:   processor,
		deleteCache: deleteCache,
		policy:      policy,
		newObj:      newObj,
		moduleName:  moduleName,
		randFloat64: rand.Float64,
	}
}

// Reconcile reads the CR from Kubernetes and delegates to the SyncProcessor.
//   - Object exists -> processor.Upsert()
//   - Object NotFound -> processor.Delete() with delete-cache lookup
//   - ErrSkipSync -> log and requeue at SkipRequeue interval
//   - ErrDependencyMissing -> requeue with DependencyDelay + jitter
//   - ErrDeleteKeyLost -> log warning, do not requeue
func (r *ReadOnlyReconciler[T]) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	start := time.Now()

	// Centralized privacy context — every DB operation inherits this.
	ctx = privacy.DecisionContext(ctx, privacy.Allow)

	result, outcome, err := r.reconcileInner(ctx, req, logger)

	duration := time.Since(start).Seconds()
	metrics.ReconcileTotal.WithLabelValues(r.moduleName, outcome).Inc()
	metrics.ReconcileDuration.WithLabelValues(r.moduleName, outcome).Observe(duration)

	return result, err
}

// reconcileInner performs the actual reconciliation logic and returns the
// result, classified outcome label, and error.
func (r *ReadOnlyReconciler[T]) reconcileInner(ctx context.Context, req ctrl.Request, logger logr.Logger) (ctrl.Result, string, error) {
	obj := r.newObj()

	err := r.client.Get(ctx, req.NamespacedName, obj)

	if apierrors.IsNotFound(err) {
		logger.V(1).Info("object not found, treating as delete")
		lastKnown := r.deleteCache.LoadAndDelete(req.NamespacedName)
		var typed T
		if lastKnown != nil {
			var ok bool
			typed, ok = lastKnown.(T)
			if !ok {
				logger.Error(nil, "delete cache type assertion failed",
					"key", req.NamespacedName,
					"expectedType", fmt.Sprintf("%T", r.newObj()),
					"actualType", fmt.Sprintf("%T", lastKnown),
				)
				// Fall through with zero-value typed — processor uses convention-based derivation.
			}
		}
		if delErr := r.processor.Delete(ctx, req.NamespacedName, typed); delErr != nil {
			if IsDeleteKeyLost(delErr) {
				logger.Info("delete key lost, cannot remove from database",
					"key", req.NamespacedName,
					"reason", delErr.Error())
				return ctrl.Result{}, metrics.OutcomeDeleteKeyLost, nil
			}

			logger.Error(delErr, "delete failed")

			return ctrl.Result{}, metrics.OutcomeError, delErr
		}

		logger.V(1).Info("delete successful")
		return ctrl.Result{}, metrics.OutcomeDeleteSuccess, nil
	}
	if err != nil {
		return ctrl.Result{}, metrics.OutcomeError, err
	}

	if upsertErr := r.processor.Upsert(ctx, obj); upsertErr != nil {
		if IsSkipSync(upsertErr) {
			logger.V(1).Info("CR lacks required data, skipping sync",
				"key", req.NamespacedName,
				"reason", upsertErr.Error())
			return ctrl.Result{RequeueAfter: r.policy.SkipRequeue}, metrics.OutcomeSkip, nil
		}
		if IsDependencyMissing(upsertErr) {
			logger.V(1).Info("dependency not yet synced, requeuing",
				"key", req.NamespacedName,
				"reason", upsertErr.Error())
			return ctrl.Result{RequeueAfter: r.dependencyRequeue()}, metrics.OutcomeDependencyMissing, nil
		}

		logger.Error(upsertErr, "upsert failed")

		return ctrl.Result{}, metrics.OutcomeError, upsertErr
	}

	logger.V(1).Info("reconcile successful")
	return r.successResult(), metrics.OutcomeSuccess, nil
}

// dependencyRequeue returns DependencyDelay plus a random jitter in
// [0, DependencyJitter). When DependencyJitter is 0, no jitter is applied.
func (r *ReadOnlyReconciler[T]) dependencyRequeue() time.Duration {
	d := r.policy.DependencyDelay
	if r.policy.DependencyJitter > 0 {
		d += time.Duration(r.randFloat64() * float64(r.policy.DependencyJitter))
	}
	return d
}

// successResult returns the ctrl.Result for a successful reconciliation.
// When PeriodicResync is 0, returns an empty result (event-driven only).
// When non-zero, adds ±20% jitter to spread resync load.
func (r *ReadOnlyReconciler[T]) successResult() ctrl.Result {
	resync := r.policy.PeriodicResync
	if resync == 0 {
		return ctrl.Result{}
	}
	// ±20% jitter: multiply by [0.8, 1.2)
	jitter := 0.8 + r.randFloat64()*0.4
	return ctrl.Result{RequeueAfter: time.Duration(float64(resync) * jitter)}
}

// SetRandFloat64 replaces the random number generator used for jitter
// calculations. This is intended for deterministic testing only.
func (r *ReadOnlyReconciler[T]) SetRandFloat64(fn func() float64) {
	r.randFloat64 = fn
}
