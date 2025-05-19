// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package secrets_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/telekom/controlplane/common-server/pkg/server/middleware/security"
	"github.com/telekom/controlplane/common-server/pkg/store"
	"github.com/telekom/controlplane/common-server/pkg/store/secrets"
	"github.com/telekom/controlplane/common-server/test/mocks"
	"github.com/telekom/controlplane/secret-manager/api/fake"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func NewObject(namespace, name string) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{}
	obj.SetUnstructuredContent(map[string]any{
		"metadata": map[string]any{
			"name":      name,
			"namespace": namespace,
		},
		"spec": map[string]any{
			"secret": "$<my-secret-placeholder>",
		},
	})
	return obj
}

var _ = Describe("Secrets Store", func() {

	var ctx context.Context
	var mockStore *mocks.MockObjectStore[*unstructured.Unstructured]
	var secretManager *fake.MockSecretManager
	var secretsStore *secrets.SecretStore[*unstructured.Unstructured]

	BeforeEach(func() {
		ctx = context.Background()

		mockStore = mocks.NewMockObjectStore[*unstructured.Unstructured](GinkgoT())
		secretManager = fake.NewMockSecretManager(GinkgoT())
		secretsStore = secrets.WrapStore(mockStore, []string{"spec.secret"}, secrets.NewSecretManagerResolver(secretManager))
	})

	Context("WrapStore", func() {
		It("should wrap the store", func() {
			// Create a mock store
			mockStore := mocks.NewMockObjectStore[*unstructured.Unstructured](GinkgoT())

			// Wrap the store
			wrappedStore := secrets.WrapStore(mockStore, []string{"spec.secret"}, secrets.NewSecretManagerResolver(fake.NewMockSecretManager(GinkgoT())))

			// Check if the wrapped store is of the correct type
			Expect(wrappedStore).To(BeAssignableToTypeOf(&secrets.SecretStore[*unstructured.Unstructured]{}))
		})

	})

	Context("Get", func() {

		It("should replace secret values", func() {
			obj := NewObject("default", "foo")

			mockStore.EXPECT().Get(ctx, "default", "foo").Return(obj, nil)
			secretManager.EXPECT().Get(ctx, "my-secret-placeholder").Return("topsecret", nil)

			result, err := secretsStore.Get(ctx, "default", "foo")
			Expect(err).ToNot(HaveOccurred())
			Expect(result).ToNot(BeNil())

			Expect(result.UnstructuredContent()).To(Equal(map[string]any{
				"metadata": map[string]any{
					"name":      "foo",
					"namespace": "default",
				},
				"spec": map[string]any{
					"secret": "topsecret",
				},
			}))
		})

		It("should not replace secret values when the business-ctx is obfuscated", func() {
			bCtx := &security.BusinessContext{
				AccessType: security.AccessTypeObfuscated,
			}
			ctx = security.ToContext(ctx, bCtx)

			obj := NewObject("default", "foo")

			mockStore.EXPECT().Get(ctx, "default", "foo").Return(obj, nil)

			result, err := secretsStore.Get(ctx, "default", "foo")
			Expect(err).ToNot(HaveOccurred())
			Expect(result).ToNot(BeNil())

			Expect(result.UnstructuredContent()).To(Equal(map[string]any{
				"metadata": map[string]any{
					"name":      "foo",
					"namespace": "default",
				},
				"spec": map[string]any{
					"secret": "**********",
				},
			}))
		})

	})

	Context("List", func() {

		It("should replace secret values", func() {

			items := []*unstructured.Unstructured{
				NewObject("default", "foo"),
				NewObject("default", "bar"),
			}

			mockStore.EXPECT().List(ctx, store.ListOpts{}).Return(&store.ListResponse[*unstructured.Unstructured]{
				Items: items,
			}, nil)

			secretManager.EXPECT().Get(ctx, "my-secret-placeholder").Return("topsecret", nil).Times(2)

			result, err := secretsStore.List(ctx, store.ListOpts{})
			Expect(err).ToNot(HaveOccurred())
			Expect(result).ToNot(BeNil())
			Expect(result.Items).To(HaveLen(2))

			Expect(result.Items[0].UnstructuredContent()).To(Equal(map[string]any{
				"metadata": map[string]any{
					"name":      "foo",
					"namespace": "default",
				},
				"spec": map[string]any{
					"secret": "topsecret",
				},
			}))

			Expect(result.Items[1].UnstructuredContent()).To(Equal(map[string]any{
				"metadata": map[string]any{
					"name":      "bar",
					"namespace": "default",
				},
				"spec": map[string]any{
					"secret": "topsecret",
				},
			}))

		})

		It("should not replace secret values when the business-ctx is obfuscated", func() {
			bCtx := &security.BusinessContext{
				AccessType: security.AccessTypeObfuscated,
			}
			ctx = security.ToContext(ctx, bCtx)

			items := []*unstructured.Unstructured{
				NewObject("default", "foo"),
				NewObject("default", "bar"),
			}

			mockStore.EXPECT().List(ctx, store.ListOpts{}).Return(&store.ListResponse[*unstructured.Unstructured]{
				Items: items,
			}, nil)

			result, err := secretsStore.List(ctx, store.ListOpts{})
			Expect(err).ToNot(HaveOccurred())
			Expect(result).ToNot(BeNil())
			Expect(result.Items).To(HaveLen(2))

			Expect(result.Items[0].UnstructuredContent()).To(Equal(map[string]any{
				"metadata": map[string]any{
					"name":      "foo",
					"namespace": "default",
				},
				"spec": map[string]any{
					"secret": "**********",
				},
			}))

			Expect(result.Items[1].UnstructuredContent()).To(Equal(map[string]any{
				"metadata": map[string]any{
					"name":      "bar",
					"namespace": "default",
				},
				"spec": map[string]any{
					"secret": "**********",
				},
			}))
		})
	})
})
