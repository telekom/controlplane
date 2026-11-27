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
	ctypes "github.com/telekom/controlplane/common/pkg/types"
	"github.com/telekom/controlplane/controlplane-api/ent"
	entexposure "github.com/telekom/controlplane/controlplane-api/ent/apiexposure"
	"github.com/telekom/controlplane/controlplane-api/ent/enttest"
	entlistener "github.com/telekom/controlplane/controlplane-api/ent/listener"
	_ "github.com/telekom/controlplane/controlplane-api/ent/runtime"
	"github.com/telekom/controlplane/controlplane-api/ent/zone"
	"github.com/telekom/controlplane/controlplane-api/pkg/model"
	"github.com/telekom/controlplane/projector/internal/domain/shared"
	"github.com/telekom/controlplane/projector/internal/infrastructure"
	projectorruntime "github.com/telekom/controlplane/projector/internal/runtime"
	spectrev1 "github.com/telekom/controlplane/spectre/api/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
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

	It("links observer, consumer, provider applications along with subscription and exposure", func() {
		Expect(repo.Upsert(ctx, data)).To(Succeed())
		row := client.Listener.Query().OnlyX(ctx)
		Expect(row.QueryApplication().OnlyX(ctx).ID).To(Equal(observer.ID))
		Expect(row.QueryConsumer().OnlyX(ctx).ID).To(Equal(consumer.ID))
		Expect(row.QueryProvider().OnlyX(ctx).ID).To(Equal(provider.ID))
		Expect(row.QuerySubscription().OnlyX(ctx).ID).To(Equal(sub.ID))
		Expect(row.QueryExposure().OnlyX(ctx).QueryOwner().OnlyX(ctx).ID).To(Equal(provider.ID))
		Expect(observer.QueryListeners().CountX(ctx)).To(Equal(1))
		Expect(consumer.QueryListeners().CountX(ctx)).To(Equal(0))
	})

	It("retargets all edges and clears filters on conflict", func() {
		data.RequestFilter = &model.ListenerFilter{Trigger: map[string]any{"method": "GET"}}
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
		Expect(row.QueryConsumer().OnlyX(ctx).ID).To(Equal(otherConsumer.ID))
		Expect(row.QueryProvider().OnlyX(ctx).ID).To(Equal(otherProvider.ID))
		Expect(row.QuerySubscription().OnlyX(ctx).QueryOwner().OnlyX(ctx).ID).To(Equal(otherConsumer.ID))
		Expect(row.QueryExposure().OnlyX(ctx).QueryOwner().OnlyX(ctx).ID).To(Equal(otherProvider.ID))
	})

	It("persists a blocked Listener with nil subscription or exposure and links them once present", func() {
		// Remove subscription first — Listener should still persist with nil subscription and requeue.
		client.ApiSubscription.DeleteOne(sub).ExecX(ctx)
		resolver.EvictAPISubscriptionID(data.BasePath, consumer.Name, "consumer-team")
		data.StatusPhase = "PENDING"
		data.StatusMessage = "blocked: no ApiSubscription for consumer"
		Expect(projectorruntime.IsDependencyMissing(repo.Upsert(ctx, data))).To(BeTrue())

		row := client.Listener.Query().OnlyX(ctx)
		Expect(row.StatusPhase).To(HaveValue(Equal(entlistener.StatusPhasePending)))
		Expect(row.StatusMessage).To(HaveValue(Equal("blocked: no ApiSubscription for consumer")))
		Expect(row.QueryApplication().OnlyX(ctx).ID).To(Equal(observer.ID))
		Expect(row.QueryConsumer().OnlyX(ctx).ID).To(Equal(consumer.ID))
		Expect(row.QueryProvider().OnlyX(ctx).ID).To(Equal(provider.ID))
		Expect(row.QuerySubscription().ExistX(ctx)).To(BeFalse())
		Expect(row.QueryExposure().ExistX(ctx)).To(BeTrue())

		// Also remove exposure — both optional edges become nil while row stays visible.
		client.ApiExposure.Delete().Where(entexposure.HasOwnerWith()).ExecX(ctx)
		resolver.EvictAPIExposureID(data.BasePath, provider.Name, "provider-team")
		data.StatusMessage = "blocked: no ApiSubscription or ApiExposure"
		Expect(projectorruntime.IsDependencyMissing(repo.Upsert(ctx, data))).To(BeTrue())

		row = client.Listener.Query().OnlyX(ctx)
		Expect(row.StatusMessage).To(HaveValue(Equal("blocked: no ApiSubscription or ApiExposure")))
		Expect(row.QuerySubscription().ExistX(ctx)).To(BeFalse())
		Expect(row.QueryExposure().ExistX(ctx)).To(BeFalse())

		// Create subscription and exposure — next upsert links both edges and succeeds.
		newSub := client.ApiSubscription.Create().SetBasePath(data.BasePath).
			SetNamespace(consumer.Namespace).SetName("consumer-sub-new").SetOwnerID(consumer.ID).SaveX(ctx)
		newExposure := client.ApiExposure.Create().SetBasePath(data.BasePath).
			SetNamespace(provider.Namespace).SetVisibility(entexposure.VisibilityWorld).
			SetActive(true).SetFeatures([]string{}).SetOwnerID(provider.ID).SaveX(ctx)
		data.StatusPhase = "READY"
		data.StatusMessage = "provisioned"
		Expect(repo.Upsert(ctx, data)).To(Succeed())

		row = client.Listener.Query().OnlyX(ctx)
		Expect(row.StatusPhase).To(HaveValue(Equal(entlistener.StatusPhaseReady)))
		Expect(row.QuerySubscription().OnlyX(ctx).ID).To(Equal(newSub.ID))
		Expect(row.QueryExposure().OnlyX(ctx).ID).To(Equal(newExposure.ID))
	})

	It("clears subscription and exposure FKs on conflict when retargeted to a path without them", func() {
		Expect(repo.Upsert(ctx, data)).To(Succeed())
		row := client.Listener.Query().OnlyX(ctx)
		Expect(row.QuerySubscription().ExistX(ctx)).To(BeTrue())
		Expect(row.QueryExposure().ExistX(ctx)).To(BeTrue())

		data.BasePath = "/unsubscribed-api"
		data.StatusPhase = "PENDING"
		data.StatusMessage = "blocked: waiting for subscription and exposure"
		Expect(projectorruntime.IsDependencyMissing(repo.Upsert(ctx, data))).To(BeTrue())

		row = client.Listener.Query().OnlyX(ctx)
		Expect(row.APIBasePath).To(Equal("/unsubscribed-api"))
		Expect(row.StatusPhase).To(HaveValue(Equal(entlistener.StatusPhasePending)))
		Expect(row.QuerySubscription().ExistX(ctx)).To(BeFalse())
		Expect(row.QueryExposure().ExistX(ctx)).To(BeFalse())
	})

	It("retries without inserting when observer, consumer, or provider Application is missing", func() {
		data.Consumer.Name = "missing"
		Expect(projectorruntime.IsDependencyMissing(repo.Upsert(ctx, data))).To(BeTrue())
		Expect(client.Listener.Query().CountX(ctx)).To(BeZero())
		data.Consumer = Key{Namespace: consumer.Namespace, Name: consumer.Name}
		data.Provider.Name = "missing"
		Expect(projectorruntime.IsDependencyMissing(repo.Upsert(ctx, data))).To(BeTrue())
		Expect(client.Listener.Query().CountX(ctx)).To(BeZero())
		data.Provider = Key{Namespace: provider.Namespace, Name: provider.Name}
		data.Observer.Name = "missing"
		Expect(projectorruntime.IsDependencyMissing(repo.Upsert(ctx, data))).To(BeTrue())
		Expect(client.Listener.Query().CountX(ctx)).To(BeZero())
	})

	It("keeps previous edges when retargeting waits for a missing Application", func() {
		Expect(repo.Upsert(ctx, data)).To(Succeed())
		data.Observer = Key{Namespace: observer.Namespace, Name: "missing"}
		Expect(projectorruntime.IsDependencyMissing(repo.Upsert(ctx, data))).To(BeTrue())
		row := client.Listener.Query().OnlyX(ctx)
		Expect(row.QueryApplication().OnlyX(ctx).ID).To(Equal(observer.ID))
		Expect(row.QueryConsumer().OnlyX(ctx).ID).To(Equal(consumer.ID))
		Expect(row.QueryProvider().OnlyX(ctx).ID).To(Equal(provider.ID))
		Expect(row.QuerySubscription().OnlyX(ctx).ID).To(Equal(sub.ID))
		Expect(row.QueryExposure().OnlyX(ctx).QueryOwner().OnlyX(ctx).ID).To(Equal(provider.ID))
	})

	It("deletes by Kubernetes identity", func() {
		Expect(repo.Upsert(ctx, data)).To(Succeed())
		Expect(repo.Delete(ctx, Key{Namespace: data.Meta.Namespace, Name: data.Meta.Name})).To(Succeed())
		Expect(repo.Delete(ctx, Key{Namespace: data.Meta.Namespace, Name: data.Meta.Name})).To(Succeed())
		Expect(client.Listener.Query().CountX(ctx)).To(BeZero())
	})

	It("removes stale projections when a Listener is skipped", func() {
		scheme := runtime.NewScheme()
		Expect(spectrev1.AddToScheme(scheme)).To(Succeed())
		envLabels := map[string]string{shared.EnvironmentLabelKey: "prod"}
		app := &spectrev1.SpectreApplication{ObjectMeta: metav1.ObjectMeta{Namespace: data.Meta.Namespace, Name: "spectre-app", Labels: envLabels},
			Spec: spectrev1.SpectreApplicationSpec{Application: ctypes.TypedObjectRef{ObjectRef: ctypes.ObjectRef{Namespace: observer.Namespace, Name: observer.Name}}}}
		reader := fake.NewClientBuilder().WithScheme(scheme).WithObjects(app).Build()
		obj := &spectrev1.Listener{ObjectMeta: metav1.ObjectMeta{Namespace: data.Meta.Namespace, Name: data.Meta.Name, Labels: envLabels},
			Spec: spectrev1.ListenerSpec{Application: ctypes.ObjectRef{Name: app.Name},
				Consumer:    ctypes.TypedObjectRef{ObjectRef: ctypes.ObjectRef{Namespace: consumer.Namespace, Name: consumer.Name}},
				Provider:    ctypes.TypedObjectRef{ObjectRef: ctypes.ObjectRef{Namespace: provider.Namespace, Name: provider.Name}},
				ApiListener: &spectrev1.ApiListener{ApiBasePath: data.BasePath}}}
		translator := &Translator{Reader: reader}
		proc := &listenerProcessor{processor: projectorruntime.NewProcessor[*spectrev1.Listener, *Data, Key](translator, repo),
			translator: translator, repo: repo}
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
})
