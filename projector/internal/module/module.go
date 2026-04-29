// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

// Package module provides the type-erased registration boundary for resource
// modules. Each domain package exports a single Module variable that the
// bootstrap wires into the controller-runtime manager.
package module

import (
	"github.com/telekom/controlplane/controlplane-api/ent"
	"github.com/telekom/controlplane/projector/internal/config"
	"github.com/telekom/controlplane/projector/internal/infrastructure"
	"github.com/telekom/controlplane/projector/internal/runtime"
	"golang.org/x/time/rate"
	"k8s.io/client-go/util/workqueue"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// Module is the type-erased registration boundary for a resource module.
// Go does not allow []Module[T, D, K] where type parameters vary per element,
// so this interface provides type erasure at the registration boundary while
// keeping full type safety inside each module.
type Module interface {
	Name() string
	Register(mgr ctrl.Manager, deps ModuleDeps) error
}

// ModuleDeps carries shared infrastructure injected into every module at
// registration. All fields are created once in the bootstrap and shared
// across all modules.
type ModuleDeps struct {
	DeleteCache *infrastructure.DeleteCache
	EntClient   *ent.Client
	EdgeCache   *infrastructure.EdgeCache
	IDResolver  *infrastructure.IDResolver
	Config      *config.Config
}

// TypedModule is the generic module implementation. Type parameters are
// captured inside the struct; the Module interface erases them.
//
// Fields are exported to allow struct literal construction from domain packages.
type TypedModule[T client.Object, D any, K any] struct {
	// ModuleName is the controller name (used in logs and metrics).
	ModuleName string

	// NewObj returns a new zero-value typed object for the reconciler.
	NewObj func() T

	// Translator maps K8s objects to domain payloads and identity keys.
	Translator runtime.Translator[T, D, K]

	// RepoFactory creates the repository, wired with shared infrastructure.
	RepoFactory func(deps ModuleDeps) runtime.Repository[K, D]
}

// Name returns the module name.
func (m *TypedModule[T, D, K]) Name() string { return m.ModuleName }

// Register creates the full processing pipeline and wires it into the
// controller-runtime manager:
//  1. Creates the repository via RepoFactory
//  2. Builds the generic Processor
//  3. Builds the ReadOnlyReconciler with ErrorPolicy from config
//  4. Registers a named controller with Watches, RateLimiter, and concurrency
func (m *TypedModule[T, D, K]) Register(mgr ctrl.Manager, deps ModuleDeps) error {
	cfg := deps.Config

	policy := runtime.NewErrorPolicyFromConfig(cfg)

	repo := m.RepoFactory(deps)
	proc := runtime.NewProcessor(m.Translator, repo)
	rec := runtime.NewReadOnlyReconciler(
		mgr.GetClient(),
		proc,
		deps.DeleteCache,
		m.ModuleName,
		m.NewObj,
		policy,
	)

	ctrlOpts := controller.Options{
		MaxConcurrentReconciles: cfg.ConcurrencyFor(m.ModuleName),
		RateLimiter:             newRateLimiter(cfg),
		ReconciliationTimeout:   cfg.ReconcileTimeout,
	}

	return ctrl.NewControllerManagedBy(mgr).
		Named("projector-"+m.ModuleName).
		Watches(m.NewObj(),
			infrastructure.NewSyncEventHandler(deps.DeleteCache),
			builder.WithPredicates(predicate.ResourceVersionChangedPredicate{}),
		).
		WithOptions(ctrlOpts).
		Complete(rec)
}

// newRateLimiter constructs a composite rate limiter from config:
//   - Per-item exponential backoff (for errored reconciles)
//   - Global token bucket (for steady-state throughput)
func newRateLimiter(cfg *config.Config) workqueue.TypedRateLimiter[reconcile.Request] {
	return workqueue.NewTypedMaxOfRateLimiter(
		workqueue.NewTypedItemExponentialFailureRateLimiter[reconcile.Request](
			cfg.RateLimiterBaseDelay,
			cfg.RateLimiterMaxDelay,
		),
		&workqueue.TypedBucketRateLimiter[reconcile.Request]{
			Limiter: rate.NewLimiter(rate.Limit(cfg.RateLimiterQPS), cfg.RateLimiterBurst),
		},
	)
}
