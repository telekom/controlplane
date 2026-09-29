// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package listener

import (
	"context"
	"fmt"

	apiv1 "github.com/telekom/controlplane/api/api/v1"
	appv1 "github.com/telekom/controlplane/application/api/v1"
	approvalv1 "github.com/telekom/controlplane/approval/api/v1"
	cconfig "github.com/telekom/controlplane/common/pkg/config"
	cc "github.com/telekom/controlplane/common/pkg/controller"
	"github.com/telekom/controlplane/projector/internal/domain/approval"
	"github.com/telekom/controlplane/projector/internal/domain/approvalrequest"
	"github.com/telekom/controlplane/projector/internal/domain/shared"
	"github.com/telekom/controlplane/projector/internal/infrastructure"
	"github.com/telekom/controlplane/projector/internal/module"
	"github.com/telekom/controlplane/projector/internal/runtime"
	spectrev1 "github.com/telekom/controlplane/spectre/api/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

const applicationIndex = "spec.application"
const approvalTargetIndex = "spec.target.listener"
const observerIndex = "spec.application.objectRef"
const consumerIndex = "spec.consumer.owner"
const providerIndex = "spec.provider.owner"
const subscriptionIndex = "spec.consumer.subscription"
const exposureIndex = "spec.provider.exposure"

func ownerKey(team, name string) string      { return fmt.Sprintf("%q/%q", team, name) }
func edgeKey(team, name, path string) string { return fmt.Sprintf("%q/%q/%q", team, name, path) }

type listenerModule struct{}

var Module module.Module = listenerModule{}

func (listenerModule) Name() string { return "listener" }

func (listenerModule) Register(mgr ctrl.Manager, deps module.ModuleDeps) error {
	for _, index := range []struct {
		name string
		fn   client.IndexerFunc
	}{
		{consumerIndex, indexConsumer},
		{providerIndex, indexProvider},
		{subscriptionIndex, indexSubscription},
		{exposureIndex, indexExposure},
	} {
		if err := mgr.GetFieldIndexer().IndexField(context.Background(), &spectrev1.Listener{}, index.name, index.fn); err != nil {
			return fmt.Errorf("index listener %s: %w", index.name, err)
		}
	}
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &spectrev1.SpectreApplication{}, observerIndex, indexObserver); err != nil {
		return fmt.Errorf("index spectre application observer: %w", err)
	}
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &spectrev1.Listener{}, applicationIndex, func(obj client.Object) []string {
		listener := obj.(*spectrev1.Listener)
		ref := listener.Spec.Application
		if ref.Name == "" {
			return nil
		}
		if ref.Namespace == "" {
			ref.Namespace = listener.Namespace
		}
		return []string{ref.Namespace + "/" + ref.Name}
	}); err != nil {
		return fmt.Errorf("index listener application: %w", err)
	}
	for _, obj := range []client.Object{&approvalv1.Approval{}, &approvalv1.ApprovalRequest{}} {
		if err := mgr.GetFieldIndexer().IndexField(context.Background(), obj, approvalTargetIndex, func(obj client.Object) []string {
			var refNamespace, refName, kind string
			switch a := obj.(type) {
			case *approvalv1.Approval:
				refNamespace, refName, kind = a.Spec.Target.Namespace, a.Spec.Target.Name, a.Spec.Target.Kind
			case *approvalv1.ApprovalRequest:
				refNamespace, refName, kind = a.Spec.Target.Namespace, a.Spec.Target.Name, a.Spec.Target.Kind
			}
			if kind != "Listener" || refName == "" {
				return nil
			}
			if refNamespace == "" {
				refNamespace = obj.GetNamespace()
			}
			return []string{refNamespace + "/" + refName}
		}); err != nil {
			return fmt.Errorf("index %T listener target: %w", obj, err)
		}
	}

	repo := NewRepository(deps.EntClient, deps.IDResolver)
	translator := &Translator{Reader: mgr.GetClient(), OnMissingApplication: repo.Delete}
	proc := &listenerProcessor{
		processor:  runtime.NewProcessor[*spectrev1.Listener, *Data, Key](translator, repo),
		translator: translator, repo: repo,
		recovery: &approvalRecovery{reader: mgr.GetClient(),
			approvals: approval.NewRepository(deps.EntClient, deps.EdgeCache, deps.IDResolver),
			requests:  approvalrequest.NewRepository(deps.EntClient, deps.EdgeCache, deps.IDResolver)},
	}
	rec := runtime.NewReadOnlyReconciler(mgr.GetClient(), proc, deps.DeleteCache, "listener",
		func() *spectrev1.Listener { return &spectrev1.Listener{} }, runtime.NewErrorPolicyFromConfig(deps.Config))
	parentHandler := handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, obj client.Object) []reconcile.Request {
		return listenersForParent(ctx, mgr.GetClient(), obj)
	})

	return ctrl.NewControllerManagedBy(mgr).
		Named("projector-listener").
		Watches(&spectrev1.Listener{}, infrastructure.NewSyncEventHandler(deps.DeleteCache),
			builder.WithPredicates(cc.Count("projector-listener", cc.RoleWatches, predicate.ResourceVersionChangedPredicate{}))).
		Watches(&spectrev1.SpectreApplication{}, handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, obj client.Object) []reconcile.Request {
			return listenersForApplication(ctx, mgr.GetClient(), obj)
		}),
			builder.WithPredicates(cc.Count("projector-listener", cc.RoleWatches, predicate.ResourceVersionChangedPredicate{}))).
		Watches(&appv1.Application{}, parentHandler,
			builder.WithPredicates(cc.Count("projector-listener", cc.RoleWatches, predicate.ResourceVersionChangedPredicate{}))).
		Watches(&apiv1.ApiSubscription{}, parentHandler,
			builder.WithPredicates(cc.Count("projector-listener", cc.RoleWatches, predicate.ResourceVersionChangedPredicate{}))).
		Watches(&apiv1.ApiExposure{}, parentHandler,
			builder.WithPredicates(cc.Count("projector-listener", cc.RoleWatches, predicate.ResourceVersionChangedPredicate{}))).
		WatchesRawSource(source.Channel(deps.ParentEvents, parentHandler)).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: deps.Config.ConcurrencyFor("listener"),
			RateLimiter:             module.NewRateLimiter(deps.Config),
			ReconciliationTimeout:   deps.Config.ReconcileTimeout,
		}).Complete(rec)
}

func indexConsumer(obj client.Object) []string {
	ref := obj.(*spectrev1.Listener).Spec.Consumer
	if ref.Name == "" || ref.Namespace == "" {
		return nil
	}
	return []string{ownerKey(shared.TeamNameFromNamespace(ref.Namespace), ref.Name)}
}

func indexProvider(obj client.Object) []string {
	ref := obj.(*spectrev1.Listener).Spec.Provider
	if ref.Name == "" || ref.Namespace == "" {
		return nil
	}
	return []string{ownerKey(shared.TeamNameFromNamespace(ref.Namespace), ref.Name)}
}

func indexSubscription(obj client.Object) []string {
	l := obj.(*spectrev1.Listener)
	if l.Spec.ApiListener == nil || l.Spec.ApiListener.ApiBasePath == "" || len(indexConsumer(obj)) == 0 {
		return nil
	}
	return []string{edgeKey(shared.TeamNameFromNamespace(l.Spec.Consumer.Namespace), l.Spec.Consumer.Name, l.Spec.ApiListener.ApiBasePath)}
}

func indexExposure(obj client.Object) []string {
	l := obj.(*spectrev1.Listener)
	if l.Spec.ApiListener == nil || l.Spec.ApiListener.ApiBasePath == "" || len(indexProvider(obj)) == 0 {
		return nil
	}
	return []string{edgeKey(shared.TeamNameFromNamespace(l.Spec.Provider.Namespace), l.Spec.Provider.Name, l.Spec.ApiListener.ApiBasePath)}
}

func indexObserver(obj client.Object) []string {
	ref := obj.(*spectrev1.SpectreApplication).Spec.Application.ObjectRef
	if ref.Namespace == "" || ref.Name == "" {
		return nil
	}
	return []string{ref.Namespace + "/" + ref.Name}
}

func listenersForParent(ctx context.Context, reader client.Reader, obj client.Object) []reconcile.Request {
	var selectors []client.MatchingFields
	var requests []reconcile.Request
	switch parent := obj.(type) {
	case *apiv1.ApiSubscription:
		if parent.Spec.Requestor.Application.Name == "" || parent.Spec.ApiBasePath == "" {
			return nil
		}
		selectors = append(selectors, client.MatchingFields{subscriptionIndex: edgeKey(shared.TeamNameFromNamespace(parent.Namespace), parent.Spec.Requestor.Application.Name, parent.Spec.ApiBasePath)})
	case *apiv1.ApiExposure:
		appName := parent.Labels[cconfig.BuildLabelKey("application")]
		if appName == "" || parent.Spec.ApiBasePath == "" {
			return nil
		}
		selectors = append(selectors, client.MatchingFields{exposureIndex: edgeKey(shared.TeamNameFromNamespace(parent.Namespace), appName, parent.Spec.ApiBasePath)})
	case *appv1.Application:
		key := ownerKey(parent.Spec.Team, parent.Name)
		selectors = append(selectors, client.MatchingFields{consumerIndex: key}, client.MatchingFields{providerIndex: key})
		apps := &spectrev1.SpectreApplicationList{}
		if err := reader.List(ctx, apps, client.MatchingFields{observerIndex: parent.Namespace + "/" + parent.Name}); err != nil {
			log.FromContext(ctx).Error(err, "list spectre applications for observer", "application", client.ObjectKeyFromObject(parent))
		} else {
			for i := range apps.Items {
				requests = append(requests, listenersForApplication(ctx, reader, &apps.Items[i])...)
			}
		}
	default:
		return nil
	}
	for _, selector := range selectors {
		list := &spectrev1.ListenerList{}
		if err := reader.List(ctx, list, selector); err != nil {
			log.FromContext(ctx).Error(err, "list listeners for parent", "parent", client.ObjectKeyFromObject(obj), "selector", selector)
			continue
		}
		for i := range list.Items {
			requests = append(requests, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(&list.Items[i])})
		}
	}
	return requests
}

func listenersForApplication(ctx context.Context, reader client.Reader, obj client.Object) []reconcile.Request {
	key := obj.GetNamespace() + "/" + obj.GetName()
	list := &spectrev1.ListenerList{}
	if err := reader.List(ctx, list, client.MatchingFields{applicationIndex: key}); err != nil {
		log.FromContext(ctx).Error(err, "list listeners for spectre application", "application", key)
		return nil
	}
	requests := make([]reconcile.Request, 0, len(list.Items))
	for i := range list.Items {
		requests = append(requests, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(&list.Items[i])})
	}
	return requests
}
