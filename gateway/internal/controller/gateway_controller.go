// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	ctrlhandler "sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	cconfig "github.com/telekom/controlplane/common/pkg/config"
	cc "github.com/telekom/controlplane/common/pkg/controller"
	"github.com/telekom/controlplane/common/pkg/types"
	v1 "github.com/telekom/controlplane/gateway/api/v1"
	"github.com/telekom/controlplane/gateway/internal/features/envoy"
	gatewayhandler "github.com/telekom/controlplane/gateway/internal/handler/gateway"
)

// GatewayReconciler reconciles a Gateway object
type GatewayReconciler struct {
	client.Client
	Scheme     *runtime.Scheme
	Recorder   record.EventRecorder
	OnCompiled func(context.Context, envoy.ResourceBundle) (XDSPublicationResult, error)

	cc.Controller[*v1.Gateway]
}

// XDSPublicationResult is the non-secret activation state reported by the management server.
type XDSPublicationResult struct {
	PersistedGeneration uint64
	Digest              string
	Activated           bool
	Idempotent          bool
}

const (
	conditionTypeXDSProgrammed = "XDSProgrammed"
)

// +kubebuilder:rbac:groups=core,resources=events,verbs=create;patch
// +kubebuilder:rbac:groups=gateway.cp.ei.telekom.de,resources=gateways,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=gateway.cp.ei.telekom.de,resources=gateways/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=gateway.cp.ei.telekom.de,resources=gateways/finalizers,verbs=update

func (r *GatewayReconciler) Reconcile( //nolint:gocyclo // Cutover ordering is clearer in one coordinator.
	ctx context.Context,
	req ctrl.Request,
) (ctrl.Result, error) {
	existing := &v1.Gateway{}
	deleting := false
	if getErr := r.Get(ctx, req.NamespacedName, existing); getErr == nil && !existing.DeletionTimestamp.IsZero() {
		deleting = true
	}
	if deleting && existing.Status.XDSTargetID != "" {
		if targetErr := validateRecordedTarget(existing, existing.Status.XDSTargetID, false); targetErr != nil {
			return ctrl.Result{}, fmt.Errorf("validating deleted Gateway xDS target: %w", targetErr)
		}
		if r.OnCompiled == nil {
			return ctrl.Result{}, fmt.Errorf("xDS publisher is not configured")
		}
		emptyBundle, bundleErr := emptyGatewayBundle(
			existing, existing.Status.XDSTargetID, "GatewayDeletion")
		if bundleErr != nil {
			return ctrl.Result{}, fmt.Errorf("building deleted Envoy Gateway deactivation: %w", bundleErr)
		}
		if _, publishErr := r.OnCompiled(ctx, emptyBundle); publishErr != nil {
			return ctrl.Result{}, fmt.Errorf("deactivating deleted Envoy Gateway: %w", publishErr)
		}
	}

	result, err := r.Controller.Reconcile(ctx, req, &v1.Gateway{})
	if err != nil {
		return result, err
	}
	if deleting {
		return result, nil
	}
	current := &v1.Gateway{}
	if getErr := r.Get(ctx, req.NamespacedName, current); getErr != nil {
		return result, client.IgnoreNotFound(getErr)
	}
	if current.Labels[cconfig.EnvironmentLabelKey] == "" {
		return result, nil
	}
	if recordedTargetID := current.Status.XDSTargetID; recordedTargetID != "" {
		if targetErr := validateRecordedTarget(current, recordedTargetID, true); targetErr != nil {
			return result, r.setXDSFailureCondition(
				ctx, req, "XDSTargetImmutable",
				"Gateway environment cannot change while its xDS target is active")
		}
	}
	bundle, err := r.compileGateway(ctx, req)
	if err != nil {
		return ctrl.Result{}, r.withXDSFailure(ctx, req, "XDSCompilationFailed", "xDS compilation failed", err)
	}
	if bundle == nil {
		return result, r.deactivateConvertedGateway(ctx, req)
	}
	currentTargetID := targetID(bundle.Target)
	previousTargetID := current.Status.XDSTargetID
	if previousTargetID != "" && previousTargetID != currentTargetID {
		return result, r.setXDSFailureCondition(
			ctx, req, "XDSTargetImmutable",
			"Gateway environment cannot change while its xDS target is active")
	}
	if previousTargetID == "" {
		if statusErr := r.setXDSTarget(ctx, req, currentTargetID); statusErr != nil {
			return ctrl.Result{}, statusErr
		}
		return ctrl.Result{Requeue: true}, nil
	}
	if r.OnCompiled == nil {
		return ctrl.Result{}, r.withXDSFailure(
			ctx, req, "XDSPublisherUnavailable", "xDS publisher is not configured",
			fmt.Errorf("xDS publisher is not configured"))
	}
	publication, err := r.OnCompiled(ctx, *bundle)
	if err != nil {
		return ctrl.Result{}, r.withXDSFailure(ctx, req, "XDSPublicationFailed", "xDS publication failed", err)
	}
	reason := "XDSPersisted"
	message := fmt.Sprintf("xDS generation %d persisted", publication.PersistedGeneration)
	if publication.Activated {
		reason = "XDSActivated"
		message = fmt.Sprintf("xDS generation %d activated", publication.PersistedGeneration)
	}
	if publication.Idempotent {
		message += " (unchanged)"
	}
	if !publication.Activated {
		reason = "XDSSuperseded"
		message = fmt.Sprintf("xDS generation %d is persisted but no longer active", publication.PersistedGeneration)
	}
	if err := r.setXDSConditionForGeneration(
		ctx, req, bundleGatewayGeneration(bundle), publication.Activated, reason, message,
	); err != nil {
		return ctrl.Result{}, err
	}
	return result, nil
}

func (r *GatewayReconciler) deactivateConvertedGateway(ctx context.Context, req ctrl.Request) error {
	gateway := &v1.Gateway{}
	if err := r.Get(ctx, req.NamespacedName, gateway); err != nil {
		return client.IgnoreNotFound(err)
	}
	previousTargetID := gateway.Status.XDSTargetID
	if gateway.Spec.Type == v1.GatewayTypeEnvoy || previousTargetID == "" {
		return nil
	}
	condition := meta.FindStatusCondition(gateway.Status.Conditions, conditionTypeXDSProgrammed)
	if condition != nil && condition.Reason == "XDSDeactivated" && condition.ObservedGeneration == gateway.Generation {
		return nil
	}
	configured, err := r.kongStateMatches(ctx, gateway, true)
	if err != nil {
		return err
	}
	if !configured {
		return r.setXDSFailureCondition(
			ctx, req, "XDSCutoverPending", "waiting for Kong resources to be provisioned")
	}
	if r.OnCompiled == nil {
		return fmt.Errorf("xDS publisher is not configured")
	}
	emptyBundle, err := emptyGatewayBundle(gateway, previousTargetID, "GatewayConversion")
	if err != nil {
		return r.withXDSFailure(ctx, req, "XDSDeactivationFailed", "xDS deactivation failed", err)
	}
	publication, err := r.OnCompiled(ctx, emptyBundle)
	if err != nil {
		return r.withXDSFailure(ctx, req, "XDSDeactivationFailed", "xDS deactivation failed", err)
	}
	message := fmt.Sprintf("xDS generation %d deactivated", publication.PersistedGeneration)
	if err := r.setXDSFailureCondition(ctx, req, "XDSDeactivated", message); err != nil {
		return err
	}
	return nil
}

func (r *GatewayReconciler) kongStateMatches(ctx context.Context, gateway *v1.Gateway, configured bool) (bool, error) {
	ref := types.ObjectRefFromObject(gateway).String()
	routes := &v1.RouteList{}
	if err := r.List(ctx, routes, client.MatchingFields{IndexFieldSpecGatewayRef: ref}); err != nil {
		return false, fmt.Errorf("listing Gateway routes for backend cutover: %w", err)
	}
	for i := range routes.Items {
		if !referenceMatchesGateway(routes.Items[i].Spec.GatewayRef, gateway) {
			continue
		}
		if !resourceReady(routes.Items[i].Status.Conditions, routes.Items[i].Generation) ||
			(len(routes.Items[i].Status.Properties) > 0) != configured {
			return false, nil
		}
	}
	consumers := &v1.ConsumerList{}
	if err := r.List(ctx, consumers, client.MatchingFields{IndexFieldSpecGateway: ref}); err != nil {
		return false, fmt.Errorf("listing Gateway consumers for backend cutover: %w", err)
	}
	for i := range consumers.Items {
		if !referenceMatchesGateway(consumers.Items[i].Spec.Gateway, gateway) {
			continue
		}
		if !resourceReady(consumers.Items[i].Status.Conditions, consumers.Items[i].Generation) ||
			(len(consumers.Items[i].Status.Properties) > 0) != configured {
			return false, nil
		}
	}
	return true, nil
}

func resourceReady(conditions []metav1.Condition, generation int64) bool {
	ready := meta.FindStatusCondition(conditions, "Ready")
	return ready != nil && ready.Status == metav1.ConditionTrue && ready.ObservedGeneration == generation
}

func referenceMatchesGateway(reference types.ObjectRef, gateway *v1.Gateway) bool {
	return reference.Equals(gateway) && (reference.UID == "" || reference.UID == gateway.UID)
}

func (r *GatewayReconciler) withXDSFailure(
	ctx context.Context,
	req ctrl.Request,
	reason string,
	message string,
	cause error,
) error {
	conditionErr := r.setXDSFailureCondition(ctx, req, reason, message)
	return errors.Join(cause, conditionErr)
}

func (r *GatewayReconciler) setXDSFailureCondition(
	ctx context.Context,
	req ctrl.Request,
	reason string,
	message string,
) error {
	return r.setXDSConditionForGeneration(ctx, req, 0, false, reason, message)
}

func (r *GatewayReconciler) setXDSConditionForGeneration(
	ctx context.Context,
	req ctrl.Request,
	expectedGeneration int64,
	programmed bool,
	reason string,
	message string,
) error {
	if err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		gateway := &v1.Gateway{}
		if err := r.Get(ctx, req.NamespacedName, gateway); err != nil {
			return client.IgnoreNotFound(err)
		}
		if expectedGeneration != 0 && gateway.Generation != expectedGeneration {
			return fmt.Errorf(
				"gateway generation changed during xDS publication: compiled %d, current %d",
				expectedGeneration, gateway.Generation)
		}
		conditionStatus := metav1.ConditionFalse
		if programmed {
			conditionStatus = metav1.ConditionTrue
		}
		gateway.SetCondition(metav1.Condition{
			Type: conditionTypeXDSProgrammed, Status: conditionStatus, Reason: reason,
			Message: message, ObservedGeneration: gateway.Generation,
		})
		return r.Status().Update(ctx, gateway)
	}); err != nil {
		return fmt.Errorf("updating Gateway xDS condition: %w", err)
	}
	return nil
}

func bundleGatewayGeneration(bundle *envoy.ResourceBundle) int64 {
	for _, source := range bundle.Source.Resources {
		if source.Kind == "Gateway" {
			return source.Generation
		}
	}
	return 0
}

func (r *GatewayReconciler) setXDSTarget(ctx context.Context, req ctrl.Request, targetID string) error {
	if err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		gateway := &v1.Gateway{}
		if err := r.Get(ctx, req.NamespacedName, gateway); err != nil {
			return client.IgnoreNotFound(err)
		}
		if gateway.Status.XDSTargetID == targetID {
			return nil
		}
		gateway.Status.XDSTargetID = targetID
		return r.Status().Update(ctx, gateway)
	}); err != nil {
		return fmt.Errorf("updating Gateway xDS target status: %w", err)
	}
	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *GatewayReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Recorder = mgr.GetEventRecorderFor("gateway-controller")
	r.Controller = cc.NewController(&gatewayhandler.GatewayHandler{}, r.Client, r.Recorder)

	return ctrl.NewControllerManagedBy(mgr).
		For(&v1.Gateway{}, builder.WithPredicates(predicate.Or(
			predicate.GenerationChangedPredicate{},
			predicate.Funcs{UpdateFunc: func(update event.UpdateEvent) bool {
				return update.ObjectOld.GetLabels()[cconfig.EnvironmentLabelKey] !=
					update.ObjectNew.GetLabels()[cconfig.EnvironmentLabelKey] ||
					(update.ObjectOld.GetDeletionTimestamp().IsZero() &&
						!update.ObjectNew.GetDeletionTimestamp().IsZero()) ||
					!slices.Equal(update.ObjectOld.GetFinalizers(), update.ObjectNew.GetFinalizers())
			}},
		))).
		Watches(&v1.Route{}, ctrlhandler.EnqueueRequestsFromMapFunc(r.mapRouteToGateway),
			builder.WithPredicates(predicate.Or(predicate.GenerationChangedPredicate{}, routeCutoverStateChanged()))).
		Watches(&v1.Consumer{}, ctrlhandler.EnqueueRequestsFromMapFunc(r.mapConsumerToGateway),
			builder.WithPredicates(predicate.Or(predicate.GenerationChangedPredicate{}, consumerCutoverStateChanged()))).
		Watches(&v1.ConsumeRoute{}, ctrlhandler.EnqueueRequestsFromMapFunc(r.mapConsumeRouteToGateway),
			builder.WithPredicates(predicate.GenerationChangedPredicate{})).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: cconfig.MaxConcurrentReconciles,
			RateLimiter:             cc.NewRateLimiter(),
		}).
		Complete(r)
}

func routeCutoverStateChanged() predicate.Predicate {
	return predicate.Funcs{UpdateFunc: func(update event.UpdateEvent) bool {
		oldRoute, oldOK := update.ObjectOld.(*v1.Route)
		newRoute, newOK := update.ObjectNew.(*v1.Route)
		return oldOK && newOK && (!maps.Equal(oldRoute.Status.Properties, newRoute.Status.Properties) ||
			!readyConditionEqual(oldRoute.Status.Conditions, newRoute.Status.Conditions))
	}}
}

func consumerCutoverStateChanged() predicate.Predicate {
	return predicate.Funcs{UpdateFunc: func(update event.UpdateEvent) bool {
		oldConsumer, oldOK := update.ObjectOld.(*v1.Consumer)
		newConsumer, newOK := update.ObjectNew.(*v1.Consumer)
		return oldOK && newOK && (!maps.Equal(oldConsumer.Status.Properties, newConsumer.Status.Properties) ||
			!readyConditionEqual(oldConsumer.Status.Conditions, newConsumer.Status.Conditions))
	}}
}

func readyConditionEqual(oldConditions, newConditions []metav1.Condition) bool {
	oldReady := meta.FindStatusCondition(oldConditions, "Ready")
	newReady := meta.FindStatusCondition(newConditions, "Ready")
	if oldReady == nil || newReady == nil {
		return oldReady == nil && newReady == nil
	}
	return oldReady.Status == newReady.Status && oldReady.ObservedGeneration == newReady.ObservedGeneration
}

func emptyGatewayBundle(gateway *v1.Gateway, targetID, sourceKind string) (envoy.ResourceBundle, error) {
	target, err := parseTargetID(targetID)
	if err != nil {
		return envoy.ResourceBundle{}, err
	}
	return envoy.ResourceBundle{
		Target: target,
		Source: envoy.SourceMetadata{Resources: []envoy.SourceReference{{
			Kind: sourceKind, Namespace: gateway.Namespace, Name: gateway.Name,
			UID: gateway.UID, Generation: gateway.Generation,
		}}},
	}, nil
}

func targetID(target envoy.TargetIdentity) string {
	return strings.Join([]string{target.Environment, target.Namespace, target.Name, string(target.UID)}, "/")
}

func parseTargetID(value string) (envoy.TargetIdentity, error) {
	parts := strings.Split(value, "/")
	if len(parts) != 4 || parts[0] == "" || parts[1] == "" || parts[2] == "" || parts[3] == "" {
		return envoy.TargetIdentity{}, fmt.Errorf("invalid xDS target ID")
	}
	return envoy.TargetIdentity{
		Environment: parts[0], Namespace: parts[1], Name: parts[2], UID: k8stypes.UID(parts[3]),
	}, nil
}

func validateRecordedTarget(gateway *v1.Gateway, value string, checkEnvironment bool) error {
	target, err := parseTargetID(value)
	if err != nil {
		return err
	}
	if target.Namespace != gateway.Namespace || target.Name != gateway.Name || target.UID != gateway.UID {
		return fmt.Errorf("xDS target does not identify this Gateway")
	}
	if checkEnvironment && target.Environment != gateway.Labels[cconfig.EnvironmentLabelKey] {
		return fmt.Errorf("xDS target environment is immutable")
	}
	return nil
}

func (r *GatewayReconciler) mapRouteToGateway(_ context.Context, obj client.Object) []reconcile.Request {
	route, ok := obj.(*v1.Route)
	if !ok {
		return nil
	}
	return []reconcile.Request{{NamespacedName: route.Spec.GatewayRef.K8s()}}
}

func (r *GatewayReconciler) mapConsumerToGateway(_ context.Context, obj client.Object) []reconcile.Request {
	consumer, ok := obj.(*v1.Consumer)
	if !ok {
		return nil
	}
	return []reconcile.Request{{NamespacedName: consumer.Spec.Gateway.K8s()}}
}

func (r *GatewayReconciler) mapConsumeRouteToGateway(ctx context.Context, obj client.Object) []reconcile.Request {
	consumeRoute, ok := obj.(*v1.ConsumeRoute)
	if !ok {
		return nil
	}
	route := &v1.Route{}
	if err := r.Get(ctx, consumeRoute.Spec.Route.K8s(), route); err != nil {
		return nil
	}
	return r.mapRouteToGateway(ctx, route)
}

func (r *GatewayReconciler) compileGateway(ctx context.Context, req ctrl.Request) (*envoy.ResourceBundle, error) {
	gateway := &v1.Gateway{}
	if err := r.Get(ctx, req.NamespacedName, gateway); err != nil {
		return nil, client.IgnoreNotFound(err)
	}
	if gateway.Spec.Type != v1.GatewayTypeEnvoy {
		return nil, nil
	}
	ref := types.ObjectRefFromObject(gateway).String()
	routes := &v1.RouteList{}
	if err := r.List(ctx, routes, client.MatchingFields{IndexFieldSpecGatewayRef: ref}); err != nil {
		return nil, fmt.Errorf("listing Gateway routes: %w", err)
	}
	consumers := &v1.ConsumerList{}
	if err := r.List(ctx, consumers, client.MatchingFields{IndexFieldSpecGateway: ref}); err != nil {
		return nil, fmt.Errorf("listing Gateway consumers: %w", err)
	}
	consumeRoutes := &v1.ConsumeRouteList{}
	if err := r.List(ctx, consumeRoutes); err != nil {
		return nil, fmt.Errorf("listing Gateway consume routes: %w", err)
	}
	bundle, err := envoy.CompileGateway(ctx, &envoy.GatewayAggregate{
		Environment: gateway.Labels[cconfig.EnvironmentLabelKey], Gateway: gateway,
		Routes: routes.Items, Consumers: consumers.Items, ConsumeRoutes: consumeRoutes.Items,
	})
	if err != nil {
		return nil, fmt.Errorf("compiling Gateway aggregate: %w", err)
	}
	return &bundle, nil
}
