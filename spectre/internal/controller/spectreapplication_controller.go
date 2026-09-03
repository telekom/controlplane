// SPDX-FileCopyrightText: 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

//nolint:dupl // controller boilerplate is structurally identical by design
package controller

import (
	"context"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	crhandler "sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	applicationv1 "github.com/telekom/controlplane/application/api/v1"
	cconfig "github.com/telekom/controlplane/common/pkg/config"
	cc "github.com/telekom/controlplane/common/pkg/controller"
	eventv1 "github.com/telekom/controlplane/event/api/v1"
	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
	pubsubv1 "github.com/telekom/controlplane/pubsub/api/v1"
	spectrev1 "github.com/telekom/controlplane/spectre/api/v1"
	"github.com/telekom/controlplane/spectre/internal/handler"
)

// SpectreApplicationReconciler reconciles a SpectreApplication object
type SpectreApplicationReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder

	cc.Controller[*spectrev1.SpectreApplication]
}

// +kubebuilder:rbac:groups=spectre.cp.ei.telekom.de,resources=spectreapplications,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=spectre.cp.ei.telekom.de,resources=spectreapplications/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=spectre.cp.ei.telekom.de,resources=spectreapplications/finalizers,verbs=update

func (r *SpectreApplicationReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	return r.Controller.Reconcile(ctx, req, &spectrev1.SpectreApplication{})
}

// SetupWithManager sets up the controller with the Manager.
func (r *SpectreApplicationReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Recorder = mgr.GetEventRecorderFor("spectreapplication-controller")
	r.Controller = cc.NewController(&handler.SpectreApplicationHandler{}, r.Client, r.Recorder)

	return ctrl.NewControllerManagedBy(mgr).
		For(&spectrev1.SpectreApplication{}).
		// Owner-labelled Publishers (readiness changes update parent readiness).
		Watches(
			&pubsubv1.Publisher{},
			crhandler.EnqueueRequestsFromMapFunc(r.mapOwnedChildToSpectreApplication),
			builder.WithPredicates(cc.Count("spectreapplication", cc.RoleWatches, predicate.ResourceVersionChangedPredicate{})),
		).
		// Owner-labelled Subscribers (readiness changes update parent readiness).
		Watches(
			&pubsubv1.Subscriber{},
			crhandler.EnqueueRequestsFromMapFunc(r.mapOwnedChildToSpectreApplication),
			builder.WithPredicates(cc.Count("spectreapplication", cc.RoleWatches, predicate.ResourceVersionChangedPredicate{})),
		).
		// Owner-labelled Routes (readiness changes update parent readiness).
		Watches(
			&gatewayv1.Route{},
			crhandler.EnqueueRequestsFromMapFunc(r.mapOwnedChildToSpectreApplication),
			builder.WithPredicates(cc.Count("spectreapplication", cc.RoleWatches, predicate.ResourceVersionChangedPredicate{})),
		).
		// Referenced Application changes.
		Watches(
			&applicationv1.Application{},
			crhandler.EnqueueRequestsFromMapFunc(r.mapApplicationToSpectreApplications),
			builder.WithPredicates(cc.Count("spectreapplication", cc.RoleWatches, predicate.ResourceVersionChangedPredicate{})),
		).
		// Zone changes via the Application dependency chain.
		Watches(
			&adminv1.Zone{},
			crhandler.EnqueueRequestsFromMapFunc(r.mapZoneToSpectreApplications),
			builder.WithPredicates(cc.Count("spectreapplication", cc.RoleWatches, predicate.ResourceVersionChangedPredicate{})),
		).
		// EventConfig changes via the zone dependency chain.
		Watches(
			&eventv1.EventConfig{},
			crhandler.EnqueueRequestsFromMapFunc(r.mapEventConfigToSpectreApplications),
			builder.WithPredicates(cc.Count("spectreapplication", cc.RoleWatches, predicate.ResourceVersionChangedPredicate{})),
		).
		// EventStore readiness changes.
		Watches(
			&pubsubv1.EventStore{},
			crhandler.EnqueueRequestsFromMapFunc(r.mapEventStoreToSpectreApplications),
			builder.WithPredicates(cc.Count("spectreapplication", cc.RoleWatches, predicate.ResourceVersionChangedPredicate{})),
		).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: cconfig.MaxConcurrentReconciles,
			RateLimiter:             cc.NewRateLimiter(),
		}).
		Named("spectreapplication").
		Complete(r)
}

// mapOwnedChildToSpectreApplication maps an owner-labelled child (Publisher,
// Subscriber, or Route) back to the owning SpectreApplication via OwnerUidLabelKey.
// Uses a UID field index instead of a cluster-wide parent scan.
func (r *SpectreApplicationReconciler) mapOwnedChildToSpectreApplication(
	ctx context.Context,
	obj client.Object,
) []reconcile.Request {
	logger := log.FromContext(ctx)
	labels := obj.GetLabels()
	if labels == nil {
		return nil
	}

	ownerUID := labels[cconfig.OwnerUidLabelKey]
	if ownerUID == "" {
		return nil
	}

	list := &spectrev1.SpectreApplicationList{}
	if err := r.List(ctx, list, client.MatchingFields{UidIndexKey: ownerUID}); err != nil {
		logger.Error(err, "Failed to list SpectreApplications for owned child")
		return nil
	}

	var reqs []reconcile.Request
	for i := range list.Items {
		reqs = append(reqs, reconcile.Request{
			NamespacedName: client.ObjectKeyFromObject(&list.Items[i]),
		})
	}

	return reqs
}

// mapApplicationToSpectreApplications maps an Application change to
// SpectreApplications that reference it.
func (r *SpectreApplicationReconciler) mapApplicationToSpectreApplications(
	ctx context.Context,
	obj client.Object,
) []reconcile.Request {
	logger := log.FromContext(ctx)
	app, ok := obj.(*applicationv1.Application)
	if !ok {
		return nil
	}

	list := &spectrev1.SpectreApplicationList{}
	if err := r.List(ctx, list, client.MatchingLabels{
		cconfig.EnvironmentLabelKey: app.Labels[cconfig.EnvironmentLabelKey],
	}); err != nil {
		logger.Error(err, "Failed to list SpectreApplications for Application")
		return nil
	}

	appRef := types.NamespacedName{Name: app.Name, Namespace: app.Namespace}
	var reqs []reconcile.Request
	for i := range list.Items {
		sa := &list.Items[i]
		if sa.Spec.Application.Name == appRef.Name && sa.Spec.Application.Namespace == appRef.Namespace {
			reqs = append(reqs, reconcile.Request{
				NamespacedName: client.ObjectKeyFromObject(sa),
			})
		}
	}

	return reqs
}

// mapZoneToSpectreApplications maps a Zone change to SpectreApplications
// whose referenced Application is in that Zone.
func (r *SpectreApplicationReconciler) mapZoneToSpectreApplications(
	ctx context.Context,
	obj client.Object,
) []reconcile.Request {
	logger := log.FromContext(ctx)
	zone, ok := obj.(*adminv1.Zone)
	if !ok {
		return nil
	}

	// Find Applications in this Zone.
	appList := &applicationv1.ApplicationList{}
	if err := r.List(ctx, appList, client.MatchingLabels{
		cconfig.EnvironmentLabelKey: zone.Labels[cconfig.EnvironmentLabelKey],
	}); err != nil {
		logger.Error(err, "Failed to list Applications for Zone")
		return nil
	}

	zoneRef := types.NamespacedName{Name: zone.Name, Namespace: zone.Namespace}
	appRefs := make(map[types.NamespacedName]struct{})
	for i := range appList.Items {
		a := &appList.Items[i]
		if a.Spec.Zone.Name == zoneRef.Name && a.Spec.Zone.Namespace == zoneRef.Namespace {
			appRefs[types.NamespacedName{Name: a.Name, Namespace: a.Namespace}] = struct{}{}
		}
	}

	if len(appRefs) == 0 {
		return nil
	}

	saList := &spectrev1.SpectreApplicationList{}
	if err := r.List(ctx, saList, client.MatchingLabels{
		cconfig.EnvironmentLabelKey: zone.Labels[cconfig.EnvironmentLabelKey],
	}); err != nil {
		logger.Error(err, "Failed to list SpectreApplications for Zone")
		return nil
	}

	var reqs []reconcile.Request
	for i := range saList.Items {
		sa := &saList.Items[i]
		ref := types.NamespacedName{Name: sa.Spec.Application.Name, Namespace: sa.Spec.Application.Namespace}
		if _, ok := appRefs[ref]; ok {
			reqs = append(reqs, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(sa)})
		}
	}

	return reqs
}

// mapEventConfigToSpectreApplications maps an EventConfig change to
// SpectreApplications in the same zone.
func (r *SpectreApplicationReconciler) mapEventConfigToSpectreApplications(
	ctx context.Context,
	obj client.Object,
) []reconcile.Request {
	logger := log.FromContext(ctx)
	ec, ok := obj.(*eventv1.EventConfig)
	if !ok {
		return nil
	}

	appList := &applicationv1.ApplicationList{}
	if err := r.List(ctx, appList, client.MatchingLabels{
		cconfig.EnvironmentLabelKey: ec.Labels[cconfig.EnvironmentLabelKey],
	}); err != nil {
		logger.Error(err, "Failed to list Applications for EventConfig")
		return nil
	}

	zoneRef := types.NamespacedName{Name: ec.Spec.Zone.Name, Namespace: ec.Spec.Zone.Namespace}
	appRefs := make(map[types.NamespacedName]struct{})
	for i := range appList.Items {
		a := &appList.Items[i]
		if a.Spec.Zone.Name == zoneRef.Name && a.Spec.Zone.Namespace == zoneRef.Namespace {
			appRefs[types.NamespacedName{Name: a.Name, Namespace: a.Namespace}] = struct{}{}
		}
	}

	if len(appRefs) == 0 {
		return nil
	}

	saList := &spectrev1.SpectreApplicationList{}
	if err := r.List(ctx, saList, client.MatchingLabels{
		cconfig.EnvironmentLabelKey: ec.Labels[cconfig.EnvironmentLabelKey],
	}); err != nil {
		logger.Error(err, "Failed to list SpectreApplications for EventConfig")
		return nil
	}

	var reqs []reconcile.Request
	for i := range saList.Items {
		sa := &saList.Items[i]
		ref := types.NamespacedName{Name: sa.Spec.Application.Name, Namespace: sa.Spec.Application.Namespace}
		if _, ok := appRefs[ref]; ok {
			reqs = append(reqs, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(sa)})
		}
	}

	return reqs
}

// mapEventStoreToSpectreApplications maps an EventStore change to
// SpectreApplications that depend on it via EventConfig.
func (r *SpectreApplicationReconciler) mapEventStoreToSpectreApplications(
	ctx context.Context,
	obj client.Object,
) []reconcile.Request {
	logger := log.FromContext(ctx)
	es, ok := obj.(*pubsubv1.EventStore)
	if !ok {
		return nil
	}

	ecList := &eventv1.EventConfigList{}
	if err := r.List(ctx, ecList, client.MatchingLabels{
		cconfig.EnvironmentLabelKey: es.Labels[cconfig.EnvironmentLabelKey],
	}); err != nil {
		logger.Error(err, "Failed to list EventConfigs for EventStore")
		return nil
	}

	var reqs []reconcile.Request
	for i := range ecList.Items {
		ec := &ecList.Items[i]
		if ec.Status.EventStore != nil && ec.Status.EventStore.Name == es.Name && ec.Status.EventStore.Namespace == es.Namespace {
			reqs = append(reqs, r.mapEventConfigToSpectreApplications(ctx, ec)...)
		}
	}

	return reqs
}
