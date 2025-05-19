// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package tree_test

import (
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/telekom/controlplane/common-server/pkg/server/tree"
	"github.com/telekom/controlplane/common-server/test/mocks"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func NewMockStore(kind string) *mocks.MockObjectStore[*unstructured.Unstructured] {
	s := mocks.NewMockObjectStore[*unstructured.Unstructured](GinkgoT())
	gvr := schema.GroupVersionResource{
		Group:    "testgroup",
		Version:  "v1",
		Resource: strings.ToLower(kind) + "s",
	}
	gvk := schema.GroupVersionKind{
		Group:   "testgroup",
		Version: "v1",
		Kind:    kind,
	}

	s.EXPECT().Info().Return(gvr, gvk)

	return s
}

var _ = Describe("Storeutil", func() {

	Context("LookupStores", func() {

		It("should add a store", func() {

			fooStore := NewMockStore("Foo")
			tree.LookupStores.AddStore(fooStore)

			barStore := NewMockStore("Bar")
			tree.LookupStores.AddStore(barStore)

			actualFooStore, ok := tree.LookupStores.GetStore("testgroup/v1", "Foo")
			Expect(ok).To(BeTrue())
			Expect(actualFooStore).To(Equal(fooStore))

			actualBarStore, ok := tree.LookupStores.GetStore("testgroup/v1", "Bar")
			Expect(ok).To(BeTrue())
			Expect(actualBarStore).To(Equal(barStore))
		})
	})

	Context("GetControllerOf", func() {

		It("should returns false if the object has no owner reference", func() {
			obj := &unstructured.Unstructured{}
			ref, ok := tree.GetControllerOf(obj)
			Expect(ok).To(BeFalse())
			Expect(ref).To(Equal(tree.OwnerReference{}))
		})

		It("should returns the OwnerRefernce of the provided object", func() {
			obj := &unstructured.Unstructured{}
			t := true
			obj.SetOwnerReferences([]metav1.OwnerReference{
				{
					APIVersion: "testgroup/v1",
					Kind:       "TestObject",
					Name:       "foo",
					UID:        "123",
					Controller: &t,
				},
			})

			ref, ok := tree.GetControllerOf(obj)
			Expect(ok).To(BeTrue())
			Expect(ref).To(Equal(tree.OwnerReference{
				ApiVersion: "testgroup/v1",
				Kind:       "TestObject",
				Name:       "foo",
				Namespace:  "",
				Uid:        "123",
			}))

			Expect(ref.GetAPIVersion()).To(Equal("testgroup/v1"))
			Expect(ref.GetKind()).To(Equal("TestObject"))
		})
	})
})
