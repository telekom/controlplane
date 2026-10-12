// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package realm

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/errors/ctrlerrors"
	"github.com/telekom/controlplane/common/pkg/handler"
	identityv1 "github.com/telekom/controlplane/identity/api/v1"
	"github.com/telekom/controlplane/identity/internal/handler/identityprovider"
	"github.com/telekom/controlplane/identity/pkg/keycloak"
)

var _ handler.Handler[*identityv1.Realm] = &HandlerRealm{}

type HandlerRealm struct {
	ServiceFactory keycloak.ServiceFactory
	// CA is the shared CA cache that also serves client acquisition.
	CA keycloak.CAResolver
}

// NewHandlerRealm creates a HandlerRealm.
func NewHandlerRealm(factory keycloak.ServiceFactory, ca keycloak.CAResolver) *HandlerRealm {
	return &HandlerRealm{ServiceFactory: factory, CA: ca}
}

func (h *HandlerRealm) CreateOrUpdate(ctx context.Context, realm *identityv1.Realm) error {
	logger := log.FromContext(ctx)
	if realm == nil {
		return fmt.Errorf("realm is nil")
	}

	identityProvider, err := identityprovider.GetIdentityProviderByName(ctx, realm.Spec.IdentityProvider)
	if err != nil {
		if apierrors.IsNotFound(err) {
			err = ctrlerrors.BlockedErrorf("IdentityProvider %q not found", realm.Spec.IdentityProvider.String())
		}
		h.reportCommittedTrust(ctx, realm, err)
		return err
	}
	if staleErr := requireCurrentIdp(realm, identityProvider); staleErr != nil {
		h.reportCommittedTrust(ctx, realm, staleErr)
		return staleErr
	}

	candidate, err := stageSnapshot(realm, identityProvider)
	if err != nil {
		return err
	}
	if trustErr := keycloak.ValidateProviderCA(ctx, h.CA, realm.Namespace, candidate.TLS.Ref()); trustErr != nil {
		realm.SetCondition(identityv1.NewTrustNotReadyCondition(trustErr.Error()))
		return trustErr
	}
	realm.SetCondition(identityv1.NewTrustReadyCondition("Provider trust resolved"))

	// A changed target is not migrated: resources at a previous target are left in place.
	applySnapshot(&realm.Status, candidate)

	realmClient, err := h.ServiceFactory.ServiceFor(ctx, realm)
	if err != nil {
		return fmt.Errorf("failed to get keycloak client: %w", err)
	}

	err = realmClient.CreateOrReplaceRealm(ctx, realm)
	if err != nil {
		return fmt.Errorf("failed to create or update realm: %w", err)
	}

	// If secret rotation is configured, ensure the Keycloak realm has the
	// corresponding client-policy profile + policy. Otherwise, remove any
	// previously created profile + policy so they don't linger.
	if realm.SupportsGracefulSecretRotation() {
		logger.Info("configuring secret rotation policy for realm", "realm", realm.Name, "policy", realm.Spec.SecretRotation)
		if err := realmClient.ConfigureSecretRotationPolicy(
			ctx, realm.Name, realm.Spec.SecretRotation,
		); err != nil {
			return fmt.Errorf("failed to configure secret rotation policy: %w", err)
		}
	} else {
		if err := realmClient.DeleteSecretRotationPolicy(ctx, realm.Name); err != nil {
			return fmt.Errorf("failed to delete secret rotation policy: %w", err)
		}
	}

	// Configure the managed client scope for custom claims. This is
	// always called — an empty claims slice removes the scope if it exists.
	if err := realmClient.ConfigureClientScopes(ctx, realm.Name, realm.Spec.Claims); err != nil {
		return fmt.Errorf("failed to configure client scopes: %w", err)
	}

	realm.SetCondition(condition.NewDoneProcessingCondition("Created Realm"))
	realm.SetCondition(condition.NewReadyCondition(condition.ReasonProvisioned, "Realm is ready"))

	return nil
}

// reportCommittedTrust sets TrustReady when no new candidate can be staged. A committed
// snapshot reports the resolution of its persisted trust; otherwise trust is not resolved
// and TrustReady is set False so a previous True never remains stale.
func (h *HandlerRealm) reportCommittedTrust(ctx context.Context, realm *identityv1.Realm, cause error) {
	switch identityv1.ClassifyRealmSnapshot(&realm.Status) {
	case identityv1.RealmSnapshotComplete:
		if err := keycloak.RequireSnapshotHTTPS(&realm.Status); err != nil {
			realm.SetCondition(identityv1.NewTrustNotReadyCondition(err.Error()))
			return
		}
		if err := keycloak.ValidateProviderCA(ctx, h.CA, realm.Namespace, realm.Status.TLS.Ref()); err != nil {
			realm.SetCondition(identityv1.NewTrustNotReadyCondition(err.Error()))
			return
		}
		realm.SetCondition(identityv1.NewTrustReadyCondition("Committed provider trust resolved"))
	case identityv1.RealmSnapshotPartial:
		realm.SetCondition(identityv1.NewTrustNotReadyCondition(
			fmt.Sprintf("realm %s has a partial connection snapshot and requires operator repair", realm.Name)))
	default:
		realm.SetCondition(identityv1.NewTrustNotReadyCondition(
			fmt.Sprintf("provider trust not resolved: %s", cause)))
	}
}

func (h *HandlerRealm) Delete(ctx context.Context, realm *identityv1.Realm) error {
	logger := log.FromContext(ctx)
	logger.Info("RealmHandler Delete", "realm", realm.Name, "namespace", realm.Namespace)

	switch identityv1.ClassifyRealmSnapshot(&realm.Status) {
	case identityv1.RealmSnapshotEmpty:
		logger.Info("Realm has no recorded connection, nothing to clean up")
		return nil
	case identityv1.RealmSnapshotPartial:
		return ctrlerrors.BlockedErrorf("realm %s has a partial connection snapshot and requires operator repair", realm.Name)
	}

	realmClient, err := h.ServiceFactory.ServiceFor(ctx, realm)
	if err != nil {
		return fmt.Errorf("failed to get keycloak client: %w", err)
	}

	err = realmClient.DeleteRealm(ctx, realm.Name)
	if err != nil {
		return fmt.Errorf("failed to delete realm: %w", err)
	}

	return nil
}
