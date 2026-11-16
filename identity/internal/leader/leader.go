// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package leader

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"strings"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/telekom/controlplane/common/pkg/types"
	identityv1 "github.com/telekom/controlplane/identity/api/v1"
)

const (
	IndexFieldSpecAdminURL         = "spec.adminUrl"
	IndexFieldSpecIdentityProvider = "spec.identityProvider"
	IndexFieldSpecRealm            = "spec.realm"
)

// AdminURLIndexValue extracts the admin URL of an IdentityProvider for the field index.
func AdminURLIndexValue(obj client.Object) []string {
	idp, ok := obj.(*identityv1.IdentityProvider)
	if !ok {
		return nil
	}
	if idp.Spec.AdminUrl == "" {
		return nil
	}
	return []string{idp.Spec.AdminUrl}
}

// RegisterAdminURLIndex installs the IdentityProvider URL index.
func RegisterAdminURLIndex(ctx context.Context, indexer client.FieldIndexer) error {
	return indexer.IndexField(ctx, &identityv1.IdentityProvider{}, IndexFieldSpecAdminURL, AdminURLIndexValue)
}

// RealmsForAdminURL returns live realms attached to any IdentityProvider using the same URL.
func RealmsForAdminURL(ctx context.Context, reader client.Reader, idp *identityv1.IdentityProvider) ([]identityv1.Realm, error) {
	return liveRealmsForAdminURL(ctx, reader, idp.Spec.AdminUrl, "", types.ObjectRefFromObject(idp).String())
}

// RealmPeers returns the live members of a realm's leader-election group.
func RealmPeers(ctx context.Context, reader client.Reader, realm *identityv1.Realm, idp *identityv1.IdentityProvider) ([]identityv1.Realm, error) {
	peers, err := liveRealmsForAdminURL(ctx, reader, idp.Spec.AdminUrl, realm.Name, types.ObjectRefFromObject(idp).String())
	if err != nil {
		return nil, err
	}
	return withLiveSelf(peers, realm), nil
}

// RealmPeersForRealm returns peers using both the Realm's current IdentityProvider URL
// and its last reconciled URL, so changes and deletion can hand leadership over safely.
func RealmPeersForRealm(ctx context.Context, reader client.Reader, realm *identityv1.Realm) ([]identityv1.Realm, error) {
	peers := make([]identityv1.Realm, 0)
	if realm.Spec.IdentityProvider != nil {
		idp := &identityv1.IdentityProvider{}
		// audit:ignore 13-environment-isolation/unscoped-get We need to get the IdentityProvider to determine leadership.
		err := reader.Get(ctx, realm.Spec.IdentityProvider.K8s(), idp)
		switch {
		case err == nil:
			currentPeers, peersErr := RealmPeers(ctx, reader, realm, idp)
			if peersErr != nil {
				return nil, peersErr
			}
			peers = append(peers, currentPeers...)
		case client.IgnoreNotFound(err) != nil:
			return nil, fmt.Errorf("getting IdentityProvider %s for Realm %s: %w", realm.Spec.IdentityProvider, client.ObjectKeyFromObject(realm), err)
		}
	}

	if realm.Status.AdminUrl != "" {
		previousPeers, err := liveRealmsForAdminURL(ctx, reader, realm.Status.AdminUrl, realm.Name)
		if err != nil {
			return nil, err
		}
		peers = append(peers, previousPeers...)
	}
	return uniqueObjects(withLiveSelf(peers, realm)), nil
}

// ClientsForRealmGroup returns live clients with the same client ID across the given realms.
func ClientsForRealmGroup(ctx context.Context, reader client.Reader, realms []identityv1.Realm, identityClient *identityv1.Client) ([]identityv1.Client, error) {
	peers := make([]identityv1.Client, 0)
	groupRealms := uniqueObjects(realms)
	for i := range groupRealms {
		ref := types.ObjectRefFromObject(&groupRealms[i]).String()
		clients := &identityv1.ClientList{}
		// audit:ignore 13-environment-isolation/unscoped-list We need to list all clients for the given realm to determine leadership.
		if err := reader.List(ctx, clients, client.MatchingFields{IndexFieldSpecRealm: ref}); err != nil {
			return nil, fmt.Errorf("listing Clients for Realm %s: %w", ref, err)
		}
		for j := range clients.Items {
			candidate := clients.Items[j]
			if candidate.Spec.ClientId == identityClient.Spec.ClientId && candidate.DeletionTimestamp.IsZero() {
				peers = append(peers, candidate)
			}
		}
	}
	return withLiveSelf(peers, identityClient), nil
}

// ElectRealm returns the leader among the given live realms without modifying the slice.
func ElectRealm(realms []identityv1.Realm) *identityv1.Realm {
	return elect(realms)
}

// ElectClient returns the leader among the given live clients without modifying the slice.
func ElectClient(clients []identityv1.Client) *identityv1.Client {
	return elect(clients)
}

// objectPtr constrains generic helpers to pointer types of Kubernetes API objects.
type objectPtr[T any] interface {
	*T
	client.Object
}

// elect picks the candidate with the oldest creation timestamp. Candidates created in the
// same second are ordered by namespace, then name, so every reconciler elects the same leader.
func elect[T any, PT objectPtr[T]](candidates []T) *T {
	if len(candidates) == 0 {
		return nil
	}
	elected := slices.MinFunc(candidates, func(a, b T) int {
		return compareCandidates(PT(&a), PT(&b))
	})
	return &elected
}

func compareCandidates(a, b client.Object) int {
	return cmp.Or(
		a.GetCreationTimestamp().Compare(b.GetCreationTimestamp().Time),
		strings.Compare(a.GetNamespace(), b.GetNamespace()),
		strings.Compare(a.GetName(), b.GetName()),
	)
}

// liveRealmsForAdminURL lists live realms of every IdentityProvider with the admin URL
// and of extraIdpRefs. An empty realmName matches all realms.
func liveRealmsForAdminURL(ctx context.Context, reader client.Reader, adminURL, realmName string, extraIdpRefs ...string) ([]identityv1.Realm, error) {
	idps := &identityv1.IdentityProviderList{}
	// Shared Keycloak realms need one leader across all environments using this admin URL.
	if err := reader.List(ctx, idps, client.MatchingFields{IndexFieldSpecAdminURL: adminURL}); err != nil {
		return nil, fmt.Errorf("listing IdentityProviders for admin URL %q: %w", adminURL, err)
	}
	idpRefs := slices.Clone(extraIdpRefs)
	for i := range idps.Items {
		idpRefs = append(idpRefs, types.ObjectRefFromObject(&idps.Items[i]).String())
	}
	slices.Sort(idpRefs)

	realms := make([]identityv1.Realm, 0)
	for _, idpRef := range slices.Compact(idpRefs) {
		idpRealms := &identityv1.RealmList{}
		if err := reader.List(ctx, idpRealms, client.MatchingFields{IndexFieldSpecIdentityProvider: idpRef}); err != nil {
			return nil, fmt.Errorf("listing Realms for IdentityProvider %s: %w", idpRef, err)
		}
		for i := range idpRealms.Items {
			realm := &idpRealms.Items[i]
			if realm.DeletionTimestamp.IsZero() && (realmName == "" || realm.Name == realmName) {
				realms = append(realms, *realm)
			}
		}
	}
	return realms, nil
}

// withLiveSelf adds a copy of self to peers unless it is being deleted or already listed.
func withLiveSelf[T any, PT objectPtr[T]](peers []T, self PT) []T {
	if !self.GetDeletionTimestamp().IsZero() {
		return peers
	}
	key := client.ObjectKeyFromObject(self)
	for i := range peers {
		if client.ObjectKeyFromObject(PT(&peers[i])) == key {
			return peers
		}
	}
	copied, ok := self.DeepCopyObject().(PT)
	if !ok {
		copied = self
	}
	return append(peers, *copied)
}

// uniqueObjects removes later duplicates of the same object key, keeping order.
func uniqueObjects[T any, PT objectPtr[T]](objects []T) []T {
	unique := make([]T, 0, len(objects))
	seen := make(map[client.ObjectKey]struct{}, len(objects))
	for i := range objects {
		key := client.ObjectKeyFromObject(PT(&objects[i]))
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		unique = append(unique, objects[i])
	}
	return unique
}
