// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package runtime_test

import (
	"context"
	"time"

	"github.com/telekom/controlplane/projector/internal/runtime"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// --- mock SyncProcessor ---

type mockSyncProcessor struct {
	upsertErr error
	deleteErr error

	upsertCalled bool
	deleteCalled bool
}

func (m *mockSyncProcessor) Upsert(_ context.Context, _ *corev1.ConfigMap) error {
	m.upsertCalled = true
	return m.upsertErr
}

func (m *mockSyncProcessor) Delete(_ context.Context, _ types.NamespacedName, _ *corev1.ConfigMap) error {
	m.deleteCalled = true
	return m.deleteErr
}

// --- mock DeleteCacheReader ---

type mockDeleteCache struct {
	obj client.Object
}

func (m *mockDeleteCache) LoadAndDelete(_ client.ObjectKey) client.Object {
	return m.obj
}

// --- helpers ---

// switchableReader wraps a client.Reader and allows swapping the underlying
// reader at test time (e.g. to simulate an object disappearing for delete).
type switchableReader struct {
	inner client.Reader
}

func (s *switchableReader) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	return s.inner.Get(ctx, key, obj, opts...)
}

func (s *switchableReader) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	return s.inner.List(ctx, list, opts...)
}

func newConfigMap(name, ns string) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
		},
	}
}

func newFakeClient(objs ...client.Object) client.Client {
	return fake.NewClientBuilder().
		WithObjects(objs...).
		Build()
}

// buildReconciler creates a ReadOnlyReconciler with the given overrides.
// The returned randValue pointer can be changed to control the jitter output.
func buildReconciler(
	fakeClient client.Reader,
	proc *mockSyncProcessor,
	policy runtime.ErrorPolicy,
	randValue *float64,
) *runtime.ReadOnlyReconciler[*corev1.ConfigMap] {
	dc := &mockDeleteCache{}
	rec := runtime.NewReadOnlyReconciler(
		fakeClient,
		proc,
		dc,
		"test",
		func() *corev1.ConfigMap { return &corev1.ConfigMap{} },
		policy,
	)
	// Inject deterministic random for tests.
	rec.SetRandFloat64(func() float64 { return *randValue })
	return rec
}

// buildReconcilerWithDeleteCache is like buildReconciler but allows injecting
// a custom DeleteCacheReader (needed for delete-path tests).
func buildReconcilerWithDeleteCache(
	fakeClient client.Reader,
	proc *mockSyncProcessor,
	dc runtime.DeleteCacheReader,
	policy runtime.ErrorPolicy,
	randValue *float64,
) *runtime.ReadOnlyReconciler[*corev1.ConfigMap] {
	rec := runtime.NewReadOnlyReconciler(
		fakeClient,
		proc,
		dc,
		"test",
		func() *corev1.ConfigMap { return &corev1.ConfigMap{} },
		policy,
	)
	rec.SetRandFloat64(func() float64 { return *randValue })
	return rec
}

var _ = Describe("ReadOnlyReconciler", func() {
	var (
		ctx  context.Context
		proc *mockSyncProcessor
		req  ctrl.Request
	)

	BeforeEach(func() {
		ctx = context.Background()
		proc = &mockSyncProcessor{}
		req = ctrl.Request{
			NamespacedName: types.NamespacedName{Name: "test-cm", Namespace: "default"},
		}
	})

	Describe("success requeue (PeriodicResync)", func() {
		It("returns empty result when PeriodicResync is zero (event-driven)", func() {
			cm := newConfigMap("test-cm", "default")
			fc := newFakeClient(cm)
			randVal := 0.5
			rec := buildReconciler(fc, proc, runtime.ErrorPolicy{
				PeriodicResync:  0,
				SkipRequeue:     5 * time.Minute,
				DependencyDelay: 2 * time.Second,
			}, &randVal)

			result, err := rec.Reconcile(ctx, req)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(BeZero())
			Expect(result.Requeue).To(BeFalse())
		})

		It("applies ±20% jitter when PeriodicResync is non-zero", func() {
			cm := newConfigMap("test-cm", "default")
			fc := newFakeClient(cm)
			randVal := 0.0 // jitter factor = 0.8 + 0.0*0.4 = 0.8
			rec := buildReconciler(fc, proc, runtime.ErrorPolicy{
				PeriodicResync:  10 * time.Minute,
				SkipRequeue:     5 * time.Minute,
				DependencyDelay: 2 * time.Second,
			}, &randVal)

			result, err := rec.Reconcile(ctx, req)
			Expect(err).NotTo(HaveOccurred())
			// 10m * 0.8 = 8m
			Expect(result.RequeueAfter).To(Equal(8 * time.Minute))
		})

		It("computes upper bound of resync jitter", func() {
			cm := newConfigMap("test-cm", "default")
			fc := newFakeClient(cm)
			randVal := 1.0 // jitter factor = 0.8 + 1.0*0.4 = 1.2
			rec := buildReconciler(fc, proc, runtime.ErrorPolicy{
				PeriodicResync:  10 * time.Minute,
				SkipRequeue:     5 * time.Minute,
				DependencyDelay: 2 * time.Second,
			}, &randVal)

			result, err := rec.Reconcile(ctx, req)
			Expect(err).NotTo(HaveOccurred())
			// 10m * 1.2 = 12m
			Expect(result.RequeueAfter).To(Equal(12 * time.Minute))
		})

		It("computes midpoint resync jitter", func() {
			cm := newConfigMap("test-cm", "default")
			fc := newFakeClient(cm)
			randVal := 0.5 // jitter factor = 0.8 + 0.5*0.4 = 1.0
			rec := buildReconciler(fc, proc, runtime.ErrorPolicy{
				PeriodicResync:  10 * time.Minute,
				SkipRequeue:     5 * time.Minute,
				DependencyDelay: 2 * time.Second,
			}, &randVal)

			result, err := rec.Reconcile(ctx, req)
			Expect(err).NotTo(HaveOccurred())
			// 10m * 1.0 = 10m
			Expect(result.RequeueAfter).To(Equal(10 * time.Minute))
		})
	})

	Describe("dependency-missing jitter", func() {
		It("returns base delay without jitter when DependencyJitter is zero", func() {
			cm := newConfigMap("test-cm", "default")
			fc := newFakeClient(cm)
			proc.upsertErr = runtime.ErrDependencyMissing
			randVal := 0.5
			rec := buildReconciler(fc, proc, runtime.ErrorPolicy{
				DependencyDelay:  2 * time.Second,
				DependencyJitter: 0,
				SkipRequeue:      5 * time.Minute,
			}, &randVal)

			result, err := rec.Reconcile(ctx, req)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(Equal(2 * time.Second))
		})

		It("adds jitter to dependency delay", func() {
			cm := newConfigMap("test-cm", "default")
			fc := newFakeClient(cm)
			proc.upsertErr = runtime.ErrDependencyMissing
			randVal := 0.5 // jitter = 0.5 * 3s = 1.5s
			rec := buildReconciler(fc, proc, runtime.ErrorPolicy{
				DependencyDelay:  2 * time.Second,
				DependencyJitter: 3 * time.Second,
				SkipRequeue:      5 * time.Minute,
			}, &randVal)

			result, err := rec.Reconcile(ctx, req)
			Expect(err).NotTo(HaveOccurred())
			// 2s + 1.5s = 3.5s
			Expect(result.RequeueAfter).To(Equal(3500 * time.Millisecond))
		})

		It("returns base delay when rand is 0", func() {
			cm := newConfigMap("test-cm", "default")
			fc := newFakeClient(cm)
			proc.upsertErr = runtime.ErrDependencyMissing
			randVal := 0.0 // jitter = 0.0 * 3s = 0
			rec := buildReconciler(fc, proc, runtime.ErrorPolicy{
				DependencyDelay:  2 * time.Second,
				DependencyJitter: 3 * time.Second,
				SkipRequeue:      5 * time.Minute,
			}, &randVal)

			result, err := rec.Reconcile(ctx, req)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(Equal(2 * time.Second))
		})

		It("returns base+full jitter when rand is ~1", func() {
			cm := newConfigMap("test-cm", "default")
			fc := newFakeClient(cm)
			proc.upsertErr = runtime.ErrDependencyMissing
			randVal := 0.999 // jitter ≈ 3s
			rec := buildReconciler(fc, proc, runtime.ErrorPolicy{
				DependencyDelay:  2 * time.Second,
				DependencyJitter: 3 * time.Second,
				SkipRequeue:      5 * time.Minute,
			}, &randVal)

			result, err := rec.Reconcile(ctx, req)
			Expect(err).NotTo(HaveOccurred())
			// 2s + 0.999*3s ≈ 4.997s
			Expect(result.RequeueAfter).To(BeNumerically("~", 5*time.Second, 10*time.Millisecond))
		})
	})

	Describe("skip requeue", func() {
		It("requeues with SkipRequeue from policy", func() {
			cm := newConfigMap("test-cm", "default")
			fc := newFakeClient(cm)
			proc.upsertErr = runtime.ErrSkipSync
			randVal := 0.5
			rec := buildReconciler(fc, proc, runtime.ErrorPolicy{
				SkipRequeue:     10 * time.Minute,
				DependencyDelay: 2 * time.Second,
			}, &randVal)

			result, err := rec.Reconcile(ctx, req)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(Equal(10 * time.Minute))
		})
	})

	Describe("ErrorPolicy wiring", func() {
		It("uses the provided policy, not DefaultErrorPolicy", func() {
			cm := newConfigMap("test-cm", "default")
			fc := newFakeClient(cm)
			proc.upsertErr = runtime.ErrSkipSync
			randVal := 0.5
			customPolicy := runtime.ErrorPolicy{
				SkipRequeue:      42 * time.Second,
				DependencyDelay:  7 * time.Second,
				DependencyJitter: 1 * time.Second,
				PeriodicResync:   99 * time.Second,
			}
			rec := buildReconciler(fc, proc, customPolicy, &randVal)

			result, err := rec.Reconcile(ctx, req)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(Equal(42 * time.Second))
		})
	})
})
