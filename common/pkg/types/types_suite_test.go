// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package types

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/scheme"
)

func TestTypes(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Types Suite")
}

var _ = Describe("ObjectRef", func() {

	Context("ObjectRef", func() {

		var ref = &ObjectRef{
			Name:      "test",
			Namespace: "test",
		}

		It("should successfully deep copy", func() {
			deepCopy := ref.DeepCopy()
			Expect(deepCopy).To(Equal(ref))

			deepCopy.Name = "test2"
			Expect(deepCopy).NotTo(Equal(ref))
		})

		It("should successfully deep copy into", func() {
			deepCopy := &ObjectRef{}
			ref.DeepCopyInto(deepCopy)
			Expect(deepCopy).To(Equal(ref))

			deepCopy.Name = "test2"
			Expect(deepCopy).NotTo(Equal(ref))
		})

		It("should successfully return string", func() {
			Expect(ref.String()).To(Equal("test/test"))
		})

		It("should returns the k8s objectkey", func() {
			Expect(ref.K8s().String()).To(Equal("test/test"))
		})

		It("should construct a new instance from object", func() {
			obj := unstructured.Unstructured{}
			obj.SetName("test")
			obj.SetNamespace("test")
			newRef := ObjectRefFromObject(&obj)
			Expect(newRef.Name).To(Equal("test"))
			Expect(newRef.Namespace).To(Equal("test"))
		})

		It("should successfully compare", func() {
			obj := &unstructured.Unstructured{}
			obj.SetName("test")
			obj.SetNamespace("test")

			Expect(ref.Equals(obj)).To(BeTrue())

			obj.SetName("test2")

			Expect(ref.Equals(obj)).To(BeFalse())
		})

		It("should successfully return false if empty", func() {
			ptrRef := &ObjectRef{}
			Expect(ptrRef.IsEmpty()).To(BeTrue())

			valueRef := ObjectRef{}
			Expect(valueRef.IsEmpty()).To(BeTrue())

			valueRef.Name = "test"
			Expect(valueRef.IsEmpty()).To(BeFalse())
			Expect(ptrRef.IsEmpty()).To(BeTrue())

		})

	})

	Context("TypedObjectRef", func() {

		var ref = &TypedObjectRef{
			TypeMeta: metav1.TypeMeta{
				Kind:       "Object",
				APIVersion: "testgroup.cp.ei.telekom.de/v1",
			},
			ObjectRef: ObjectRef{
				Name:      "test",
				Namespace: "test",
			},
		}

		It("should successfully deep copy", func() {
			deepCopy := ref.DeepCopy()
			Expect(deepCopy).To(Equal(ref))

			deepCopy.Name = "test2"
			Expect(deepCopy).NotTo(Equal(ref))
		})

		It("should successfully deep copy into", func() {
			deepCopy := &TypedObjectRef{}
			ref.DeepCopyInto(deepCopy)
			Expect(deepCopy).To(Equal(ref))

			deepCopy.Name = "test2"
			Expect(deepCopy).NotTo(Equal(ref))
		})

		It("should successfully return string", func() {

			Expect(ref.String()).To(Equal("testgroup.cp.ei.telekom.de/v1/Object:test/test"))
		})

		It("should construct a new instance from object", func() {
			obj := unstructured.Unstructured{}
			obj.SetName("test")
			obj.SetNamespace("test")
			obj.SetGroupVersionKind(schema.GroupVersionKind{
				Group:   "testgroup.cp.ei.telekom.de",
				Version: "v1",
				Kind:    "Object",
			})

			gv := schema.GroupVersion{
				Group:   "testgroup.cp.ei.telekom.de",
				Version: "v1",
			}
			scheme.Scheme.AddKnownTypes(gv, &unstructured.Unstructured{}, &unstructured.UnstructuredList{})
			metav1.AddToGroupVersion(scheme.Scheme, gv)

			newRef := TypedObjectRefFromObject(&obj, scheme.Scheme)
			Expect(newRef.Name).To(Equal("test"))
			Expect(newRef.Namespace).To(Equal("test"))
			Expect(newRef.Kind).To(Equal("Object"))
			Expect(newRef.APIVersion).To(Equal("testgroup.cp.ei.telekom.de/v1"))
		})

		It("should successfully compare", func() {
			obj := &unstructured.Unstructured{}
			obj.SetName("test")
			obj.SetNamespace("test")
			obj.SetGroupVersionKind(schema.GroupVersionKind{
				Group:   "testgroup.cp.ei.telekom.de",
				Version: "v1",
				Kind:    "Object",
			})

			Expect(ref.Equals(obj)).To(BeTrue())

			obj.SetKind("test2")

			Expect(ref.Equals(obj)).To(BeFalse())

		})

	})

	Context("Equals", func() {

		It("should successfully compare", func() {
			obj1 := &unstructured.Unstructured{}
			obj1.SetName("test")
			obj1.SetNamespace("test")
			obj1.SetGroupVersionKind(schema.GroupVersionKind{
				Group:   "testgroup.cp.ei.telekom.de",
				Version: "v1",
				Kind:    "Object",
			})

			obj2 := &unstructured.Unstructured{}
			obj2.SetName("test")
			obj2.SetNamespace("test")
			obj2.SetGroupVersionKind(schema.GroupVersionKind{
				Group:   "testgroup.cp.ei.telekom.de",
				Version: "v1",
				Kind:    "Object",
			})

			Expect(Equals(obj1, obj2)).To(BeTrue())

			obj2.SetGroupVersionKind(schema.GroupVersionKind{
				Group:   "testgroup.cp.ei.telekom.de",
				Version: "v1",
				Kind:    "Object2",
			})

			Expect(Equals(obj1, obj2)).To(BeFalse())
		})

		It("should successfully compare with nil", func() {
			By("setting up objects")
			obj1 := &unstructured.Unstructured{}
			obj1.SetName("test")
			obj1.SetNamespace("test")
			obj1.SetGroupVersionKind(schema.GroupVersionKind{
				Group:   "testgroup.cp.ei.telekom.de",
				Version: "v1",
				Kind:    "Object",
			})

			obj2 := &unstructured.Unstructured{}
			obj2.SetName("test")
			obj2.SetNamespace("test")

			By("checking with nil GVK")
			Expect(Equals(obj1, obj2)).To(BeFalse())

			By("checking with empty GVK")
			obj1.SetGroupVersionKind(schema.GroupVersionKind{})
			Expect(Equals(obj1, obj2)).To(BeTrue())

			By("checking with nil objects")
			Expect(Equals(nil, obj2)).To(BeFalse())
			Expect(Equals(obj1, nil)).To(BeFalse())
			Expect(Equals(nil, nil)).To(BeTrue())

		})
	})
})
