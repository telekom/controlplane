// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package util

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	identityv1 "github.com/telekom/controlplane/identity/api/v1"
	"github.com/telekom/controlplane/identity/pkg/api"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestRealmUtil(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Keycloak Util Suite")
}

type attributeMap = map[string]interface{}

func attrs(m attributeMap) *attributeMap {
	return ptr.To(m)
}

var _ = Describe("Realm mapping", func() {
	Describe("MapToRealmRepresentation", func() {
		It("should not set attributes when the spec has none", func() {
			rep := MapToRealmRepresentation(&identityv1.Realm{ObjectMeta: metav1.ObjectMeta{Name: "r"}})
			Expect(rep.Attributes).To(BeNil())
			Expect(*rep.Realm).To(Equal("r"))
			Expect(*rep.Enabled).To(BeTrue())
		})

		It("should map spec attributes", func() {
			realm := &identityv1.Realm{
				ObjectMeta: metav1.ObjectMeta{Name: "r"},
				Spec:       identityv1.RealmSpec{Attributes: map[string]string{"frontendUrl": "https://iam.example.com/auth"}},
			}
			rep := MapToRealmRepresentation(realm)
			Expect(*rep.Attributes).To(Equal(map[string]interface{}{"frontendUrl": "https://iam.example.com/auth"}))
		})
	})

	Describe("ReconcileRealmAttributes", func() {
		It("should not manage attributes when nothing is desired or owned", func() {
			result, managed := ReconcileRealmAttributes(attributeMap{"manual": "x"}, nil, nil)
			Expect(managed).To(BeFalse())
			Expect(result).To(BeNil())
		})

		It("should overlay desired attributes on nil existing attributes", func() {
			result, managed := ReconcileRealmAttributes(nil, map[string]string{"a": "1"}, nil)
			Expect(managed).To(BeTrue())
			Expect(result).To(Equal(attributeMap{"a": "1"}))
		})

		It("should preserve unmanaged attributes and overlay desired ones", func() {
			existing := attributeMap{"manual": "x", "a": "old"}
			result, _ := ReconcileRealmAttributes(existing, map[string]string{"a": "new", "b": "2"}, nil)
			Expect(result).To(Equal(attributeMap{"manual": "x", "a": "new", "b": "2"}))

			By("not modifying the existing map")
			Expect(existing).To(Equal(attributeMap{"manual": "x", "a": "old"}))
		})

		It("should remove previously managed keys that are no longer desired", func() {
			existing := attributeMap{"manual": "x", "a": "1", "b": "2"}
			result, _ := ReconcileRealmAttributes(existing, map[string]string{"a": "1"}, []string{"a", "b"})
			Expect(result).To(Equal(attributeMap{"manual": "x", "a": "1"}))
		})

		It("should return an empty non-nil map when the last managed key is removed", func() {
			result, managed := ReconcileRealmAttributes(attributeMap{"a": "1"}, nil, []string{"a"})
			Expect(managed).To(BeTrue())
			Expect(result).ToNot(BeNil())
			Expect(result).To(BeEmpty())
		})

		It("should keep an empty string value instead of treating it as removal", func() {
			result, _ := ReconcileRealmAttributes(nil, map[string]string{"frontendUrl": ""}, nil)
			Expect(result).To(HaveKeyWithValue("frontendUrl", ""))
		})

		It("should be idempotent when applied repeatedly", func() {
			existing := attributeMap{"manual": "x", "a": "1", "b": "2"}
			desired := map[string]string{"a": "1"}
			first, _ := ReconcileRealmAttributes(existing, desired, []string{"a", "b"})
			second, _ := ReconcileRealmAttributes(first, desired, ManagedAttributeKeys(desired))
			Expect(second).To(Equal(first))
			Expect(CompareRealmRepresentation(
				&api.RealmRepresentation{Attributes: &first},
				&api.RealmRepresentation{Attributes: &second},
			)).To(BeTrue())
		})
	})

	Describe("ManagedAttributeKeys", func() {
		It("should return sorted keys", func() {
			Expect(ManagedAttributeKeys(map[string]string{"b": "", "a": "", "c": ""})).To(Equal([]string{"a", "b", "c"}))
		})

		It("should return nil for no attributes", func() {
			Expect(ManagedAttributeKeys(nil)).To(BeNil())
		})
	})

	Describe("Attributes", func() {
		It("should dereference optional maps", func() {
			Expect(Attributes(nil)).To(BeNil())
			Expect(Attributes(attrs(attributeMap{"a": "1"}))).To(Equal(attributeMap{"a": "1"}))
		})
	})

	Describe("RealmAttribute", func() {
		It("should return string values and ignore missing or non-string values", func() {
			a := attributeMap{"s": "v", "n": 1}
			Expect(RealmAttribute(a, "s")).To(Equal("v"))
			Expect(RealmAttribute(a, "n")).To(BeEmpty())
			Expect(RealmAttribute(a, "missing")).To(BeEmpty())
			Expect(RealmAttribute(nil, "s")).To(BeEmpty())
		})
	})

	Describe("CompareRealmRepresentation", func() {
		base := func(a *map[string]interface{}) *api.RealmRepresentation {
			return &api.RealmRepresentation{Realm: ptr.To("r"), Enabled: ptr.To(true), Attributes: a}
		}

		It("should ignore attributes when they are not desired", func() {
			Expect(CompareRealmRepresentation(base(attrs(map[string]interface{}{"x": "1"})), base(nil))).To(BeTrue())
		})

		It("should treat nil and empty existing attributes as equal to empty desired", func() {
			Expect(CompareRealmRepresentation(base(nil), base(attrs(map[string]interface{}{})))).To(BeTrue())
		})

		It("should detect changed, added and removed attributes", func() {
			existing := base(attrs(map[string]interface{}{"a": "1"}))
			Expect(CompareRealmRepresentation(existing, base(attrs(map[string]interface{}{"a": "2"})))).To(BeFalse())
			Expect(CompareRealmRepresentation(existing, base(attrs(map[string]interface{}{"a": "1", "b": "2"})))).To(BeFalse())
			Expect(CompareRealmRepresentation(existing, base(attrs(map[string]interface{}{})))).To(BeFalse())
			Expect(CompareRealmRepresentation(existing, base(attrs(map[string]interface{}{"b": "1"})))).To(BeFalse())
		})
	})

	Describe("MergeRealmRepresentation", func() {
		It("should keep existing attributes when none are desired", func() {
			existing := &api.RealmRepresentation{Attributes: attrs(map[string]interface{}{"x": "1"}), DisplayName: ptr.To("d")}
			merged := MergeRealmRepresentation(existing, &api.RealmRepresentation{Realm: ptr.To("r"), Enabled: ptr.To(true)})
			Expect(*merged.Attributes).To(Equal(map[string]interface{}{"x": "1"}))
			Expect(*merged.DisplayName).To(Equal("d"))
		})

		It("should replace attributes with the desired set", func() {
			existing := &api.RealmRepresentation{Attributes: attrs(map[string]interface{}{"x": "1"})}
			merged := MergeRealmRepresentation(existing, &api.RealmRepresentation{Attributes: attrs(map[string]interface{}{"y": "2"})})
			Expect(*merged.Attributes).To(Equal(map[string]interface{}{"y": "2"}))
		})
	})
})
