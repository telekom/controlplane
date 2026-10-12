// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"os"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	identityv1 "github.com/telekom/controlplane/identity/api/v1"
)

var (
	IndexFieldSpecIdentityProvider = "spec.identityProvider"
	IndexFieldSpecRealm            = "spec.realm"
)

func RegisterIndexesOrDie(ctx context.Context, mgr ctrl.Manager) {
	// Index the Realm by the IdentityProvider it references.
	// This allows efficient lookup of all Realms pointing to a specific IdentityProvider.
	filterIdpOnRealm := func(obj client.Object) []string {
		realm, ok := obj.(*identityv1.Realm)
		if !ok {
			return nil
		}
		if realm.Spec.IdentityProvider == nil {
			return nil
		}
		return []string{realm.Spec.IdentityProvider.String()}
	}

	err := mgr.GetFieldIndexer().IndexField(ctx, &identityv1.Realm{}, IndexFieldSpecIdentityProvider, filterIdpOnRealm)
	if err != nil {
		ctrl.Log.Error(err, "unable to create fieldIndex for Realm", "FieldIndex", IndexFieldSpecIdentityProvider)
		os.Exit(1)
	}

	// Index the Client by the Realm it references.
	// This allows efficient lookup of all Clients pointing to a specific Realm.
	filterRealmOnClient := func(obj client.Object) []string {
		identityClient, ok := obj.(*identityv1.Client)
		if !ok {
			return nil
		}
		if identityClient.Spec.Realm == nil {
			return nil
		}
		return []string{identityClient.Spec.Realm.String()}
	}

	err = mgr.GetFieldIndexer().IndexField(ctx, &identityv1.Client{}, IndexFieldSpecRealm, filterRealmOnClient)
	if err != nil {
		ctrl.Log.Error(err, "unable to create fieldIndex for Client", "FieldIndex", IndexFieldSpecRealm)
		os.Exit(1)
	}
}

func requestsFor[T client.Object](items []T) []reconcile.Request {
	reqs := make([]reconcile.Request, 0, len(items))
	for _, item := range items {
		reqs = append(reqs, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(item)})
	}
	return reqs
}

func itemPtrs[T any](items []T) []*T {
	out := make([]*T, len(items))
	for i := range items {
		out[i] = &items[i]
	}
	return out
}
