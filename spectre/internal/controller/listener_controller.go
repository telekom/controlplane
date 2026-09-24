// SPDX-FileCopyrightText: 2025 Deutsche Telekom IT GmbH
// Copyright 2026.
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"maps"

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
	apiv1 "github.com/telekom/controlplane/api/api/v1"
	applicationv1 "github.com/telekom/controlplane/application/api/v1"
	approvalv1 "github.com/telekom/controlplane/approval/api/v1"
	cconfig "github.com/telekom/controlplane/common/pkg/config"
	cc "github.com/telekom/controlplane/common/pkg/controller"
	ctypes "github.com/telekom/controlplane/common/pkg/types"
	"github.com/telekom/controlplane/common/pkg/util/labelutil"
	eventv1 "github.com/telekom/controlplane/event/api/v1"
	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
	identityv1 "github.com/telekom/controlplane/identity/api/v1"
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
// +kubebuilder:rbac:groups=api.cp.ei.telekom.de,resources=apiexposures,verbs=get;list;watch

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
	// Safety reads (revocation, retirement) bypass the cache.
	r.Controller = cc.NewController(&handler.ListenerHandler{Reader: mgr.GetAPIReader()}, r.Client, r.Recorder)

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
		// Consumer, provider, and observer (A) Applications.
		Watches(
			&applicationv1.Application{},
			crhandler.EnqueueRequestsFromMapFunc(r.mapApplicationToListeners),
			builder.WithPredicates(cc.Count("listener", cc.RoleWatches, predicate.ResourceVersionChangedPredicate{})),
		).
		// Zones — readiness changes affect blocked Listeners. Status-only
		// updates pass ResourceVersionChangedPredicate.
		Watches(
			&adminv1.Zone{},
			crhandler.EnqueueRequestsFromMapFunc(r.mapZoneToListeners),
			builder.WithPredicates(cc.Count("listener", cc.RoleWatches, predicate.ResourceVersionChangedPredicate{})),
		).
		// EventConfigs — readiness or CallbackURL/ProxyCallbackURLs changes.
		// Status-only updates pass ResourceVersionChangedPredicate.
		Watches(
			&eventv1.EventConfig{},
			crhandler.EnqueueRequestsFromMapFunc(r.mapEventConfigToListeners),
			builder.WithPredicates(cc.Count("listener", cc.RoleWatches, predicate.ResourceVersionChangedPredicate{})),
		).
		// Realms — IssuerUrl changes affect RouteListener gateway credentials.
		Watches(
			&identityv1.Realm{},
			crhandler.EnqueueRequestsFromMapFunc(r.mapRealmToListeners),
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
		// ApiExposures — activation/deactivation or application label changes.
		Watches(
			&apiv1.ApiExposure{},
			crhandler.EnqueueRequestsFromMapFunc(r.mapApiExposureToListeners),
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
// matches the Route or whose applied placement captures on it. This ensures a
// Listener blocked before Route creation or whose Route changes mode
// reconciles immediately.
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

	t := &listenerTargets{routes: refSet{client.ObjectKeyFromObject(route): {}}}
	var reqs []reconcile.Request
	for i := range list.Items {
		l := &list.Items[i]
		byPath := l.Spec.ApiListener != nil && labelutil.NormalizeValue(l.Spec.ApiListener.ApiBasePath) == route.Name
		if byPath || t.statusRefersTo(l) {
			reqs = append(reqs, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(l)})
		}
	}

	return reqs
}

// mapApplicationToListeners maps an Application change to Listeners that
// reference it as consumer or provider, or that reference it indirectly
// as the observer (A) via SpectreApplication.spec.application.
func (r *ListenerReconciler) mapApplicationToListeners(
	ctx context.Context,
	obj client.Object,
) []reconcile.Request {
	app, ok := obj.(*applicationv1.Application)
	if !ok {
		return nil
	}

	return r.listenersFor(ctx, app.Labels[cconfig.EnvironmentLabelKey],
		&listenerTargets{apps: refSet{client.ObjectKeyFromObject(app): {}}})
}

// mapZoneToListeners maps a Zone change to Listeners whose consumer, provider
// or observer (A) Application is in that Zone or in a proxy zone that targets
// it, and to Listeners whose applied placement references the Zone.
func (r *ListenerReconciler) mapZoneToListeners(
	ctx context.Context,
	obj client.Object,
) []reconcile.Request {
	zone, ok := obj.(*adminv1.Zone)
	if !ok {
		return nil
	}

	env := zone.Labels[cconfig.EnvironmentLabelKey]
	if env == "" {
		return nil
	}
	zones := withProxyZones(refSet{client.ObjectKeyFromObject(zone): {}}, r.eventConfigsIn(ctx, env))
	return r.listenersFor(ctx, env, &listenerTargets{zones: zones})
}

// mapEventConfigToListeners maps an EventConfig change to Listeners that
// depend on EventConfigs via their zone. EventConfig is keyed by zone,
// so a change can affect any Listener in that zone: through its consumer,
// provider or observer (A) Application, or its applied placement. Proxy zones
// that target the EventConfig's zone are included, because a Listener in a
// proxy zone reads the target zone's EventConfig.
func (r *ListenerReconciler) mapEventConfigToListeners(
	ctx context.Context,
	obj client.Object,
) []reconcile.Request {
	ec, ok := obj.(*eventv1.EventConfig)
	if !ok {
		return nil
	}

	env := ec.Labels[cconfig.EnvironmentLabelKey]
	if env == "" {
		return nil
	}
	zones := withProxyZones(refSet{ec.Spec.Zone.K8s(): {}}, r.eventConfigsIn(ctx, env))
	return r.listenersFor(ctx, env, &listenerTargets{zones: zones})
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

// mapRealmToListeners maps a Realm change to Listeners that depend on it
// via the Zone's status.identityRealm. A Realm change (e.g. IssuerUrl
// update) affects RouteListener gateway credentials for all Listeners
// whose consumer, provider or observer (A) Applications are in Zones
// referencing that Realm, or whose applied placement references such a Zone.
func (r *ListenerReconciler) mapRealmToListeners(
	ctx context.Context,
	obj client.Object,
) []reconcile.Request {
	logger := log.FromContext(ctx)
	realm, ok := obj.(*identityv1.Realm)
	if !ok {
		return nil
	}

	realmRef := types.NamespacedName{Name: realm.Name, Namespace: realm.Namespace}

	// Find Zones whose status.identityRealm references this Realm.
	zoneList := &adminv1.ZoneList{}
	if err := r.List(ctx, zoneList, client.MatchingLabels{
		cconfig.EnvironmentLabelKey: realm.Labels[cconfig.EnvironmentLabelKey],
	}); err != nil {
		logger.Error(err, "Failed to list Zones for Realm")
		return nil
	}

	zoneRefs := refSet{}
	for i := range zoneList.Items {
		z := &zoneList.Items[i]
		if z.Status.IdentityRealm != nil &&
			z.Status.IdentityRealm.Name == realmRef.Name &&
			z.Status.IdentityRealm.Namespace == realmRef.Namespace {
			zoneRefs[types.NamespacedName{Name: z.Name, Namespace: z.Namespace}] = struct{}{}
		}
	}

	if len(zoneRefs) == 0 {
		return nil
	}

	return r.listenersFor(ctx, realm.Labels[cconfig.EnvironmentLabelKey], &listenerTargets{zones: zoneRefs})
}

// mapEventStoreToListeners maps an EventStore change to Listeners that
// depend on it via EventConfig, or whose applied placement or drain references
// it. An EventStore becoming Ready unblocks parents that were waiting for it.
func (r *ListenerReconciler) mapEventStoreToListeners(
	ctx context.Context,
	obj client.Object,
) []reconcile.Request {
	es, ok := obj.(*pubsubv1.EventStore)
	if !ok {
		return nil
	}

	env := es.Labels[cconfig.EnvironmentLabelKey]
	if env == "" {
		return nil
	}
	t := &listenerTargets{eventStores: refSet{client.ObjectKeyFromObject(es): {}}}
	ecs := r.eventConfigsIn(ctx, env)
	zones := refSet{}
	for i := range ecs {
		if t.eventStores.has(ecs[i].Status.EventStore) {
			zones[ecs[i].Spec.Zone.K8s()] = struct{}{}
		}
	}
	t.zones = withProxyZones(zones, ecs)
	return r.listenersFor(ctx, env, t)
}

// mapApiExposureToListeners maps an ApiExposure change to Listeners whose
// apiBasePath corresponds to a Route owned by the ApiExposure. This ensures
// that activation/deactivation or application label changes on the exposure
// wake the affected Listeners.
func (r *ListenerReconciler) mapApiExposureToListeners(
	ctx context.Context,
	obj client.Object,
) []reconcile.Request {
	logger := log.FromContext(ctx)
	exposure, ok := obj.(*apiv1.ApiExposure)
	if !ok {
		return nil
	}

	// Collect Route names referenced in the ApiExposure's status.
	routeNames := make(map[string]struct{})
	if exposure.Status.Route != nil {
		routeNames[exposure.Status.Route.Name] = struct{}{}
	}
	for i := range exposure.Status.ProxyRoutes {
		routeNames[exposure.Status.ProxyRoutes[i].Name] = struct{}{}
	}

	if len(routeNames) == 0 {
		return nil
	}

	envLabel := exposure.Labels[cconfig.EnvironmentLabelKey]
	if envLabel == "" {
		return nil
	}

	list := &spectrev1.ListenerList{}
	if err := r.List(ctx, list, client.MatchingLabels{
		cconfig.EnvironmentLabelKey: envLabel,
	}); err != nil {
		logger.Error(err, "Failed to list Listeners for ApiExposure")
		return nil
	}

	var reqs []reconcile.Request
	for i := range list.Items {
		l := &list.Items[i]
		if l.Spec.ApiListener == nil {
			continue
		}
		routeName := labelutil.NormalizeValue(l.Spec.ApiListener.ApiBasePath)
		if _, ok := routeNames[routeName]; ok {
			reqs = append(reqs, reconcile.Request{
				NamespacedName: client.ObjectKeyFromObject(l),
			})
		}
	}

	return reqs
}

// refSet is a set of object keys; a nil refSet matches nothing.
type refSet map[types.NamespacedName]struct{}

// has reports whether ref names a member. Matching uses Name and Namespace
// only, like ObjectRef.Equals; the UID is ignored.
func (s refSet) has(ref *ctypes.ObjectRef) bool {
	if ref == nil {
		return false
	}
	_, ok := s[ref.K8s()]
	return ok
}

// listenerTargets holds the changed objects, by kind, that Listeners may
// depend on.
type listenerTargets struct {
	apps, zones, eventStores, routes refSet
}

// statusRefersTo reports whether the Listener's applied placement or drain
// references a target. It keeps applied and draining dependencies mapped after
// the spec associations move away from them and while a drain needs them.
func (t *listenerTargets) statusRefersTo(l *spectrev1.Listener) bool {
	if ap := l.Status.AppliedPlacement; ap != nil &&
		(t.zones.has(ap.CaptureZone) || t.zones.has(ap.DeliveryZone) || t.zones.has(ap.CallbackOriginZone) ||
			t.eventStores.has(ap.CaptureEventStore) || t.eventStores.has(ap.DeliveryEventStore) ||
			t.routes.has(ap.CaptureRoute)) {
		return true
	}
	return l.Status.Draining != nil && t.eventStores.has(l.Status.Draining.SourceEventStore)
}

// listenersFor returns one request per Listener in env that depends on t:
// through its consumer or provider Application, its observer (A) Application
// via SpectreApplication.spec.application, an Application in a target zone,
// or its applied placement and drain refs. It returns nil when env is empty or
// the Listener list fails.
func (r *ListenerReconciler) listenersFor(ctx context.Context, env string, t *listenerTargets) []reconcile.Request {
	if env == "" {
		return nil
	}
	inEnv := client.MatchingLabels{cconfig.EnvironmentLabelKey: env}
	apps := r.appsInZones(ctx, inEnv, t.zones)
	maps.Copy(apps, t.apps)
	observers := r.spectreAppsFor(ctx, inEnv, apps)

	list := &spectrev1.ListenerList{}
	if err := r.List(ctx, list, inEnv); err != nil {
		log.FromContext(ctx).Error(err, "Failed to list Listeners for dependency change")
		return nil
	}

	var reqs []reconcile.Request
	for i := range list.Items {
		l := &list.Items[i]
		if apps.has(&l.Spec.Consumer.ObjectRef) || apps.has(&l.Spec.Provider.ObjectRef) ||
			observers.has(&l.Spec.Application) || t.statusRefersTo(l) {
			reqs = append(reqs, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(l)})
		}
	}
	return reqs
}

// appsInZones returns the Applications in env whose spec.zone is in zones. A
// list failure is logged and yields no Applications; status refs still match.
func (r *ListenerReconciler) appsInZones(ctx context.Context, inEnv client.MatchingLabels, zones refSet) refSet {
	apps := refSet{}
	if len(zones) == 0 {
		return apps
	}
	list := &applicationv1.ApplicationList{}
	if err := r.List(ctx, list, inEnv); err != nil {
		log.FromContext(ctx).Error(err, "Failed to list Applications for Listener dependency mapping")
		return apps
	}
	for i := range list.Items {
		if zones.has(&list.Items[i].Spec.Zone) {
			apps[client.ObjectKeyFromObject(&list.Items[i])] = struct{}{}
		}
	}
	return apps
}

// spectreAppsFor returns the SpectreApplications in env whose spec.application
// is in apps. A list failure is logged and yields none; consumer and provider
// matching still works.
func (r *ListenerReconciler) spectreAppsFor(ctx context.Context, inEnv client.MatchingLabels, apps refSet) refSet {
	observers := refSet{}
	if len(apps) == 0 {
		return observers
	}
	list := &spectrev1.SpectreApplicationList{}
	if err := r.List(ctx, list, inEnv); err != nil {
		log.FromContext(ctx).Error(err, "Failed to list SpectreApplications for Listener dependency mapping")
		return observers
	}
	for i := range list.Items {
		if apps.has(&list.Items[i].Spec.Application.ObjectRef) {
			observers[client.ObjectKeyFromObject(&list.Items[i])] = struct{}{}
		}
	}
	return observers
}

// eventConfigsIn lists the EventConfigs in env. A list failure is logged and
// yields none, so only the changed object's own zone is mapped.
func (r *ListenerReconciler) eventConfigsIn(ctx context.Context, env string) []eventv1.EventConfig {
	list := &eventv1.EventConfigList{}
	if err := r.List(ctx, list, client.MatchingLabels{cconfig.EnvironmentLabelKey: env}); err != nil {
		log.FromContext(ctx).Error(err, "Failed to list EventConfigs for Listener dependency mapping")
		return nil
	}
	return list.Items
}

// withProxyZones returns zones plus every proxy zone whose EventConfig targets
// one of them: a Listener in a proxy zone reads the target zone and its
// EventConfig (resolveSSEBackendZone).
func withProxyZones(zones refSet, ecs []eventv1.EventConfig) refSet {
	out := maps.Clone(zones)
	for i := range ecs {
		p := &ecs[i]
		if p.IsProxy() && zones.has(&p.Spec.Proxy.TargetZone) {
			out[p.Spec.Zone.K8s()] = struct{}{}
		}
	}
	return out
}
