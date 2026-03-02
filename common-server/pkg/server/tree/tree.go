// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package tree

import (
	"context"
	"strings"

	"github.com/go-logr/logr"
	"github.com/telekom/controlplane/common-server/pkg/problems"
	"github.com/telekom/controlplane/common-server/pkg/store"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

const ownerReferencesPath = "metadata.ownerReferences.#.uid"

type TreeFactory struct {
	LookupStores            *Stores
	LookupResourceHierarchy ResourceHierarchy
}

func NewFactory(resourceHierarchy ResourceHierarchy) *TreeFactory {
	if resourceHierarchy == nil {
		resourceHierarchy = NewStaticResourceHierarchy()
	}

	return &TreeFactory{
		LookupStores:            &Stores{},
		LookupResourceHierarchy: resourceHierarchy,
	}
}

func (t *TreeFactory) GetTree(ctx context.Context, startStore store.ObjectStore[*unstructured.Unstructured], namespace, name string, maxDepth int) (*ResourceTree, error) {
	tree := NewResourceTree()
	err := t.GetOwner(ctx, startStore, tree, namespace, name, maxDepth, 0)
	if err != nil {
		return nil, err
	}

	if tree.Root == nil {
		return tree, problems.NotFound("resource tree not found")
	}

	rootName := tree.Root.Value.GetName()
	rootNamespace := tree.Root.Value.GetNamespace()

	gvk := tree.Root.Value.GetObjectKind().GroupVersionKind()
	rootStore, ok := t.LookupStores.GetStore(gvk.GroupVersion().String(), gvk.Kind)
	if !ok {
		return tree, nil
	}

	// construct a brand new tree to store all the children starting from the root object
	tree = NewResourceTree()
	return tree, t.GetChildren(ctx, rootStore, tree, rootNamespace, rootName, maxDepth, 0)
}

func (t *TreeFactory) GetOwner(ctx context.Context, s store.ObjectStore[*unstructured.Unstructured], tree *ResourceTree, namespace, name string, maxDepth, curDepth int) error {
	obj, err := s.Get(ctx, namespace, name)
	if err != nil {
		if problems.IsNotFound(err) {
			return nil
		}
		return err
	}

	if tree.Root == nil {
		tree.SetRoot(obj)
	} else {
		tree.ReplaceRoot(obj)
	}

	if curDepth >= maxDepth {
		return nil
	}

	ownerInfo, ok := t.LookupResourceHierarchy.GetOwner(obj)
	if !ok {
		return nil
	}

	ownerStore, ok := t.LookupStores.GetStore(ownerInfo.GetAPIVersion(), ownerInfo.GetKind())
	if !ok {
		return nil
	}

	filters, ownerRef, ok := ownerInfo.GetFiltersForOwner(obj)
	if !ok {
		// first get owner
		opts := store.NewListOpts()
		opts.Filters = filters
		listRes, err := ownerStore.List(ctx, opts)
		if err != nil {
			return err
		}
		if len(listRes.Items) == 0 {
			return nil
		}
		ownerRef.Name = listRes.Items[0].GetName()
		ownerRef.Namespace = listRes.Items[0].GetNamespace()
	}

	return t.GetOwner(ctx, ownerStore, tree, ownerRef.Namespace, ownerRef.Name, maxDepth, curDepth+1)
}

func (t *TreeFactory) GetChildren(ctx context.Context, s store.ObjectStore[*unstructured.Unstructured], tree *ResourceTree, namespace, name string, maxDepth, curDepth int) error {
	log := logr.FromContextOrDiscard(ctx)
	log.V(1).Info("Getting children", "namespace", namespace, "name", name)
	obj, err := s.Get(ctx, namespace, name)
	if err != nil {
		if problems.IsNotFound(err) {
			return nil
		}
		return err
	}

	if tree.Root == nil {
		tree.SetRoot(obj)
	} else {
		tree.SetCurrent(tree.AddNewNode(obj))
	}

	if curDepth >= maxDepth {
		return nil
	}

	current := tree.GetCurrent()
	for _, childInfo := range t.LookupResourceHierarchy.GetChildren(obj) {
		log.V(1).Info("Listing children", "apiVersion", childInfo.GetAPIVersion(), "kind", childInfo.GetKind())

		childStore, ok := t.LookupStores.GetStore(childInfo.GetAPIVersion(), childInfo.GetKind())
		if !ok {
			continue
		}

		opts := store.NewListOpts()
		opts.Filters = childInfo.GetFiltersFor(obj)
		log.V(1).Info("Listing children", "filters", opts.Filters)
		childObjs, err := childStore.List(ctx, opts)
		if err != nil {
			return err
		}

		log.V(1).Info("Found children", "count", len(childObjs.Items))

		if len(childObjs.Items) == 0 {
			continue
		}

		for _, childObj := range childObjs.Items {
			tree.SetCurrent(current)
			err = t.GetChildren(ctx, childStore, tree, childObj.GetNamespace(), childObj.GetName(), maxDepth, curDepth+1)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

var _ GVK = TreeResourceInfo{}

type LabelMatcher struct {
	Parent string
	Child  string
}

type MatchVia struct {
	OwnerRef *struct{}       `json:"ownerRef,omitempty" yaml:"ownerRef,omitempty"`
	Labels   *[]LabelMatcher `json:"labels,omitempty" yaml:"labels,omitempty"`
}

type TreeResourceInfo struct {
	APIVersion string   `json:"apiVersion"  yaml:"apiVersion"`
	Kind       string   `json:"kind" yaml:"kind"`
	MatchVia   MatchVia `json:"matchVia" yaml:"matchVia"`
}

func (t TreeResourceInfo) GetAPIVersion() string {
	return t.APIVersion
}

func (t TreeResourceInfo) GetKind() string {
	return t.Kind
}

func (t TreeResourceInfo) GetFiltersFor(obj *unstructured.Unstructured) []store.Filter {
	if t.MatchVia.Labels != nil {
		filters := make([]store.Filter, 0, len(*t.MatchVia.Labels))

		for _, label := range *t.MatchVia.Labels {
			childLabelKey := strings.ReplaceAll(label.Child, ".", "\\.")
			parentLabels := obj.GetLabels()
			if len(parentLabels) == 0 {
				return nil
			}
			value, found := parentLabels[label.Parent]
			if !found {
				return nil
			}
			filters = append(filters, store.Filter{
				Path:  "metadata.labels." + childLabelKey,
				Op:    store.OpEqual,
				Value: value,
			})
		}
		return filters
	}

	return []store.Filter{
		{
			Path:  ownerReferencesPath,
			Op:    store.OpEqual,
			Value: string(obj.GetUID()),
		},
	}
}

func (t TreeResourceInfo) GetFiltersForOwner(obj *unstructured.Unstructured) ([]store.Filter, OwnerReference, bool) {
	filters := t.GetFiltersFor(obj)
	if isViaOwnerRef(filters) {
		ownerRef, ok := GetControllerOf(obj)
		return nil, ownerRef, ok
	}

	return filters, OwnerReference{}, false
}

func isViaOwnerRef(filters []store.Filter) bool {
	if len(filters) > 1 {
		return false
	}
	return filters[0].Path == ownerReferencesPath
}
