// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package framework

import (
	"context"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

// GenericMigrationReconciler is a generic reconciler for any resource type
type GenericMigrationReconciler struct {
	client.Client
	Scheme       *runtime.Scheme
	RemoteClient client.Client
	Migrator     ResourceMigrator
	Log          logr.Logger
}

// Reconcile handles the reconciliation of resources for migration
func (r *GenericMigrationReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	migratorName := r.Migrator.GetName()
	log.Info("Reconciling resource for migration",
		"migrator", migratorName,
		"name", req.Name,
		"namespace", req.Namespace)

	// Fetch the resource
	obj := r.Migrator.GetNewResourceType()
	if err := r.Get(ctx, req.NamespacedName, obj); err != nil {
		log.V(1).Info("Resource not found, likely deleted")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Compute legacy identifier
	legacyNamespace, legacyName, skip, err := r.Migrator.ComputeLegacyIdentifier(ctx, obj)
	if err != nil {
		log.Error(err, "Failed to compute legacy identifier")
		return ctrl.Result{RequeueAfter: r.Migrator.GetRequeueAfter()}, err
	}

	if skip {
		log.Info("Skipping migration for this resource",
			"reason", "ComputeLegacyIdentifier returned skip=true")
		return ctrl.Result{RequeueAfter: r.Migrator.GetRequeueAfter()}, nil
	}

	log.Info("Computed legacy identifier",
		"legacyNamespace", legacyNamespace,
		"legacyName", legacyName)

	// Fetch from legacy cluster
	log.Info("Fetching from legacy cluster",
		"namespace", legacyNamespace,
		"name", legacyName)
	legacyObj, err := r.Migrator.FetchFromLegacy(ctx, r.RemoteClient, legacyNamespace, legacyName)
	if err != nil {
		if client.IgnoreNotFound(err) == nil {
			log.Info("Legacy resource not found, skipping migration",
				"legacyNamespace", legacyNamespace,
				"legacyName", legacyName)
			return ctrl.Result{RequeueAfter: r.Migrator.GetRequeueAfter()}, nil
		}
		log.Error(err, "Failed to fetch from legacy cluster")
		return ctrl.Result{RequeueAfter: r.Migrator.GetRequeueAfter()}, err
	}

	log.Info("Fetched legacy resource successfully")

	// Check if migration is needed
	if !r.Migrator.HasChanged(ctx, obj, legacyObj) {
		log.Info("No changes detected, skipping update")
		return ctrl.Result{RequeueAfter: r.Migrator.GetRequeueAfter()}, nil
	}

	log.Info("Changes detected, applying migration")

	// Apply migration
	if err := r.Migrator.ApplyMigration(ctx, obj, legacyObj); err != nil {
		log.Error(err, "Failed to apply migration")
		return ctrl.Result{RequeueAfter: r.Migrator.GetRequeueAfter()}, err
	}

	// Update the resource
	if err := r.Update(ctx, obj); err != nil {
		log.Error(err, "Failed to update resource")
		return ctrl.Result{RequeueAfter: r.Migrator.GetRequeueAfter()}, err
	}

	log.Info("Successfully migrated resource")
	return ctrl.Result{RequeueAfter: r.Migrator.GetRequeueAfter()}, nil
}

// SetupWithManager sets up the controller with the Manager
func (r *GenericMigrationReconciler) SetupWithManager(mgr ctrl.Manager) error {
	resourceType := r.Migrator.GetNewResourceType()

	return ctrl.NewControllerManagedBy(mgr).
		For(resourceType).
		WithEventFilter(predicate.GenerationChangedPredicate{}).
		Complete(r)
}
