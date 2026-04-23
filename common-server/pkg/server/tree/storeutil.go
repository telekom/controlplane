// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package tree

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/telekom/controlplane/common-server/pkg/store"
)

var LookupStores = &Stores{
	stores: make(map[string]store.ObjectStore[*unstructured.Unstructured]),
}

type Stores struct {
	stores map[string]store.ObjectStore[*unstructured.Unstructured]
}

func (s *Stores) GetStore(groupVersion, kind string) (store.ObjectStore[*unstructured.Unstructured], bool) {
	storeId := groupVersion + "." + kind
	store, ok := s.stores[storeId]
	return store, ok
}

func (s *Stores) AddStore(store store.ObjectStore[*unstructured.Unstructured]) {
	_, gvk := store.Info()
	storeId := gvk.Group + "/" + gvk.Version + "." + gvk.Kind
	s.stores[storeId] = store
}

func GetControllerOf(obj *unstructured.Unstructured) (ref OwnerReference, ok bool) {
	owners := obj.GetOwnerReferences()
	if len(owners) == 0 {
		return ref, ok
	}
	for _, owner := range owners {
		if owner.Controller != nil && *owner.Controller {
			return OwnerReference{
				ApiVersion: owner.APIVersion,
				Kind:       owner.Kind,
				Name:       owner.Name,
				Namespace:  obj.GetNamespace(),
				Uid:        string(owner.UID),
			}, true
		}
	}

	return ref, ok
}
