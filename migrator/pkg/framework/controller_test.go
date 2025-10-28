// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package framework

import (
	"context"
	"errors"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// Mock resource for testing
type MockResource struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              MockResourceSpec   `json:"spec,omitempty"`
	Status            MockResourceStatus `json:"status,omitempty"`
}

type MockResourceSpec struct {
	State string `json:"state,omitempty"`
}

type MockResourceStatus struct {
	Phase string `json:"phase,omitempty"`
}

func (m *MockResource) DeepCopyObject() runtime.Object {
	return &MockResource{
		TypeMeta:   m.TypeMeta,
		ObjectMeta: *m.ObjectMeta.DeepCopy(),
		Spec:       m.Spec,
		Status:     m.Status,
	}
}

// MockResourceList is a list of MockResource
type MockResourceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []MockResource `json:"items"`
}

func (m *MockResourceList) DeepCopyObject() runtime.Object {
	out := &MockResourceList{}
	out.TypeMeta = m.TypeMeta
	out.ListMeta = *m.ListMeta.DeepCopy()
	if m.Items != nil {
		out.Items = make([]MockResource, len(m.Items))
		for i := range m.Items {
			out.Items[i] = *m.Items[i].DeepCopyObject().(*MockResource)
		}
	}
	return out
}

// addMockResourceToScheme registers MockResource with the scheme
func addMockResourceToScheme(scheme *runtime.Scheme) error {
	scheme.AddKnownTypes(schema.GroupVersion{
		Group:   "test.framework.io",
		Version: "v1",
	}, &MockResource{}, &MockResourceList{})
	metav1.AddToGroupVersion(scheme, schema.GroupVersion{
		Group:   "test.framework.io",
		Version: "v1",
	})
	return nil
}

// Mock migrator for testing
type mockMigrator struct {
	name                    string
	computeIdentifierFunc   func(ctx context.Context, obj client.Object) (string, string, bool, error)
	fetchFunc               func(ctx context.Context, remoteClient client.Client, ns, name string) (client.Object, error)
	hasChangedFunc          func(ctx context.Context, current, legacy client.Object) bool
	applyMigrationFunc      func(ctx context.Context, current, legacy client.Object) error
	getRequeueAfterDuration time.Duration
}

func (m *mockMigrator) GetName() string {
	return m.name
}

func (m *mockMigrator) GetNewResourceType() client.Object {
	return &MockResource{}
}

func (m *mockMigrator) GetLegacyAPIGroup() string {
	return "legacy.test.io"
}

func (m *mockMigrator) ComputeLegacyIdentifier(ctx context.Context, obj client.Object) (string, string, bool, error) {
	if m.computeIdentifierFunc != nil {
		return m.computeIdentifierFunc(ctx, obj)
	}
	return "default", "test", false, nil
}

func (m *mockMigrator) FetchFromLegacy(ctx context.Context, remoteClient client.Client, namespace, name string) (client.Object, error) {
	if m.fetchFunc != nil {
		return m.fetchFunc(ctx, remoteClient, namespace, name)
	}
	return &MockResource{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		Spec:       MockResourceSpec{State: "migrated"},
	}, nil
}

func (m *mockMigrator) HasChanged(ctx context.Context, current, legacy client.Object) bool {
	if m.hasChangedFunc != nil {
		return m.hasChangedFunc(ctx, current, legacy)
	}
	return true
}

func (m *mockMigrator) ApplyMigration(ctx context.Context, current, legacy client.Object) error {
	if m.applyMigrationFunc != nil {
		return m.applyMigrationFunc(ctx, current, legacy)
	}
	currentMock := current.(*MockResource)
	legacyMock := legacy.(*MockResource)
	currentMock.Spec.State = legacyMock.Spec.State
	return nil
}

func (m *mockMigrator) GetRequeueAfter() time.Duration {
	if m.getRequeueAfterDuration > 0 {
		return m.getRequeueAfterDuration
	}
	return 30 * time.Second
}

var _ = Describe("GenericMigrationReconciler", func() {
	var (
		reconciler   *GenericMigrationReconciler
		mockResource *MockResource
		ctx          context.Context
		scheme       *runtime.Scheme
		migrator     *mockMigrator
	)

	BeforeEach(func() {
		ctx = context.Background()
		scheme = runtime.NewScheme()

		// Register MockResource with the scheme
		Expect(addMockResourceToScheme(scheme)).To(Succeed())

		mockResource = &MockResource{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-resource",
				Namespace: "default",
			},
			Spec: MockResourceSpec{State: "pending"},
		}

		migrator = &mockMigrator{
			name:                    "mock",
			getRequeueAfterDuration: 10 * time.Second,
		}
	})

	Describe("Reconcile", func() {
		Context("when resource exists and migration is successful", func() {
			It("should apply migration and update resource", func() {
				fakeClient := fake.NewClientBuilder().
					WithScheme(scheme).
					WithObjects(mockResource).
					Build()

				remoteClient := fake.NewClientBuilder().
					WithScheme(scheme).
					Build()

				reconciler = &GenericMigrationReconciler{
					Client:       fakeClient,
					Scheme:       scheme,
					RemoteClient: remoteClient,
					Migrator:     migrator,
					Log:          logr.Discard(),
				}

				req := ctrl.Request{
					NamespacedName: types.NamespacedName{
						Name:      "test-resource",
						Namespace: "default",
					},
				}

				result, err := reconciler.Reconcile(ctx, req)

				Expect(err).NotTo(HaveOccurred())
				Expect(result.RequeueAfter).To(Equal(10 * time.Second))

				// Verify resource was updated
				updated := &MockResource{}
				err = fakeClient.Get(ctx, req.NamespacedName, updated)
				Expect(err).NotTo(HaveOccurred())
				Expect(updated.Spec.State).To(Equal("migrated"))
			})
		})

		Context("when resource does not exist", func() {
			It("should not return error", func() {
				fakeClient := fake.NewClientBuilder().
					WithScheme(scheme).
					Build()

				remoteClient := fake.NewClientBuilder().
					WithScheme(scheme).
					Build()

				reconciler = &GenericMigrationReconciler{
					Client:       fakeClient,
					Scheme:       scheme,
					RemoteClient: remoteClient,
					Migrator:     migrator,
					Log:          logr.Discard(),
				}

				req := ctrl.Request{
					NamespacedName: types.NamespacedName{
						Name:      "non-existent",
						Namespace: "default",
					},
				}

				result, err := reconciler.Reconcile(ctx, req)

				Expect(err).NotTo(HaveOccurred())
				Expect(result.RequeueAfter).To(Equal(time.Duration(0)))
			})
		})

		Context("when ComputeLegacyIdentifier returns skip=true", func() {
			It("should skip migration", func() {
				migrator.computeIdentifierFunc = func(ctx context.Context, obj client.Object) (string, string, bool, error) {
					return "", "", true, nil
				}

				fakeClient := fake.NewClientBuilder().
					WithScheme(scheme).
					WithObjects(mockResource).
					Build()

				remoteClient := fake.NewClientBuilder().
					WithScheme(scheme).
					Build()

				reconciler = &GenericMigrationReconciler{
					Client:       fakeClient,
					Scheme:       scheme,
					RemoteClient: remoteClient,
					Migrator:     migrator,
					Log:          logr.Discard(),
				}

				req := ctrl.Request{
					NamespacedName: types.NamespacedName{
						Name:      "test-resource",
						Namespace: "default",
					},
				}

				result, err := reconciler.Reconcile(ctx, req)

				Expect(err).NotTo(HaveOccurred())
				Expect(result.RequeueAfter).To(Equal(10 * time.Second))

				// Verify resource was NOT updated
				updated := &MockResource{}
				err = fakeClient.Get(ctx, req.NamespacedName, updated)
				Expect(err).NotTo(HaveOccurred())
				Expect(updated.Spec.State).To(Equal("pending"))
			})
		})

		Context("when legacy resource not found", func() {
			It("should skip migration without error", func() {
				migrator.fetchFunc = func(ctx context.Context, remoteClient client.Client, ns, name string) (client.Object, error) {
					return nil, apierrors.NewNotFound(schema.GroupResource{
						Group:    "test.framework.io",
						Resource: "mockresources",
					}, "test")
				}

				fakeClient := fake.NewClientBuilder().
					WithScheme(scheme).
					WithObjects(mockResource).
					Build()

				remoteClient := fake.NewClientBuilder().
					WithScheme(scheme).
					Build()

				reconciler = &GenericMigrationReconciler{
					Client:       fakeClient,
					Scheme:       scheme,
					RemoteClient: remoteClient,
					Migrator:     migrator,
					Log:          logr.Discard(),
				}

				req := ctrl.Request{
					NamespacedName: types.NamespacedName{
						Name:      "test-resource",
						Namespace: "default",
					},
				}

				result, err := reconciler.Reconcile(ctx, req)

				Expect(err).NotTo(HaveOccurred())
				Expect(result.RequeueAfter).To(Equal(10 * time.Second))
			})
		})

		Context("when HasChanged returns false", func() {
			It("should skip update", func() {
				migrator.hasChangedFunc = func(ctx context.Context, current, legacy client.Object) bool {
					return false
				}

				fakeClient := fake.NewClientBuilder().
					WithScheme(scheme).
					WithObjects(mockResource).
					Build()

				remoteClient := fake.NewClientBuilder().
					WithScheme(scheme).
					Build()

				reconciler = &GenericMigrationReconciler{
					Client:       fakeClient,
					Scheme:       scheme,
					RemoteClient: remoteClient,
					Migrator:     migrator,
					Log:          logr.Discard(),
				}

				req := ctrl.Request{
					NamespacedName: types.NamespacedName{
						Name:      "test-resource",
						Namespace: "default",
					},
				}

				result, err := reconciler.Reconcile(ctx, req)

				Expect(err).NotTo(HaveOccurred())
				Expect(result.RequeueAfter).To(Equal(10 * time.Second))

				// Verify resource was NOT updated
				updated := &MockResource{}
				err = fakeClient.Get(ctx, req.NamespacedName, updated)
				Expect(err).NotTo(HaveOccurred())
				Expect(updated.Spec.State).To(Equal("pending"))
			})
		})

		Context("when ApplyMigration returns error", func() {
			It("should return error and requeue", func() {
				migrator.applyMigrationFunc = func(ctx context.Context, current, legacy client.Object) error {
					return errors.New("migration failed")
				}

				fakeClient := fake.NewClientBuilder().
					WithScheme(scheme).
					WithObjects(mockResource).
					Build()

				remoteClient := fake.NewClientBuilder().
					WithScheme(scheme).
					Build()

				reconciler = &GenericMigrationReconciler{
					Client:       fakeClient,
					Scheme:       scheme,
					RemoteClient: remoteClient,
					Migrator:     migrator,
					Log:          logr.Discard(),
				}

				req := ctrl.Request{
					NamespacedName: types.NamespacedName{
						Name:      "test-resource",
						Namespace: "default",
					},
				}

				result, err := reconciler.Reconcile(ctx, req)

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("migration failed"))
				Expect(result.RequeueAfter).To(Equal(10 * time.Second))
			})
		})
	})
})
