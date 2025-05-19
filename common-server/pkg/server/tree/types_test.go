// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package tree_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/telekom/controlplane/common-server/pkg/server/tree"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func NewUnstructured(name string) *unstructured.Unstructured {
	u := &unstructured.Unstructured{}
	u.SetName(name)
	u.SetNamespace("default")
	return u
}

var _ = Describe("Tree Types", func() {

	Context("ResourceTree", func() {

		It("should create a new ResourceTree", func() {
			rt := tree.NewResourceTree()
			Expect(rt).ToNot(BeNil())
		})

		It("should set the root node", func() {
			rt := tree.NewResourceTree()
			rt.SetRoot(NewUnstructured("root"))
			Expect(rt.Root).ToNot(BeNil())
			Expect(rt.Root.Value.GetName()).To(Equal("root"))
			Expect(rt.GetCurrent()).To(Equal(rt.Root))
		})

		It("should add a new node", func() {
			rt := tree.NewResourceTree()
			rt.SetRoot(NewUnstructured("root"))
			rt.AddNewNode(NewUnstructured("child"))
			Expect(rt.Root.Children).To(HaveLen(1))
			Expect(rt.Root.Children[0].Value.GetName()).To(Equal("child"))
		})

		It("should replace the root node", func() {
			rt := tree.NewResourceTree()
			rt.SetRoot(NewUnstructured("root"))
			rt.AddNewNode(NewUnstructured("child"))
			rt.ReplaceRoot(NewUnstructured("new-root"))
			Expect(rt.Root.Children).To(HaveLen(1))
			Expect(rt.Root.Children[0].Value.GetName()).To(Equal("root"))
			Expect(rt.Root.Children[0].Children).To(HaveLen(1))
			Expect(rt.Root.Children[0].Children[0].Value.GetName()).To(Equal("child"))
		})

		It("should add a new child", func() {
			rt := tree.NewResourceTree()
			rt.SetRoot(NewUnstructured("root"))
			rt.Root.AddNewChild(NewUnstructured("child"))
			Expect(rt.Root.Children).To(HaveLen(1))
			Expect(rt.Root.Children[0].Value.GetName()).To(Equal("child"))
		})

		It("should add a new reference", func() {
			rt := tree.NewResourceTree()
			rt.SetRoot(NewUnstructured("root"))
			rt.Root.AddNewReference(NewUnstructured("ref"))
			Expect(rt.Root.References).To(HaveLen(1))
			Expect(rt.Root.References[0].Value.GetName()).To(Equal("ref"))
		})
	})
})
