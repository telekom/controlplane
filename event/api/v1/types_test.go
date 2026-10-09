// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v1_test

import (
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	ctypes "github.com/telekom/controlplane/common/pkg/types"
	v1 "github.com/telekom/controlplane/event/api/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("EventTypeSpec CEL rules", func() {
	// The reserved-prefix rule is enforced by the API server at admission time
	// via a CEL XValidation marker on EventTypeSpec. We verify the rule is
	// present in the generated CRD YAML so a missing regeneration step does
	// not silently drop it.
	It("has a CEL rule rejecting the reserved de.telekom.ei.listener prefix", func() {
		crdBytes, err := os.ReadFile("../../config/crd/bases/event.cp.ei.telekom.de_eventtypes.yaml")
		Expect(err).ToNot(HaveOccurred(), "CRD YAML must be readable — run 'make manifests' first")

		crd := string(crdBytes)
		Expect(crd).To(ContainSubstring("de.telekom.ei.listener"),
			"generated CRD must contain the reserved-prefix validation rule")
		Expect(crd).To(ContainSubstring("!self.type.startsWith"),
			"generated CRD must contain the startsWith CEL expression")
	})
})

var _ = Describe("MakeEventTypeName", func() {
	DescribeTable("converts event type strings to Kubernetes resource names",
		func(input, expected string) {
			Expect(v1.MakeEventTypeName(input)).To(Equal(expected))
		},
		Entry("normal event type with dots",
			"de.telekom.eni.quickstart.v1",
			"de-telekom-eni-quickstart-v1",
		),
		Entry("already lowercase with no dots",
			"simple",
			"simple",
		),
		Entry("empty string",
			"",
			"",
		),
		Entry("mixed case with dots",
			"De.Telekom.V1",
			"de-telekom-v1",
		),
	)
})

var _ = Describe("EventConfig", func() {
	Describe("SupportsZone", func() {
		var config *v1.EventConfig

		BeforeEach(func() {
			config = &v1.EventConfig{
				Spec: v1.EventConfigSpec{
					Zone: ctypes.ObjectRef{
						Name:      "zone-a",
						Namespace: "default",
					},
					Mesh: &v1.MeshConfig{
						FullMesh:  false,
						ZoneNames: []string{"zone-b", "zone-c"},
					},
				},
			}
		})

		It("returns true for an exact zone match", func() {
			Expect(config.SupportsZone("zone-a")).To(BeTrue())
		})

		It("returns true when FullMesh is enabled even for a different zone", func() {
			config.Spec.Mesh.FullMesh = true
			Expect(config.SupportsZone("zone-x")).To(BeTrue())
		})

		It("returns true when the zone is in the ZoneNames list", func() {
			Expect(config.SupportsZone("zone-b")).To(BeTrue())
		})

		It("returns false when the zone is not in the list and FullMesh is false", func() {
			Expect(config.SupportsZone("zone-x")).To(BeFalse())
		})

		It("returns false with empty ZoneNames, FullMesh false, and a different zone", func() {
			config.Spec.Mesh.ZoneNames = nil
			Expect(config.SupportsZone("zone-x")).To(BeFalse())
		})

		It("returns true when Mesh is nil (defaults to full mesh)", func() {
			config.Spec.Mesh = nil
			Expect(config.SupportsZone("zone-x")).To(BeTrue())
		})
	})
})

var _ = Describe("NewObservedObjectRef", func() {
	It("returns nil for nil input", func() {
		// Pass an explicitly nil ctypes.Object interface value
		Expect(v1.NewObservedObjectRef(nil)).To(BeNil())
	})

	It("returns a correct ObservedObjectRef for a valid object", func() {
		obj := &v1.EventConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name:       "test",
				Namespace:  "default",
				Generation: 3,
			},
		}
		ref := v1.NewObservedObjectRef(obj)
		Expect(ref).ToNot(BeNil())
		Expect(ref.Name).To(Equal("test"))
		Expect(ref.Namespace).To(Equal("default"))
		Expect(ref.ObservedGeneration).To(Equal(int64(3)))
	})
})
