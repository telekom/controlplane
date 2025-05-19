package tree_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/telekom/controlplane/common-server/pkg/server/tree"
	"github.com/telekom/controlplane/common-server/pkg/store"
	"github.com/telekom/controlplane/common-server/test/mocks"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var _ = Describe("Tree", Ordered, func() {

	True := true
	ctx := context.Background()
	var fooStore *mocks.MockObjectStore[*unstructured.Unstructured]
	var barStore *mocks.MockObjectStore[*unstructured.Unstructured]
	var bazStore *mocks.MockObjectStore[*unstructured.Unstructured]

	BeforeEach(func() {
		fooStore = NewMockStore("Foo")
		barStore = NewMockStore("Bar")
		bazStore = NewMockStore("Baz")
		tree.LookupStores.AddStore(fooStore)
		tree.LookupStores.AddStore(barStore)
		tree.LookupStores.AddStore(bazStore)
	})

	Context("GetTree", func() {
		It("should return a tree with just the root object", func() {
			fooStore.EXPECT().Get(ctx, "default", "foo").Return(&unstructured.Unstructured{}, nil)

			rt, err := tree.GetTree(ctx, fooStore, "default", "foo", 1)
			Expect(err).ToNot(HaveOccurred())
			Expect(rt).NotTo(BeNil())
			Expect(rt.Root).NotTo(BeNil())
			Expect(rt.Root.Children).To(BeEmpty())
		})

		It("should return a tree with the root object and its children", func() {

			// Given

			fooObject := NewUnstructured("foo0")
			fooObject.SetUID("1234")
			fooObject.SetGroupVersionKind(schema.GroupVersionKind{Group: "testgroup", Version: "v1", Kind: "Foo"})
			childOfFoo := tree.TreeResourceInfo{
				APIVersion: "testgroup/v1",
				Kind:       "Bar",
			}
			tree.LookupResourceHierarchy.AddChild(fooObject, childOfFoo)

			fooStore.EXPECT().Get(ctx, "default", "foo0").Return(fooObject, nil)

			barObject := NewUnstructured("bar0")
			barObject.SetGroupVersionKind(schema.GroupVersionKind{Group: "testgroup", Version: "v1", Kind: "Bar"})
			barObject.SetOwnerReferences([]metav1.OwnerReference{
				{
					APIVersion: "testgroup/v1",
					Kind:       "Foo",
					Name:       "foo0",
					UID:        "1234",
					Controller: &True,
				},
			})
			expectedListOpts := store.ListOpts{
				Limit: 100,
				Filters: []store.Filter{
					{
						Path:  "metadata.ownerReferences.#.uid",
						Op:    store.OpEqual,
						Value: "1234",
					},
				},
			}
			barStore.EXPECT().List(ctx, expectedListOpts).Return(&store.ListResponse[*unstructured.Unstructured]{Items: []*unstructured.Unstructured{barObject}}, nil)
			barStore.EXPECT().Get(ctx, "default", "bar0").Return(barObject, nil)

			// When

			rt, err := tree.GetTree(ctx, fooStore, "default", "foo0", 2)

			// Then

			Expect(err).ToNot(HaveOccurred())
			Expect(rt).NotTo(BeNil())
			Expect(rt.Root).NotTo(BeNil())
			Expect(rt.Root.Children).To(HaveLen(1))
			Expect(rt.Root.Children[0].Children).To(BeEmpty())

		})

		It("should return a tree with the root object and its children and grandchildren", func() {

			// Given

			// -- Foo

			fooObject := NewUnstructured("foo0")
			fooObject.SetUID("1234")
			fooObject.SetGroupVersionKind(schema.GroupVersionKind{Group: "testgroup", Version: "v1", Kind: "Foo"})
			childOfFoo := tree.TreeResourceInfo{
				APIVersion: "testgroup/v1",
				Kind:       "Bar",
			}
			tree.LookupResourceHierarchy.AddChild(fooObject, childOfFoo)

			fooStore.EXPECT().Get(ctx, "default", "foo0").Return(fooObject, nil)

			// -- Bar

			barObject := NewUnstructured("bar0")
			barObject.SetUID("5678")
			barObject.SetGroupVersionKind(schema.GroupVersionKind{Group: "testgroup", Version: "v1", Kind: "Bar"})
			barObject.SetOwnerReferences([]metav1.OwnerReference{
				{
					APIVersion: "testgroup/v1",
					Kind:       "Foo",
					Name:       "foo0",
					UID:        "1234",
					Controller: &True,
				},
			})
			childOfBar := tree.TreeResourceInfo{
				APIVersion: "testgroup/v1",
				Kind:       "Baz",
			}
			tree.LookupResourceHierarchy.AddChild(barObject, childOfBar)

			expectedBarListOpts := store.ListOpts{
				Limit: 100,
				Filters: []store.Filter{
					{
						Path:  "metadata.ownerReferences.#.uid",
						Op:    store.OpEqual,
						Value: "1234",
					},
				},
			}
			barStore.EXPECT().List(ctx, expectedBarListOpts).Return(&store.ListResponse[*unstructured.Unstructured]{Items: []*unstructured.Unstructured{barObject}}, nil)
			barStore.EXPECT().Get(ctx, "default", "bar0").Return(barObject, nil)

			// -- Baz

			bazObject := NewUnstructured("baz0")
			bazObject.SetGroupVersionKind(schema.GroupVersionKind{Group: "testgroup", Version: "v1", Kind: "Baz"})
			bazObject.SetOwnerReferences([]metav1.OwnerReference{
				{
					APIVersion: "testgroup/v1",
					Kind:       "Bar",
					Name:       "bar0",
					UID:        "5678",
					Controller: &True,
				},
			})
			expectedBazListOpts := store.ListOpts{
				Limit: 100,
				Filters: []store.Filter{
					{
						Path:  "metadata.ownerReferences.#.uid",
						Op:    store.OpEqual,
						Value: "5678",
					},
				},
			}
			bazStore.EXPECT().List(ctx, expectedBazListOpts).Return(&store.ListResponse[*unstructured.Unstructured]{Items: []*unstructured.Unstructured{bazObject}}, nil)
			bazStore.EXPECT().Get(ctx, "default", "baz0").Return(bazObject, nil)

			// When

			rt, err := tree.GetTree(ctx, barStore, "default", "bar0", 3)

			// Then

			Expect(err).ToNot(HaveOccurred())
			Expect(rt).NotTo(BeNil())
			Expect(rt.Root).NotTo(BeNil())
			Expect(rt.Root.Value.GetName()).To(Equal("foo0"))
			Expect(rt.Root.Children).To(HaveLen(1))
			Expect(rt.Root.Children[0].Children).To(HaveLen(1))
			Expect(rt.Root.Children[0].Children[0].Value.GetName()).To(Equal("baz0"))
			Expect(rt.Root.Children[0].Children[0].Children).To(BeEmpty())
		})
	})
})
