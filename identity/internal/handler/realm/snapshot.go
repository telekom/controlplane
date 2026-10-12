// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package realm

import (
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/errors/ctrlerrors"
	ctypes "github.com/telekom/controlplane/common/pkg/types"
	identityv1 "github.com/telekom/controlplane/identity/api/v1"
	"github.com/telekom/controlplane/identity/pkg/keycloak"
)

// requireCurrentIdp rejects an IdentityProvider whose Ready/TrustReady conditions were not
// observed for its current generation, or whose status URLs do not match its current spec.
// This rejects observed staleness; it is not a cross-object transaction.
func requireCurrentIdp(realm *identityv1.Realm, idp *identityv1.IdentityProvider) error {
	ref := realm.Spec.IdentityProvider.String()
	for _, t := range []string{condition.ConditionTypeReady, identityv1.TrustReadyConditionType} {
		c := meta.FindStatusCondition(idp.Status.Conditions, t)
		if c == nil || c.Status != metav1.ConditionTrue || c.ObservedGeneration != idp.Generation {
			return ctrlerrors.BlockedErrorf("IdentityProvider %q condition %s is not true for generation %d", ref, t, idp.Generation)
		}
	}
	if idp.Status.AdminUrl != idp.Spec.AdminUrl ||
		idp.Status.AdminTokenUrl != keycloak.DetermineAdminTokenUrlFrom(idp.Spec.AdminUrl, keycloak.MasterRealm) {
		return ctrlerrors.BlockedErrorf("IdentityProvider %q status does not reflect its current spec", ref)
	}
	return nil
}

// stageSnapshot derives the desired connection snapshot from one IdentityProvider spec.
// All target URLs use the same derivations; credential inputs are copied as configured
// (literal or secret reference), never resolved.
func stageSnapshot(realm *identityv1.Realm, idp *identityv1.IdentityProvider) (*identityv1.RealmStatus, error) {
	candidate := &identityv1.RealmStatus{
		IssuerUrl:     keycloak.DetermineIssuerUrlFrom(idp.Spec.AdminUrl, realm.Name),
		AdminClientId: idp.Spec.AdminClientId,
		AdminUserName: idp.Spec.AdminUserName,
		AdminPassword: idp.Spec.AdminPassword,
		AdminUrl:      idp.Spec.AdminUrl,
		AdminTokenUrl: keycloak.DetermineAdminTokenUrlFrom(idp.Spec.AdminUrl, keycloak.MasterRealm),
	}
	if err := ValidateRealmStatus(candidate); err != nil {
		return nil, ctrlerrors.BlockedErrorf("IdentityProvider %q is not valid: %s", realm.Spec.IdentityProvider.String(), err)
	}
	ref := idp.Spec.TLS.Ref()
	if ref == nil {
		return candidate, nil
	}
	if idp.Namespace != realm.Namespace {
		err := ctrlerrors.BlockedErrorf("IdentityProvider %q CA bundle reference cannot be used by a Realm in namespace %q",
			realm.Spec.IdentityProvider.String(), realm.Namespace)
		realm.SetCondition(identityv1.NewTrustNotReadyCondition(err.Error()))
		return nil, err
	}
	if err := keycloak.RequireHTTPS(candidate.IssuerUrl, candidate.AdminUrl, candidate.AdminTokenUrl); err != nil {
		blocked := ctrlerrors.BlockedErrorf("IdentityProvider %q: %s", realm.Spec.IdentityProvider.String(), err)
		realm.SetCondition(identityv1.NewTrustNotReadyCondition(blocked.Error()))
		return nil, blocked
	}
	candidate.TLS = &ctypes.ProviderTLS{CABundleConfigMapRef: ref.DeepCopy()}
	return candidate, nil
}

// applySnapshot copies the connection fields of src into dst. They are persisted with the
// normal status update and serve as the cleanup connection on deletion.
func applySnapshot(dst, src *identityv1.RealmStatus) {
	dst.IssuerUrl = src.IssuerUrl
	dst.AdminClientId = src.AdminClientId
	dst.AdminUserName = src.AdminUserName
	dst.AdminPassword = src.AdminPassword
	dst.AdminUrl = src.AdminUrl
	dst.AdminTokenUrl = src.AdminTokenUrl
	dst.TLS = src.TLS.DeepCopy()
}
