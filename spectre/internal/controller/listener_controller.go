// SPDX-FileCopyrightText: 2025 Deutsche Telekom IT GmbH
// Copyright 2026.
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
	approvalv1 "github.com/telekom/controlplane/approval/api/v1"
	cconfig "github.com/telekom/controlplane/common/pkg/config"
	cc "github.com/telekom/controlplane/common/pkg/controller"
	"github.com/telekom/controlplane/common/pkg/util/labelutil"
	eventv1 "github.com/telekom/controlplane/event/api/v1"
	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
	pubsubv1 "github.com/telekom/controlplane/pubsub/api/v1"
	spectrev1 "github.com/telekom/controlplane/spectre/api/v1"
	"github.com/telekom/controlplane/spectre/internal/handler"
	"github.com/telekom/controlplane/spectre/internal/handler/util"
)

// ListenerReconciler reconciles a Listener object
type ListenerReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder

	cc.Controller[*spectrev1.Listener]
}

// +kubebuilder:rbac:groups=spectre.cp.ei.telekom.de,resources=listeners,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=spectre.cp.ei.telekom.de,resources=listeners/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=spectre.cp.ei.telekom.de,resources=listeners/finalizers,verbs=update
// +kubebuilder:rbac:groups=core,resources=events,verbs=create;patch
// +kubebuilder:rbac:groups=pubsub.cp.ei.telekom.de,resources=publishers;subscribers;eventstores,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=gateway.cp.ei.telekom.de,resources=routelisteners;routes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=approval.cp.ei.telekom.de,resources=approvalrequests;approvals,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=application.cp.ei.telekom.de,resources=applications,verbs=get;list;watch
// +kubebuilder:rbac:groups=admin.cp.ei.telekom.de,resources=zones,verbs=get;list;watch
// +kubebuilder:rbac:groups=identity.cp.ei.telekom.de,resources=realms,verbs=get;list;watch
// +kubebuilder:rbac:groups=event.cp.ei.telekom.de,resources=eventconfigs,verbs=get;list;watch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Listener object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.24.1/pkg/reconcile
func (r *ListenerReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	return r.Controller.Reconcile(ctx, req, &spectrev1.Listener{})
}

// SetupWithManager sets up the controller with the Manager.
func (r *ListenerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Recorder = mgr.GetEventRecorderFor("listener-controller")
	r.Controller = cc.NewController(&handler.ListenerHandler{}, r.Client, r.Recorder)

	owns := builder.WithPredicates(cc.Count("listener", cc.RoleOwns))

	return ctrl.NewControllerManagedBy(mgr).
		For(&spectrev1.Listener{}).
		// Approval children — same pattern as ApiSubscription and EventSubscription.
		Owns(&approvalv1.ApprovalRequest{}, owns).
		Owns(&approvalv1.Approval{}, owns).
		// SpectreApplication status changes (e.g. Id becomes populated).
		Watches(
			&spectrev1.SpectreApplication{},
			crhandler.EnqueueRequestsFromMapFunc(r.mapSpectreApplicationToListeners),
			builder.WithPredicates(cc.Count("listener", cc.RoleWatches, predicate.ResourceVersionChangedPredicate{})),
		).
		// Owner-labelled RouteListeners (readiness changes update parent readiness).
		Watches(
			&gatewayv1.RouteListener{},
			crhandler.EnqueueRequestsFromMapFunc(r.mapOwnedChildToListener),
			builder.WithPredicates(cc.Count("listener", cc.RoleWatches, predicate.ResourceVersionChangedPredicate{})),
		).
		// Owner-labelled bridge Subscribers (readiness changes update parent readiness).
		Watches(
			&pubsubv1.Subscriber{},
			crhandler.EnqueueRequestsFromMapFunc(r.mapOwnedChildToListener),
			builder.WithPredicates(cc.Count("listener", cc.RoleWatches, predicate.ResourceVersionChangedPredicate{})),
		).
		// Target Routes — creation after a Listener block, or mode changes (pass-through/failover).
		Watches(
			&gatewayv1.Route{},
			crhandler.EnqueueRequestsFromMapFunc(r.mapRouteToListeners),
			builder.WithPredicates(cc.Count("listener", cc.RoleWatches, predicate.ResourceVersionChangedPredicate{})),
		).
		// Consumer and provider Applications.
		Watches(
			&applicationv1.Application{},
			crhandler.EnqueueRequestsFromMapFunc(r.mapApplicationToListeners),
			builder.WithPredicates(cc.Count("listener", cc.RoleWatches, predicate.ResourceVersionChangedPredicate{})),
		).
		// Zones — readiness changes affect blocked Listeners.
		Watches(
			&adminv1.Zone{},
			crhandler.EnqueueRequestsFromMapFunc(r.mapZoneToListeners),
			builder.WithPredicates(cc.Count("listener", cc.RoleWatches, predicate.ResourceVersionChangedPredicate{})),
		).
		// EventConfigs — readiness or CallbackURL changes.
		Watches(
			&eventv1.EventConfig{},
			crhandler.EnqueueRequestsFromMapFunc(r.mapEventConfigToListeners),
			builder.WithPredicates(cc.Count("listener", cc.RoleWatches, predicate.ResourceVersionChangedPredicate{})),
		).
		// EventStores — readiness changes unblock parents.
		Watches(
			&pubsubv1.EventStore{},
			crhandler.EnqueueRequestsFromMapFunc(r.mapEventStoreToListeners),
			builder.WithPredicates(cc.Count("listener", cc.RoleWatches, predicate.ResourceVersionChangedPredicate{})),
		).
		// Shared generic Publisher — readiness changes unblock Listeners waiting for it.
		Watches(
			&pubsubv1.Publisher{},
			crhandler.EnqueueRequestsFromMapFunc(r.mapGenericPublisherToListeners),
			builder.WithPredicates(cc.Count("listener", cc.RoleWatches, predicate.ResourceVersionChangedPredicate{})),
		).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: cconfig.MaxConcurrentReconciles,
			RateLimiter:             cc.NewRateLimiter(),
		}).
		Named("listener").
		Complete(r)
}

// mapSpectreApplicationToListeners maps a SpectreApplication change to all
// Listeners that reference it via spec.application.
func (r *ListenerReconciler) mapSpectreApplicationToListeners(
	ctx context.Context,
	obj client.Object,
) []reconcile.Request {
	app, ok := obj.(*spectrev1.SpectreApplication)
	if !ok {
		return nil
	}

	logger := log.FromContext(ctx)

	list := &spectrev1.ListenerList{}
	if err := r.List(ctx, list, client.MatchingLabels{
		cconfig.EnvironmentLabelKey: app.Labels[cconfig.EnvironmentLabelKey],
	}); err != nil {
		logger.Error(err, "Failed to list Listeners for SpectreApplication")
		return nil
	}

	var reqs []reconcile.Request
	for i := range list.Items {
		if list.Items[i].Spec.Application.Name != app.Name ||
			list.Items[i].Spec.Application.Namespace != app.Namespace {
			continue
		}
		reqs = append(reqs, reconcile.Request{
			NamespacedName: client.ObjectKeyFromObject(&list.Items[i]),
		})
	}

	return reqs
}

// mapOwnedChildToListener maps an owner-labelled child (RouteListener or
// Subscriber) back to the owning Listener via the OwnerUidLabelKey.
// Uses a UID field index instead of a cluster-wide parent scan.
func (r *ListenerReconciler) mapOwnedChildToListener(
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

	list := &spectrev1.ListenerList{}
	if err := r.List(ctx, list, client.MatchingFields{UidIndexKey: ownerUID}); err != nil {
		logger.Error(err, "Failed to list Listeners for owned child")
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

// mapRouteToListeners maps a Route change to Listeners whose apiBasePath
// matches the Route. This ensures a Listener blocked before Route creation
// or whose Route changes mode reconciles immediately.
func (r *ListenerReconciler) mapRouteToListeners(
	ctx context.Context,
	obj client.Object,
) []reconcile.Request {
	route, ok := obj.(*gatewayv1.Route)
	if !ok {
		return nil
	}

	// Extract the apiBasePath from the Route name. The Route name is derived
	// deterministically from the apiBasePath via labelutil.NormalizeValue.
	// We need to find Listeners whose apiBasePath would produce this Route name.
	list := &spectrev1.ListenerList{}
	if err := r.List(ctx, list, client.MatchingLabels{
		cconfig.EnvironmentLabelKey: route.Labels[cconfig.EnvironmentLabelKey],
	}); err != nil {
		return nil
	}

	routeName := route.Name
	var reqs []reconcile.Request
	for i := range list.Items {
		if list.Items[i].Spec.ApiListener == nil {
			continue
		}
		if labelutil.NormalizeValue(list.Items[i].Spec.ApiListener.ApiBasePath) == routeName {
			reqs = append(reqs, reconcile.Request{
				NamespacedName: client.ObjectKeyFromObject(&list.Items[i]),
			})
		}
	}

	return reqs
}

// mapApplicationToListeners maps an Application change to Listeners that
// reference it as consumer or provider.
func (r *ListenerReconciler) mapApplicationToListeners(
	ctx context.Context,
	obj client.Object,
) []reconcile.Request {
	logger := log.FromContext(ctx)
	app, ok := obj.(*applicationv1.Application)
	if !ok {
		return nil
	}

	list := &spectrev1.ListenerList{}
	if err := r.List(ctx, list, client.MatchingLabels{
		cconfig.EnvironmentLabelKey: app.Labels[cconfig.EnvironmentLabelKey],
	}); err != nil {
		logger.Error(err, "Failed to list Listeners for Application")
		return nil
	}

	appRef := types.NamespacedName{Name: app.Name, Namespace: app.Namespace}
	var reqs []reconcile.Request
	for i := range list.Items {
		l := &list.Items[i]
		if (l.Spec.Consumer.Name == appRef.Name && l.Spec.Consumer.Namespace == appRef.Namespace) ||
			(l.Spec.Provider.Name == appRef.Name && l.Spec.Provider.Namespace == appRef.Namespace) {
			reqs = append(reqs, reconcile.Request{
				NamespacedName: client.ObjectKeyFromObject(l),
			})
		}
	}

	return reqs
}

// mapZoneToListeners maps a Zone change to Listeners whose consumer or
// provider Application references that Zone.
func (r *ListenerReconciler) mapZoneToListeners(
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

	// Collect Application refs that reference this Zone.
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

	// Find Listeners that reference any of these Applications.
	list := &spectrev1.ListenerList{}
	if err := r.List(ctx, list, client.MatchingLabels{
		cconfig.EnvironmentLabelKey: zone.Labels[cconfig.EnvironmentLabelKey],
	}); err != nil {
		logger.Error(err, "Failed to list Listeners for Zone")
		return nil
	}

	var reqs []reconcile.Request
	for i := range list.Items {
		l := &list.Items[i]
		consRef := types.NamespacedName{Name: l.Spec.Consumer.Name, Namespace: l.Spec.Consumer.Namespace}
		provRef := types.NamespacedName{Name: l.Spec.Provider.Name, Namespace: l.Spec.Provider.Namespace}
		if _, ok := appRefs[consRef]; ok {
			reqs = append(reqs, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(l)})
			continue
		}
		if _, ok := appRefs[provRef]; ok {
			reqs = append(reqs, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(l)})
		}
	}

	return reqs
}

// mapEventConfigToListeners maps an EventConfig change to Listeners that
// depend on EventConfigs via their zone. EventConfig is keyed by zone,
// so a change can affect any Listener in that zone.
func (r *ListenerReconciler) mapEventConfigToListeners(
	ctx context.Context,
	obj client.Object,
) []reconcile.Request {
	logger := log.FromContext(ctx)
	ec, ok := obj.(*eventv1.EventConfig)
	if !ok {
		return nil
	}

	// Find all Applications in the EventConfig's zone.
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

	list := &spectrev1.ListenerList{}
	if err := r.List(ctx, list, client.MatchingLabels{
		cconfig.EnvironmentLabelKey: ec.Labels[cconfig.EnvironmentLabelKey],
	}); err != nil {
		logger.Error(err, "Failed to list Listeners for EventConfig")
		return nil
	}

	var reqs []reconcile.Request
	for i := range list.Items {
		l := &list.Items[i]
		consRef := types.NamespacedName{Name: l.Spec.Consumer.Name, Namespace: l.Spec.Consumer.Namespace}
		provRef := types.NamespacedName{Name: l.Spec.Provider.Name, Namespace: l.Spec.Provider.Namespace}
		if _, ok := appRefs[consRef]; ok {
			reqs = append(reqs, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(l)})
			continue
		}
		if _, ok := appRefs[provRef]; ok {
			reqs = append(reqs, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(l)})
		}
	}

	return reqs
}

// mapGenericPublisherToListeners maps a Publisher change to Listeners when the
// Publisher is the shared generic Spectre Publisher. When it becomes Ready,
// blocked Listeners in the same environment can proceed.
func (r *ListenerReconciler) mapGenericPublisherToListeners(
	ctx context.Context,
	obj client.Object,
) []reconcile.Request {
	logger := log.FromContext(ctx)
	pub, ok := obj.(*pubsubv1.Publisher)
	if !ok {
		return nil
	}

	// Only react to the shared generic Publisher.
	if pub.Name != util.MakePublisherName(util.GenericEventType) {
		return nil
	}

	envLabel := pub.Labels[cconfig.EnvironmentLabelKey]
	if envLabel == "" {
		return nil
	}

	list := &spectrev1.ListenerList{}
	if err := r.List(ctx, list, client.MatchingLabels{
		cconfig.EnvironmentLabelKey: envLabel,
	}); err != nil {
		logger.Error(err, "Failed to list Listeners for generic Publisher")
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

// mapEventStoreToListeners maps an EventStore change to Listeners that
// depend on it via EventConfig. An EventStore becoming Ready unblocks
// parents that were waiting for it.
func (r *ListenerReconciler) mapEventStoreToListeners(
	ctx context.Context,
	obj client.Object,
) []reconcile.Request {
	logger := log.FromContext(ctx)
	es, ok := obj.(*pubsubv1.EventStore)
	if !ok {
		return nil
	}

	// Find EventConfigs that reference this EventStore.
	ecList := &eventv1.EventConfigList{}
	if err := r.List(ctx, ecList, client.MatchingLabels{
		cconfig.EnvironmentLabelKey: es.Labels[cconfig.EnvironmentLabelKey],
	}); err != nil {
		logger.Error(err, "Failed to list EventConfigs for EventStore")
		return nil
	}

	// For each matching EventConfig, delegate to the EventConfig mapper.
	var reqs []reconcile.Request
	for i := range ecList.Items {
		ec := &ecList.Items[i]
		if ec.Status.EventStore != nil && ec.Status.EventStore.Name == es.Name && ec.Status.EventStore.Namespace == es.Namespace {
			reqs = append(reqs, r.mapEventConfigToListeners(ctx, ec)...)
		}
	}

	return reqs
}
