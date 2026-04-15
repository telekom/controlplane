// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package zone_test

import (
	"context"

	"entgo.io/ent/privacy"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl_runtime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	_ "github.com/mattn/go-sqlite3"
	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	"github.com/telekom/controlplane/controlplane-api/ent"
	"github.com/telekom/controlplane/controlplane-api/ent/enttest"
	_ "github.com/telekom/controlplane/controlplane-api/ent/runtime"

	"github.com/telekom/controlplane/projector/internal/domain/zone"
	"github.com/telekom/controlplane/projector/internal/infrastructure"
	runtime "github.com/telekom/controlplane/projector/internal/runtime"
)

var _ = Describe("Zone Integration", func() {
	var (
		entClient   *ent.Client
		edgeCache   *infrastructure.EdgeCache
		deleteCache *infrastructure.DeleteCache
		reconciler  *runtime.ReadOnlyReconciler[*adminv1.Zone]
		ctx         context.Context
		scheme      *ctrl_runtime.Scheme
	)

	BeforeEach(func() {
		ctx = privacy.DecisionContext(context.Background(), privacy.Allow)

		var err error
		edgeCache, err = infrastructure.NewEdgeCache(100_000, 10<<20, 64)
		Expect(err).NotTo(HaveOccurred())

		entClient = enttest.Open(GinkgoT(), "sqlite3", "file:ent?mode=memory&_fk=1")
		deleteCache = &infrastructure.DeleteCache{}

		scheme = ctrl_runtime.NewScheme()
		Expect(adminv1.AddToScheme(scheme)).To(Succeed())
	})

	AfterEach(func() {
		_ = entClient.Close()
		edgeCache.Close()
	})

	// buildReconciler constructs the full pipeline with the given fake client objects.
	buildReconciler := func(objs ...ctrl_runtime.Object) {
		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithRuntimeObjects(objs...).
			Build()

		translator := &zone.Translator{}
		repo := zone.NewRepository(entClient, edgeCache)
		processor := runtime.NewProcessor[*adminv1.Zone, *zone.ZoneData, zone.ZoneKey](translator, repo)

		reconciler = runtime.NewReadOnlyReconciler(
			fakeClient,
			processor,
			deleteCache,
			"zone",
			func() *adminv1.Zone { return &adminv1.Zone{} },
			runtime.ErrorPolicy{}, // zero policy: no periodic requeue, no jitter
		)
	}

	Describe("Upsert", func() {
		It("should create a Zone in the database when CR exists", func() {
			zoneObj := &adminv1.Zone{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "integration-zone",
					Namespace: "admin",
					Labels: map[string]string{
						"cp.ei.telekom.de/environment": "test",
					},
				},
				Spec: adminv1.ZoneSpec{
					Visibility: adminv1.ZoneVisibilityWorld,
					Gateway: adminv1.GatewayConfig{
						Url: "https://gw.integration.test",
					},
				},
			}
			buildReconciler(zoneObj)

			result, err := reconciler.Reconcile(ctx, ctrl.Request{
				NamespacedName: types.NamespacedName{Name: "integration-zone", Namespace: "admin"},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(BeZero()) // requeueAfter is 0 in test

			// Verify DB state.
			z, err := entClient.Zone.Query().Only(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(z.Name).To(Equal("integration-zone"))
			Expect(z.GatewayURL).NotTo(BeNil())
			Expect(*z.GatewayURL).To(Equal("https://gw.integration.test"))
			Expect(string(z.Visibility)).To(Equal("WORLD"))
		})
	})

	Describe("Update", func() {
		It("should update DB when CR is modified", func() {
			// First reconcile: create.
			zoneObj := &adminv1.Zone{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "update-zone",
					Namespace: "admin",
				},
				Spec: adminv1.ZoneSpec{
					Visibility: adminv1.ZoneVisibilityWorld,
					Gateway: adminv1.GatewayConfig{
						Url: "https://gw1.test",
					},
				},
			}
			buildReconciler(zoneObj)

			_, err := reconciler.Reconcile(ctx, ctrl.Request{
				NamespacedName: types.NamespacedName{Name: "update-zone", Namespace: "admin"},
			})
			Expect(err).NotTo(HaveOccurred())

			// Second reconcile: update with different values.
			updatedZone := &adminv1.Zone{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "update-zone",
					Namespace: "admin",
				},
				Spec: adminv1.ZoneSpec{
					Visibility: adminv1.ZoneVisibilityEnterprise,
					Gateway:    adminv1.GatewayConfig{}, // empty URL -> nil
				},
			}
			buildReconciler(updatedZone)

			_, err = reconciler.Reconcile(ctx, ctrl.Request{
				NamespacedName: types.NamespacedName{Name: "update-zone", Namespace: "admin"},
			})
			Expect(err).NotTo(HaveOccurred())

			// Verify updated DB state.
			z, err := entClient.Zone.Query().Only(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(z.GatewayURL).To(BeNil())
			Expect(string(z.Visibility)).To(Equal("ENTERPRISE"))
		})
	})

	Describe("Delete", func() {
		It("should remove Zone from DB when CR is deleted", func() {
			// Setup: create zone in DB via reconcile.
			zoneObj := &adminv1.Zone{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "delete-zone",
					Namespace: "admin",
				},
				Spec: adminv1.ZoneSpec{
					Visibility: adminv1.ZoneVisibilityEnterprise,
				},
			}
			buildReconciler(zoneObj)

			_, err := reconciler.Reconcile(ctx, ctrl.Request{
				NamespacedName: types.NamespacedName{Name: "delete-zone", Namespace: "admin"},
			})
			Expect(err).NotTo(HaveOccurred())

			// Verify zone exists.
			count, err := entClient.Zone.Query().Count(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(count).To(Equal(1))

			// Simulate delete: rebuild with no objects, store last-known in cache.
			deleteCache.Store(zoneObj)
			buildReconciler() // empty fake client

			_, err = reconciler.Reconcile(ctx, ctrl.Request{
				NamespacedName: types.NamespacedName{Name: "delete-zone", Namespace: "admin"},
			})
			Expect(err).NotTo(HaveOccurred())

			// Verify zone is removed from DB.
			count, err = entClient.Zone.Query().Count(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(count).To(Equal(0))
		})
	})

	Describe("Skip (non-existent CR, no cache entry)", func() {
		It("should not fail when CR does not exist and no cache entry", func() {
			buildReconciler() // empty fake client, no delete cache entry

			result, err := reconciler.Reconcile(ctx, ctrl.Request{
				NamespacedName: types.NamespacedName{Name: "ghost-zone", Namespace: "admin"},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(BeZero())
		})
	})
})
