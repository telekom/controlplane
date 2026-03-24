// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package realm

import (
	"context"
	"fmt"

	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/errors/ctrlerrors"
	"github.com/telekom/controlplane/common/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"

	identityv1 "github.com/telekom/controlplane/identity/api/v1"
	"github.com/telekom/controlplane/identity/internal/handler/identityprovider"
	"github.com/telekom/controlplane/identity/pkg/keycloak"

	secrets "github.com/telekom/controlplane/secret-manager/api"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

var _ handler.Handler[*identityv1.Realm] = &HandlerRealm{}

type HandlerRealm struct {
	ClientFactory keycloak.ClientFactory
}

// NewHandlerRealm creates a HandlerRealm with the given ClientFactory.
func NewHandlerRealm(factory keycloak.ClientFactory) *HandlerRealm {
	return &HandlerRealm{ClientFactory: factory}
}

func (h *HandlerRealm) CreateOrUpdate(ctx context.Context, realm *identityv1.Realm) error {
	if realm == nil {
		return fmt.Errorf("realm is nil")
	}

	identityProvider, err := identityprovider.GetIdentityProviderByName(ctx, realm.Spec.IdentityProvider)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return ctrlerrors.BlockedErrorf("IdentityProvider %q not found", realm.Spec.IdentityProvider.String())
		}
		return err
	}

	var realmStatus = mapToRealmStatus(identityProvider, realm.Name)
	err = ValidateRealmStatus(&realmStatus)
	if err != nil {
		return ctrlerrors.BlockedErrorf("IdentityProvider %q is not valid: %s", realm.Spec.IdentityProvider.String(), err)
	}

	// Create a copy of the realmStatus so that we NEVER modify the original status
	// and accidentally write the secrets back to the cluster
	replacedRealmStatus := realmStatus.DeepCopy()
	replacedRealmStatus.AdminPassword, err = secrets.Get(ctx, realmStatus.AdminPassword)
	if err != nil {
		return fmt.Errorf("failed to retrieve password from secret manager: %w", err)
	}

	realmClient, err := h.ClientFactory.ClientFor(*replacedRealmStatus)
	if err != nil {
		return fmt.Errorf("failed to get keycloak client: %w", err)
	}

	err = realmClient.CreateOrUpdateRealm(ctx, realm)
	if err != nil {
		return fmt.Errorf("failed to create or update realm: %w", err)
	}

	realm.Status = realmStatus
	realm.SetCondition(condition.NewDoneProcessingCondition("Created Realm"))
	realm.SetCondition(condition.NewReadyCondition("Ready", "Realm is ready"))

	return nil
}

func (h *HandlerRealm) Delete(ctx context.Context, realm *identityv1.Realm) error {

	logger := log.FromContext(ctx)
	logger.Info("RealmHandler Delete", "realm", realm)

	adminPassword, err := secrets.Get(ctx, realm.Status.AdminPassword)
	if err != nil {
		return fmt.Errorf("failed to retrieve password from secret manager: %w", err)
	}

	// Use a copy of the status with the resolved password so we never
	// overwrite the original realm.Status.AdminPassword (which may be
	// a secret-manager reference) with plaintext.
	resolvedStatus := *realm.Status.DeepCopy()
	resolvedStatus.AdminPassword = adminPassword

	realmClient, err := h.ClientFactory.ClientFor(resolvedStatus)
	if err != nil {
		return fmt.Errorf("failed to get keycloak client: %w", err)
	}

	err = realmClient.DeleteRealm(ctx, realm.Name)
	if err != nil {
		return fmt.Errorf("failed to delete realm: %w", err)
	}

	return nil
}

func mapToRealmStatus(identityProvider *identityv1.IdentityProvider, realmName string) identityv1.RealmStatus {
	return identityv1.RealmStatus{
		IssuerUrl:     keycloak.DetermineIssuerUrlFrom(identityProvider.Spec.AdminUrl, realmName),
		AdminClientId: identityProvider.Spec.AdminClientId,
		AdminUserName: identityProvider.Spec.AdminUserName,
		AdminPassword: identityProvider.Spec.AdminPassword,
		AdminUrl:      identityProvider.Status.AdminUrl,
		AdminTokenUrl: identityProvider.Status.AdminTokenUrl,
	}
}
