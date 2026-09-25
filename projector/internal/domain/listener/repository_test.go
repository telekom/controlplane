// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package listener

import (
	"context"

	"entgo.io/ent/privacy"
	_ "github.com/mattn/go-sqlite3"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	approvalv1 "github.com/telekom/controlplane/approval/api/v1"
	cconfig "github.com/telekom/controlplane/common/pkg/config"
	ctypes "github.com/telekom/controlplane/common/pkg/types"
	"github.com/telekom/controlplane/controlplane-api/ent"
	entexposure "github.com/telekom/controlplane/controlplane-api/ent/apiexposure"
	"github.com/telekom/controlplane/controlplane-api/ent/enttest"
	entlistener "github.com/telekom/controlplane/controlplane-api/ent/listener"
	_ "github.com/telekom/controlplane/controlplane-api/ent/runtime"
	"github.com/telekom/controlplane/controlplane-api/ent/zone"
	"github.com/telekom/controlplane/controlplane-api/pkg/model"
	"github.com/telekom/controlplane/projector/internal/domain/approval"
	"github.com/telekom/controlplane/projector/internal/domain/approvalrequest"
	"github.com/telekom/controlplane/projector/internal/domain/shared"
	"github.com/telekom/controlplane/projector/internal/infrastructure"
	projectorruntime "github.com/telekom/controlplane/projector/internal/runtime"
	spectrev1 "github.com/telekom/controlplane/spectre/api/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

var _ = Describe("Listener repository", func() {
	var (
		ctx                                                        context.Context
		client                                                     *ent.Client
		cache                                                      *infrastructure.EdgeCache
		resolver                                                   *infrastructure.IDResolver
		repo                                                       *Repository
		data                                                       *Data
		observer, consumer, provider, otherConsumer, otherProvider *ent.Application
		sub                                                        *ent.ApiSubscription
	)
	BeforeEach(func() {
		ctx = privacy.DecisionContext(context.Background(), privacy.Allow)
		var err error
		cache, err = infrastructure.NewEdgeCache(100_000, 10<<20, 64)
		Expect(err).NotTo(HaveOccurred())
		client = enttest.Open(GinkgoT(), "sqlite3", "file:listener?mode=memory&_fk=1")
		resolver = infrastructure.NewIDResolver(client, cache, infrastructure.WithNegativeCacheTTL(0))
		repo = NewRepository(client, resolver)
		z := client.Zone.Create().SetName("zone").SetVisibility(zone.VisibilityEnterprise).SaveX(ctx)
		seedApp := func(team, name string) *ent.Application {
			t := client.Team.Create().SetName(team).SetNamespace(team).SetEmail(team + "@example.com").SaveX(ctx)
			return client.Application.Create().SetName(name).SetNamespace("prod--" + team).SetOwnerTeamID(t.ID).SetZoneID(z.ID).SaveX(ctx)
		}
		observer = seedApp("observer-team", "observer")
		consumer = seedApp("consumer-team", "consumer")
		provider = seedApp("provider-team", "provider")
		otherConsumer = seedApp("other-consumer", "consumer")
		otherProvider = seedApp("other-provider", "provider")
		seedSub := func(app *ent.Application) *ent.ApiSubscription {
			return client.ApiSubscription.Create().SetBasePath("/api").SetNamespace(app.Namespace).SetName(app.Name + "-sub").SetOwnerID(app.ID).SaveX(ctx)
		}
		sub = seedSub(consumer)
		seedSub(otherConsumer)
		seedExposure := func(app *ent.Application) {
			client.ApiExposure.Create().SetBasePath("/api").SetNamespace(app.Namespace).SetVisibility(entexposure.VisibilityWorld).
				SetActive(true).SetFeatures([]string{}).SetOwnerID(app.ID).SaveX(ctx)
		}
		seedExposure(provider)
		seedExposure(otherProvider)
		data = &Data{Meta: shared.NewMetadata("prod--observer-team", "listener", nil), StatusPhase: "READY",
			BasePath: "/api", Observer: Key{Namespace: observer.Namespace, Name: observer.Name},
			Consumer: Key{Namespace: consumer.Namespace, Name: consumer.Name},
			Provider: Key{Namespace: provider.Namespace, Name: provider.Name}}
	})
	AfterEach(func() {
		Expect(client.Close()).To(Succeed())
		cache.Close()
	})

	It("links exactly the observer, consumer subscription and provider exposure", func() {
		Expect(repo.Upsert(ctx, data)).To(Succeed())
		row := client.Listener.Query().OnlyX(ctx)
		Expect(row.QueryApplication().OnlyX(ctx).ID).To(Equal(observer.ID))
		Expect(row.QuerySubscription().OnlyX(ctx).ID).To(Equal(sub.ID))
		Expect(row.QueryExposure().OnlyX(ctx).QueryOwner().OnlyX(ctx).ID).To(Equal(provider.ID))
		Expect(observer.QueryListeners().CountX(ctx)).To(Equal(1))
		Expect(consumer.QueryListeners().CountX(ctx)).To(Equal(0))
	})

	It("retargets all edges and clears filters on conflict", func() {
		data.RequestFilter = &model.ListenerFilter{Trigger: map[string]string{"method": "GET"}}
		Expect(repo.Upsert(ctx, data)).To(Succeed())
		id := client.Listener.Query().OnlyX(ctx).ID
		data.Observer = Key{Namespace: consumer.Namespace, Name: consumer.Name}
		data.Consumer = Key{Namespace: otherConsumer.Namespace, Name: otherConsumer.Name}
		data.Provider = Key{Namespace: otherProvider.Namespace, Name: otherProvider.Name}
		data.StatusPhase = "ERROR"
		data.RequestFilter = nil
		Expect(repo.Upsert(ctx, data)).To(Succeed())
		row := client.Listener.Query().Where(entlistener.IDEQ(id)).OnlyX(ctx)
		Expect(row.StatusPhase).NotTo(BeNil())
		Expect(*row.StatusPhase).To(Equal(entlistener.StatusPhaseError))
		Expect(row.RequestFilter).To(BeNil())
		Expect(row.QueryApplication().OnlyX(ctx).ID).To(Equal(consumer.ID))
		Expect(row.QuerySubscription().OnlyX(ctx).QueryOwner().OnlyX(ctx).ID).To(Equal(otherConsumer.ID))
		Expect(row.QueryExposure().OnlyX(ctx).QueryOwner().OnlyX(ctx).ID).To(Equal(otherProvider.ID))
	})

	It("retries instead of adopting a different owner's dependency", func() {
		data.Consumer.Name = "missing"
		Expect(projectorruntime.IsDependencyMissing(repo.Upsert(ctx, data))).To(BeTrue())
		Expect(client.Listener.Query().CountX(ctx)).To(BeZero())
		data.Consumer = Key{Namespace: consumer.Namespace, Name: consumer.Name}
		data.Provider.Name = "missing"
		Expect(projectorruntime.IsDependencyMissing(repo.Upsert(ctx, data))).To(BeTrue())
		data.Provider = Key{Namespace: provider.Namespace, Name: provider.Name}
		data.Observer.Name = "missing"
		Expect(projectorruntime.IsDependencyMissing(repo.Upsert(ctx, data))).To(BeTrue())
	})

	It("removes stale edges when retargeting waits for a dependency", func() {
		Expect(repo.Upsert(ctx, data)).To(Succeed())
		data.Observer = Key{Namespace: observer.Namespace, Name: "missing"}
		Expect(projectorruntime.IsDependencyMissing(repo.Upsert(ctx, data))).To(BeTrue())
		Expect(client.Listener.Query().CountX(ctx)).To(BeZero())
	})

	It("deletes by Kubernetes identity", func() {
		Expect(repo.Upsert(ctx, data)).To(Succeed())
		Expect(repo.Delete(ctx, Key{Namespace: data.Meta.Namespace, Name: data.Meta.Name})).To(Succeed())
		Expect(repo.Delete(ctx, Key{Namespace: data.Meta.Namespace, Name: data.Meta.Name})).To(Succeed())
		Expect(client.Listener.Query().CountX(ctx)).To(BeZero())
	})

	It("removes stale projections when a Listener is skipped or its reference becomes incomplete", func() {
		scheme := runtime.NewScheme()
		Expect(spectrev1.AddToScheme(scheme)).To(Succeed())
		Expect(approvalv1.AddToScheme(scheme)).To(Succeed())
		app := &spectrev1.SpectreApplication{ObjectMeta: metav1.ObjectMeta{Namespace: data.Meta.Namespace, Name: "spectre-app"},
			Spec: spectrev1.SpectreApplicationSpec{Application: ctypes.TypedObjectRef{ObjectRef: ctypes.ObjectRef{Namespace: observer.Namespace, Name: observer.Name}}}}
		reader := fake.NewClientBuilder().WithScheme(scheme).WithObjects(app).
			WithIndex(&approvalv1.Approval{}, approvalTargetIndex, func(obj kclient.Object) []string { return nil }).
			WithIndex(&approvalv1.ApprovalRequest{}, approvalTargetIndex, func(obj kclient.Object) []string { return nil }).Build()
		obj := &spectrev1.Listener{ObjectMeta: metav1.ObjectMeta{Namespace: data.Meta.Namespace, Name: data.Meta.Name},
			Spec: spectrev1.ListenerSpec{Application: ctypes.ObjectRef{Name: app.Name},
				Consumer:    ctypes.TypedObjectRef{ObjectRef: ctypes.ObjectRef{Namespace: consumer.Namespace, Name: consumer.Name}},
				Provider:    ctypes.TypedObjectRef{ObjectRef: ctypes.ObjectRef{Namespace: provider.Namespace, Name: provider.Name}},
				ApiListener: &spectrev1.ApiListener{ApiBasePath: data.BasePath}}}
		translator := &Translator{Reader: reader, OnMissingApplication: repo.Delete}
		proc := &listenerProcessor{processor: projectorruntime.NewProcessor[*spectrev1.Listener, *Data, Key](translator, repo),
			translator: translator, repo: repo, recovery: &approvalRecovery{reader: reader}}
		Expect(proc.Upsert(ctx, obj)).To(Succeed())
		obj.Spec.Consumer.Namespace = ""
		Expect(projectorruntime.IsSkipSync(proc.Upsert(ctx, obj))).To(BeTrue())
		Expect(client.Listener.Query().CountX(ctx)).To(BeZero())
		obj.Spec.Consumer.Namespace = consumer.Namespace
		Expect(proc.Upsert(ctx, obj)).To(Succeed())
		obj.Spec.EventListener = &spectrev1.EventListener{EventType: "event"}
		Expect(projectorruntime.IsSkipSync(proc.Upsert(ctx, obj))).To(BeTrue())
		Expect(client.Listener.Query().CountX(ctx)).To(BeZero())
	})

	It("restores cascaded and late approvals after Listener recreation", func() {
		cconfig.SetFeatureEnabled(cconfig.FeatureSpectre, true)
		DeferCleanup(cconfig.SetFeatureEnabled, cconfig.FeatureSpectre, false)
		scheme := runtime.NewScheme()
		Expect(spectrev1.AddToScheme(scheme)).To(Succeed())
		Expect(approvalv1.AddToScheme(scheme)).To(Succeed())
		target := ctypes.TypedObjectRef{ObjectRef: ctypes.ObjectRef{Namespace: data.Meta.Namespace, Name: data.Meta.Name},
			TypeMeta: metav1.TypeMeta{Kind: "Listener"}}
		makeApproval := func(name, key string) *approvalv1.Approval {
			return &approvalv1.Approval{ObjectMeta: metav1.ObjectMeta{Namespace: "approvals", Name: name},
				Spec: approvalv1.ApprovalSpec{Target: target, ApprovalKey: key, Action: "listen-" + key, Strategy: approvalv1.ApprovalStrategy("Auto"),
					State: approvalv1.ApprovalState("Granted"), Decider: approvalv1.Decider{TeamName: "provider-team"}}}
		}
		makeRequest := func(name string) *approvalv1.ApprovalRequest {
			return &approvalv1.ApprovalRequest{ObjectMeta: metav1.ObjectMeta{Namespace: "approvals", Name: name},
				Spec: approvalv1.ApprovalRequestSpec{Target: target, ApprovalKey: "provider", Action: "listen-provider",
					Strategy: approvalv1.ApprovalStrategy("Auto"), State: approvalv1.ApprovalState("Pending"),
					Decider: approvalv1.Decider{TeamName: "provider-team"}}}
		}
		app := &spectrev1.SpectreApplication{ObjectMeta: metav1.ObjectMeta{Namespace: data.Meta.Namespace, Name: "spectre-app"},
			Spec: spectrev1.SpectreApplicationSpec{Application: ctypes.TypedObjectRef{ObjectRef: ctypes.ObjectRef{Namespace: observer.Namespace, Name: observer.Name}}}}
		reader := fake.NewClientBuilder().WithScheme(scheme).WithObjects(app, makeApproval("provider", "provider"), makeRequest("request")).
			WithIndex(&approvalv1.Approval{}, approvalTargetIndex, func(obj kclient.Object) []string { return []string{data.Meta.Namespace + "/" + data.Meta.Name} }).
			WithIndex(&approvalv1.ApprovalRequest{}, approvalTargetIndex, func(obj kclient.Object) []string { return []string{data.Meta.Namespace + "/" + data.Meta.Name} }).Build()
		listed := &approvalv1.ApprovalList{}
		Expect(reader.List(ctx, listed, kclient.MatchingFields{approvalTargetIndex: data.Meta.Namespace + "/" + data.Meta.Name})).To(Succeed())
		Expect(listed.Items).To(HaveLen(1))
		skip, reason := (&approval.Translator{}).ShouldSkip(&listed.Items[0])
		Expect(skip).To(BeFalse(), reason)
		obj := &spectrev1.Listener{ObjectMeta: metav1.ObjectMeta{Namespace: data.Meta.Namespace, Name: data.Meta.Name},
			Spec: spectrev1.ListenerSpec{Application: ctypes.ObjectRef{Name: app.Name},
				Consumer:    ctypes.TypedObjectRef{ObjectRef: ctypes.ObjectRef{Namespace: consumer.Namespace, Name: consumer.Name}},
				Provider:    ctypes.TypedObjectRef{ObjectRef: ctypes.ObjectRef{Namespace: provider.Namespace, Name: provider.Name}},
				ApiListener: &spectrev1.ApiListener{ApiBasePath: data.BasePath}}}
		translator := &Translator{Reader: reader, OnMissingApplication: repo.Delete}
		proc := &listenerProcessor{processor: projectorruntime.NewProcessor[*spectrev1.Listener, *Data, Key](translator, repo),
			translator: translator, repo: repo, recovery: &approvalRecovery{reader: reader,
				approvals: approval.NewRepository(client, cache, resolver), requests: approvalrequest.NewRepository(client, cache, resolver)}}
		Expect(proc.Upsert(ctx, obj)).To(Succeed())
		Expect(client.Approval.Query().CountX(ctx)).To(Equal(1))
		Expect(client.ApprovalRequest.Query().CountX(ctx)).To(Equal(1))
		Expect(repo.Delete(ctx, Key{Namespace: obj.Namespace, Name: obj.Name})).To(Succeed())
		Expect(client.Approval.Query().CountX(ctx)).To(BeZero())
		Expect(client.ApprovalRequest.Query().CountX(ctx)).To(BeZero())
		Expect(reader.Create(ctx, makeApproval("consumer", "consumer"))).To(Succeed())
		Expect(proc.Upsert(ctx, obj)).To(Succeed())
		Expect(client.Approval.Query().CountX(ctx)).To(Equal(2))
		Expect(client.ApprovalRequest.Query().CountX(ctx)).To(Equal(1))
		Expect(proc.Upsert(ctx, obj)).To(Succeed())
		Expect(client.Approval.Query().CountX(ctx)).To(Equal(2))
		Expect(client.Listener.Query().OnlyX(ctx).QueryProviderApproval().OnlyX(ctx).Name).To(Equal("provider"))
		Expect(client.Listener.Query().OnlyX(ctx).QueryConsumerApproval().OnlyX(ctx).Name).To(Equal("consumer"))
		obj.Spec.EventListener = &spectrev1.EventListener{EventType: "unsupported"}
		Expect(projectorruntime.IsSkipSync(proc.Upsert(ctx, obj))).To(BeTrue())
		Expect(client.Listener.Query().CountX(ctx)).To(BeZero())
		obj.Spec.EventListener = nil
		Expect(proc.Upsert(ctx, obj)).To(Succeed())
		Expect(client.Approval.Query().CountX(ctx)).To(Equal(2))

		Expect(client.ApiSubscription.DeleteOneID(sub.ID).Exec(ctx)).To(Succeed())
		Expect(client.Listener.Query().CountX(ctx)).To(BeZero())
		Expect(client.Approval.Query().CountX(ctx)).To(BeZero())
		Expect(client.ApprovalRequest.Query().CountX(ctx)).To(BeZero())
		sub = client.ApiSubscription.Create().SetBasePath(data.BasePath).SetNamespace(consumer.Namespace).
			SetName("consumer-sub").SetOwnerID(consumer.ID).SaveX(ctx)
		resolver.EvictAPISubscriptionID(data.BasePath, consumer.Name, shared.TeamNameFromNamespace(consumer.Namespace))
		Expect(proc.Upsert(ctx, obj)).To(Succeed())
		Expect(client.Listener.Query().OnlyX(ctx).QuerySubscription().OnlyX(ctx).ID).To(Equal(sub.ID))
		Expect(client.Approval.Query().CountX(ctx)).To(Equal(2))
		Expect(client.ApprovalRequest.Query().CountX(ctx)).To(Equal(1))
	})
})
