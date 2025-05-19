// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package tree

import (
	"github.com/telekom/controlplane/common-server/pkg/store"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

type ResourceTree struct {
	Root    *ResourceNode `json:"root,omitempty"`
	current *ResourceNode `json:"-"`
}

func NewResourceTree() *ResourceTree {
	return &ResourceTree{}
}

func (t *ResourceTree) GetCurrent() *ResourceNode {
	return t.current
}

func (t *ResourceTree) SetCurrent(node *ResourceNode) {
	t.current = node
}

func (t *ResourceTree) SetRoot(obj store.Object) {
	t.Root = &ResourceNode{
		Value: obj,
	}

	t.SetCurrent(t.Root)
}

func (t *ResourceTree) AddNode(node *ResourceNode) {
	t.current.AddChild(node)
}

func (t *ResourceTree) AddNewNode(obj store.Object) *ResourceNode {
	node := &ResourceNode{
		Value: obj,
	}
	t.AddNode(node)
	return node
}

func (t *ResourceTree) ReplaceRoot(obj store.Object) *ResourceNode {
	tmp := t.Root
	t.SetRoot(obj)
	t.Root.AddChild(tmp)
	return t.Root
}

type ResourceNode struct {
	Value      store.Object    `json:"value,omitempty"`
	Children   []*ResourceNode `json:"owns,omitempty"`
	References []*ResourceNode `json:"references,omitempty"`
}

func (n *ResourceNode) AddChild(child *ResourceNode) {
	n.Children = append(n.Children, child)
}

func (n *ResourceNode) AddNewChild(obj store.Object) *ResourceNode {
	node := &ResourceNode{
		Value: obj,
	}
	n.AddChild(node)
	return node
}

func (n *ResourceNode) AddReference(ref *ResourceNode) {
	n.References = append(n.References, ref)
}

func (n *ResourceNode) AddNewReference(obj *unstructured.Unstructured) *ResourceNode {
	node := &ResourceNode{
		Value: obj,
	}
	n.AddReference(node)
	return node
}

type GVK interface {
	GetAPIVersion() string
	GetKind() string
}

var _ GVK = OwnerReference{}

type OwnerReference struct {
	ApiVersion string `json:"apiVersion"`
	Kind       string `json:"kind"`
	Name       string `json:"name"`
	Namespace  string `json:"namespace"`
	Uid        string `json:"uid"`
}

func (o OwnerReference) GetAPIVersion() string {
	return o.ApiVersion
}

func (o OwnerReference) GetKind() string {
	return o.Kind
}
