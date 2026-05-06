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
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/log"

	identityv1 "github.com/telekom/controlplane/identity/api/v1"
	"github.com/telekom/controlplane/identity/internal/handler/identityprovider"
	"github.com/telekom/controlplane/identity/pkg/keycloak"

	secrets "github.com/telekom/controlplane/secret-manager/api"
)

var _ handler.Handler[*identityv1.Realm] = &HandlerRealm{}

type HandlerRealm struct {
	ServiceFactory keycloak.ServiceFactory
}

// NewHandlerRealm creates a HandlerRealm with the given ClientFactory.
func NewHandlerRealm(factory keycloak.ServiceFactory) *HandlerRealm {
	return &HandlerRealm{ServiceFactory: factory}
}

func (h *HandlerRealm) CreateOrUpdate(ctx context.Context, realm *identityv1.Realm) error {
	logger := log.FromContext(ctx)
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

	realm.Status.IssuerUrl = keycloak.DetermineIssuerUrlFrom(identityProvider.Spec.AdminUrl, realm.Name)
	realm.Status.AdminClientId = identityProvider.Spec.AdminClientId
	realm.Status.AdminUserName = identityProvider.Spec.AdminUserName
	realm.Status.AdminPassword = identityProvider.Spec.AdminPassword
	realm.Status.AdminUrl = identityProvider.Status.AdminUrl
	realm.Status.AdminTokenUrl = identityProvider.Status.AdminTokenUrl

	err = ValidateRealmStatus(&realm.Status)
	if err != nil {
		return ctrlerrors.BlockedErrorf("IdentityProvider %q is not valid: %s", realm.Spec.IdentityProvider.String(), err)
	}

	// Create a copy of the realmStatus so that we NEVER modify the original status
	// and accidentally write the secrets back to the cluster
	replacedRealmStatus := realm.Status.DeepCopy()
	replacedRealmStatus.AdminPassword, err = secrets.Get(ctx, realm.Status.AdminPassword)
	if err != nil {
		return fmt.Errorf("failed to retrieve password from secret manager: %w", err)
	}

	realmClient, err := h.ServiceFactory.ServiceFor(*replacedRealmStatus)
	if err != nil {
		return fmt.Errorf("failed to get keycloak client: %w", err)
	}

	err = realmClient.CreateOrReplaceRealm(ctx, realm)
	if err != nil {
		return fmt.Errorf("failed to create or update realm: %w", err)
	}

	// If secret rotation is configured, ensure the Keycloak realm has the
	// corresponding client-policy profile + policy.
	if realm.SupportsGracefulSecretRotation() {
		logger.Info("configuring secret rotation policy for realm", "realm", realm.Name, "policy", realm.Spec.SecretRotation)
		if err := realmClient.ConfigureSecretRotationPolicy(
			ctx, realm.Name, realm.Spec.SecretRotation,
		); err != nil {
			return fmt.Errorf("failed to configure secret rotation policy: %w", err)
		}
	}

	realm.SetCondition(condition.NewDoneProcessingCondition("Created Realm"))
	realm.SetCondition(condition.NewReadyCondition("Ready", "Realm is ready"))

	return nil
}

func (h *HandlerRealm) Delete(ctx context.Context, realm *identityv1.Realm) error {

	logger := log.FromContext(ctx)
	logger.Info("RealmHandler Delete", "realm", realm.Name, "namespace", realm.Namespace)

	adminPassword, err := secrets.Get(ctx, realm.Status.AdminPassword)
	if err != nil {
		return fmt.Errorf("failed to retrieve password from secret manager: %w", err)
	}

	// Use a copy of the status with the resolved password so we never
	// overwrite the original realm.Status.AdminPassword (which may be
	// a secret-manager reference) with plaintext.
	resolvedStatus := *realm.Status.DeepCopy()
	resolvedStatus.AdminPassword = adminPassword

	realmClient, err := h.ServiceFactory.ServiceFor(resolvedStatus)
	if err != nil {
		return fmt.Errorf("failed to get keycloak client: %w", err)
	}

	err = realmClient.DeleteRealm(ctx, realm.Name)
	if err != nil {
		return fmt.Errorf("failed to delete realm: %w", err)
	}

	return nil
}
