// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"strings"

	cconfig "github.com/telekom/controlplane/common/pkg/config"
	cc "github.com/telekom/controlplane/common/pkg/controller"
	identityv1 "github.com/telekom/controlplane/identity/api/v1"
	organizationv1 "github.com/telekom/controlplane/organization/api/v1"
	teamhandler "github.com/telekom/controlplane/organization/internal/handler/team"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// TeamReconciler reconciles a Team object
type TeamReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder

	cc.Controller[*organizationv1.Team]
}

// +kubebuilder:rbac:groups=organization.cp.ei.telekom.de,resources=groups,verbs=get;list;watch;patch
// +kubebuilder:rbac:groups=organization.cp.ei.telekom.de,resources=groups/status,verbs=get;list;watch;update
// +kubebuilder:rbac:groups=organization.cp.ei.telekom.de,resources=teams,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=organization.cp.ei.telekom.de,resources=teams/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=organization.cp.ei.telekom.de,resources=teams/finalizers,verbs=update
// +kubebuilder:rbac:groups=identity.cp.ei.telekom.de,resources=clients,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=gateway.cp.ei.telekom.de,resources=consumers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=admin.cp.ei.telekom.de,resources=zones,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=namespaces,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch

func (r *TeamReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	return r.Controller.Reconcile(ctx, req, &organizationv1.Team{})
}

// SetupWithManager sets up the controller with the Manager.
func (r *TeamReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Recorder = mgr.GetEventRecorderFor("team-controller")
	r.Controller = cc.NewController(&teamhandler.TeamHandler{}, r.Client, r.Recorder)

	return ctrl.NewControllerManagedBy(mgr).
		For(&organizationv1.Team{}).
		Watches(&identityv1.Client{},
			handler.EnqueueRequestsFromMapFunc(r.mapClientToTeam),
			builder.WithPredicates(predicate.GenerationChangedPredicate{})).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: cconfig.MaxConcurrentReconciles,
			RateLimiter:             cc.NewRateLimiter(),
		}).
		Complete(r)
}

func (r *TeamReconciler) mapClientToTeam(ctx context.Context, obj client.Object) []reconcile.Request {
	logger := log.FromContext(ctx)

	identityClient, ok := obj.(*identityv1.Client)
	if !ok {
		return nil
	}

	listOptsForTeams := []client.ListOption{
		client.MatchingLabels{
			cconfig.EnvironmentLabelKey: identityClient.Labels[cconfig.EnvironmentLabelKey],
		},
	}

	teamList := organizationv1.TeamList{}
	if err := r.List(ctx, &teamList, listOptsForTeams...); err != nil {
		logger.Error(err, "failed to list Teams")
		return nil
	}

	requests := make([]reconcile.Request, 0, len(teamList.Items))
	for _, team := range teamList.Items {
		if team.Status.Namespace == identityClient.GetNamespace() && strings.HasSuffix(identityClient.GetName(), "--team-user") {
			requests = append(requests, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(&team)})
		}
	}

	return requests
}
