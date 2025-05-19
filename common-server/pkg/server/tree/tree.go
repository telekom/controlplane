// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package tree

import (
	"context"

	"github.com/go-logr/logr"
	"github.com/telekom/controlplane/common-server/pkg/problems"
	"github.com/telekom/controlplane/common-server/pkg/store"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func GetTree(ctx context.Context, startStore store.ObjectStore[*unstructured.Unstructured], namespace, name string, maxDepth int) (*ResourceTree, error) {
	tree := NewResourceTree()
	err := GetOwner(ctx, startStore, tree, namespace, name, maxDepth, 0)
	if err != nil {
		return nil, err
	}

	if tree.Root == nil {
		return tree, problems.NotFound("resource tree not found")
	}

	rootName := tree.Root.Value.GetName()
	rootNamespace := tree.Root.Value.GetNamespace()

	gvk := tree.Root.Value.GetObjectKind().GroupVersionKind()
	rootStore, ok := LookupStores.GetStore(gvk.GroupVersion().String(), gvk.Kind)
	if !ok {
		return tree, nil
	}

	// construct a brand new tree to store all the children starting from the root object
	tree = NewResourceTree()
	return tree, GetChildren(ctx, rootStore, tree, rootNamespace, rootName, maxDepth, 0)
}

func GetOwner(ctx context.Context, store store.ObjectStore[*unstructured.Unstructured], tree *ResourceTree, namespace, name string, maxDepth, curDepth int) error {
	obj, err := store.Get(ctx, namespace, name)
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

	owner, ok := GetControllerOf(obj)
	if !ok {
		return nil
	}

	ownerStore, ok := LookupStores.GetStore(owner.GetAPIVersion(), owner.GetKind())
	if !ok {
		return nil
	}

	return GetOwner(ctx, ownerStore, tree, owner.Namespace, owner.Name, maxDepth, curDepth+1)
}

func GetChildren(ctx context.Context, s store.ObjectStore[*unstructured.Unstructured], tree *ResourceTree, namespace, name string, maxDepth, curDepth int) error {
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
	for _, childInfo := range LookupResourceHierarchy.GetChildren(obj) {
		log.V(1).Info("Listing children", "apiVersion", childInfo.GetAPIVersion(), "kind", childInfo.GetKind())

		childStore, ok := LookupStores.GetStore(childInfo.GetAPIVersion(), childInfo.GetKind())
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
			err = GetChildren(ctx, childStore, tree, childObj.GetNamespace(), childObj.GetName(), maxDepth, curDepth+1)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

var _ GVK = TreeResourceInfo{}

type MatchType string

const (
	MatchTypeOwnerReference MatchType = "ownerReference"
)

type TreeResourceInfo struct {
	APIVersion string
	Kind       string
	MatchType  MatchType
}

func (t TreeResourceInfo) GetAPIVersion() string {
	return t.APIVersion
}

func (t TreeResourceInfo) GetKind() string {
	return t.Kind
}

func (t TreeResourceInfo) GetFiltersFor(obj *unstructured.Unstructured) []store.Filter {
	switch t.MatchType {
	default:
		return []store.Filter{
			{
				Path:  "metadata.ownerReferences.#.uid",
				Op:    store.OpEqual,
				Value: string(obj.GetUID()),
			},
		}
	}
}
