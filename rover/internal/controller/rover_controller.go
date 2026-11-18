// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"slices"
	"strings"

	"k8s.io/client-go/util/workqueue"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	agenticv1 "github.com/telekom/controlplane/agentic/api/v1"
	apiapi "github.com/telekom/controlplane/api/api/v1"
	application "github.com/telekom/controlplane/application/api/v1"
	cconfig "github.com/telekom/controlplane/common/pkg/config"
	cc "github.com/telekom/controlplane/common/pkg/controller"
	eventv1 "github.com/telekom/controlplane/event/api/v1"
	filev1 "github.com/telekom/controlplane/file/api/v1"
	organizationv1 "github.com/telekom/controlplane/organization/api/v1"
	permissionv1 "github.com/telekom/controlplane/permission/api/v1"
	rover "github.com/telekom/controlplane/rover/api/v1"
	rover_handler "github.com/telekom/controlplane/rover/internal/handler/rover"
	application_handler "github.com/telekom/controlplane/rover/internal/handler/rover/application"
	spectrev1 "github.com/telekom/controlplane/spectre/api/v1"
)

// RoverReconciler reconciles a Rover object
type RoverReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder

	cc.Controller[*rover.Rover]
}

// +kubebuilder:rbac:groups=core,resources=events,verbs=create;patch

// +kubebuilder:rbac:groups=rover.cp.ei.telekom.de,resources=rovers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=rover.cp.ei.telekom.de,resources=rovers/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=rover.cp.ei.telekom.de,resources=rovers/finalizers,verbs=update

// +kubebuilder:rbac:groups=organization.cp.ei.telekom.de,resources=teams,verbs=get;list;watch

// +kubebuilder:rbac:groups=api.cp.ei.telekom.de,resources=apisubscriptions,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=api.cp.ei.telekom.de,resources=apiexposures,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=api.cp.ei.telekom.de,resources=apicategories,verbs=get;list;watch

// +kubebuilder:rbac:groups=event.cp.ei.telekom.de,resources=eventexposures,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=event.cp.ei.telekom.de,resources=eventsubscriptions,verbs=get;list;watch;create;update;patch;delete

// +kubebuilder:rbac:groups=agentic.cp.ei.telekom.de,resources=agenticexposures,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=agentic.cp.ei.telekom.de,resources=agenticsubscriptions,verbs=get;list;watch;create;update;patch;delete

// +kubebuilder:rbac:groups=file.cp.ei.telekom.de,resources=fileexposures,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=file.cp.ei.telekom.de,resources=filesubscriptions,verbs=get;list;watch;create;update;patch;delete

// +kubebuilder:rbac:groups=permission.cp.ei.telekom.de,resources=permissionsets,verbs=get;list;watch;create;update;patch;delete

// +kubebuilder:rbac:groups=application.cp.ei.telekom.de,resources=applications,verbs=get;list;watch;create;update;patch;delete

// +kubebuilder:rbac:groups=spectre.cp.ei.telekom.de,resources=spectreapplications,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=spectre.cp.ei.telekom.de,resources=listeners,verbs=get;list;watch;create;update;patch;delete

func (r *RoverReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	return r.Controller.Reconcile(ctx, req, &rover.Rover{})
}

// SetupWithManager sets up the controller with the Manager.
func (r *RoverReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Recorder = mgr.GetEventRecorderFor("rover-controller")
	r.Controller = cc.NewController(&rover_handler.RoverHandler{}, r.Client, r.Recorder)

	owns := builder.WithPredicates(cc.Count("rover", cc.RoleOwns))

	b := ctrl.NewControllerManagedBy(mgr).
		For(&rover.Rover{}, builder.WithPredicates(cc.Count("rover", cc.RoleFor))).
		Owns(&apiapi.ApiSubscription{}, owns).
		Owns(&apiapi.ApiExposure{}, owns).
		Owns(&application.Application{}, owns)

	if cconfig.FeaturePubSub.IsEnabled() {
		b = b.Owns(&eventv1.EventExposure{}, owns).
			Owns(&eventv1.EventSubscription{}, owns).
			Watches(&rover.Rover{}, r.publisherRoverHandler(),
				builder.WithPredicates(cc.Count("rover", cc.RoleWatches, predicate.GenerationChangedPredicate{})))
	}

	if cconfig.FeaturePermission.IsEnabled() {
		b = b.Owns(&permissionv1.PermissionSet{}, owns)
	}

	if cconfig.FeatureAiGateway.IsEnabled() {
		b = b.Owns(&agenticv1.AgenticExposure{}, owns).
			Owns(&agenticv1.AgenticSubscription{}, owns)
	}

	if cconfig.FeatureFile.IsEnabled() {
		b = b.Owns(&filev1.FileExposure{}).
			Owns(&filev1.FileSubscription{})
	}

	if cconfig.FeatureSpectre.IsEnabled() {
		b = b.Owns(&spectrev1.SpectreApplication{}, owns).
			Owns(&spectrev1.Listener{}, owns).
			Watches(&application.Application{},
				handler.EnqueueRequestsFromMapFunc(r.MapApplicationToListenerRovers),
				builder.WithPredicates(cc.Count("rover", cc.RoleWatches, predicate.GenerationChangedPredicate{}, cc.SkipInitialListPredicate{})),
			)
	}

	b = b.Watches(&organizationv1.Team{},
		handler.EnqueueRequestsFromMapFunc(r.MapTeamToRovers),
		builder.WithPredicates(cc.Count("rover", cc.RoleWatches, predicate.GenerationChangedPredicate{}, cc.SkipInitialListPredicate{})),
	)

	return b.WithOptions(controller.Options{
		MaxConcurrentReconciles: cconfig.MaxConcurrentReconciles,
		RateLimiter:             cc.NewRateLimiter(),
	}).
		Complete(r)
}

// MapTeamToRovers maps a Team to all Rovers in the team's namespace.
// This enables re-reconciliation of Rovers when Team.Spec (e.g. Email) changes.
func (r *RoverReconciler) MapTeamToRovers(ctx context.Context, obj client.Object) []reconcile.Request {
	logger := log.FromContext(ctx)

	team, ok := obj.(*organizationv1.Team)
	if !ok || team.Status.Namespace == "" {
		return nil
	}

	roverList := &rover.RoverList{}
	if err := r.List(ctx, roverList, client.InNamespace(team.Status.Namespace)); err != nil {
		logger.Error(err, "Failed to list Rovers", "namespace", team.Status.Namespace)
		return nil
	}

	reqs := make([]reconcile.Request, 0, len(roverList.Items))
	for i := range roverList.Items {
		reqs = append(reqs, reconcile.Request{
			NamespacedName: client.ObjectKeyFromObject(&roverList.Items[i]),
		})
	}

	return reqs
}

// MapApplicationToListenerRovers maps an Application to the Rovers whose listeners
// reference it as consumer or provider, so that a listener blocked on a missing
// or ambiguous Application is reconciled as soon as that Application changes.
func (r *RoverReconciler) MapApplicationToListenerRovers(ctx context.Context, obj client.Object) []reconcile.Request {
	logger := log.FromContext(ctx)

	roverList := &rover.RoverList{}
	if err := r.List(ctx, roverList,
		client.MatchingFields{ListenerApplicationIndexKey: obj.GetName()},
		client.MatchingLabels{cconfig.EnvironmentLabelKey: obj.GetLabels()[cconfig.EnvironmentLabelKey]},
	); err != nil {
		logger.Error(err, "Failed to list Rovers for Application", "application", client.ObjectKeyFromObject(obj))
		return nil
	}

	reqs := make([]reconcile.Request, 0, len(roverList.Items))
	for i := range roverList.Items {
		reqs = append(reqs, reconcile.Request{
			NamespacedName: client.ObjectKeyFromObject(&roverList.Items[i]),
		})
	}

	return reqs
}

// publisherRoverHandler requeues the Rovers that are listed as additional
// publishers by an owner Rover. On update both the old and the new object are
// mapped, so removed IDs are reconciled too.
func (r *RoverReconciler) publisherRoverHandler() handler.EventHandler {
	enqueue := func(ctx context.Context, q workqueue.TypedRateLimitingInterface[reconcile.Request], objs ...client.Object) {
		var ids []string
		var env string
		for _, o := range objs {
			ro, ok := o.(*rover.Rover)
			if !ok {
				continue
			}
			env = ro.GetLabels()[cconfig.EnvironmentLabelKey]
			ids = append(ids, application_handler.ListedPublisherIds(ro)...)
		}
		slices.Sort(ids)
		ids = slices.Compact(ids)
		for _, req := range r.MapPublisherIdsToRovers(ctx, env, ids) {
			q.Add(req)
		}
	}
	return handler.Funcs{
		CreateFunc: func(ctx context.Context, e event.CreateEvent, q workqueue.TypedRateLimitingInterface[reconcile.Request]) {
			enqueue(ctx, q, e.Object)
		},
		UpdateFunc: func(ctx context.Context, e event.UpdateEvent, q workqueue.TypedRateLimitingInterface[reconcile.Request]) {
			enqueue(ctx, q, e.ObjectOld, e.ObjectNew)
		},
		DeleteFunc: func(ctx context.Context, e event.DeleteEvent, q workqueue.TypedRateLimitingInterface[reconcile.Request]) {
			enqueue(ctx, q, e.Object)
		},
	}
}

// MapPublisherIdsToRovers resolves publisher IDs (<team.Name>--<rover.Name>) to
// Rover requests via the Applications' Status.ClientId in the given environment.
// Unresolved IDs are ignored.
func (r *RoverReconciler) MapPublisherIdsToRovers(ctx context.Context, env string, ids []string) []reconcile.Request {
	if len(ids) == 0 {
		return nil
	}
	logger := log.FromContext(ctx)
	appList := &application.ApplicationList{}
	if err := r.List(ctx, appList, client.MatchingLabels{cconfig.EnvironmentLabelKey: env}); err != nil {
		logger.Error(err, "Failed to list Applications for additional publishers")
		return nil
	}
	reqs := make([]reconcile.Request, 0, len(ids))
	for i := range appList.Items {
		app := &appList.Items[i]
		if slices.Contains(ids, app.Status.ClientId) {
			reqs = append(reqs, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(app)})
		}
	}
	slices.SortFunc(reqs, func(a, b reconcile.Request) int {
		return strings.Compare(a.String(), b.String())
	})
	return reqs
}
