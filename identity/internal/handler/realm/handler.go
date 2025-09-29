// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package realm

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
	"github.com/telekom/controlplane/common/pkg/handler"
	"github.com/telekom/controlplane/common/pkg/util/contextutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	identityv1 "github.com/telekom/controlplane/identity/api/v1"
	"github.com/telekom/controlplane/identity/internal/handler/identityprovider"
	"github.com/telekom/controlplane/identity/pkg/keycloak"

	secrets "github.com/telekom/controlplane/secret-manager/api"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

var _ handler.Handler[*identityv1.Realm] = &HandlerRealm{}

type HandlerRealm struct{}

func (h *HandlerRealm) CreateOrUpdate(ctx context.Context, realm *identityv1.Realm) error {
	logger := log.FromContext(ctx)
	if realm == nil {
		return fmt.Errorf("realm is nil")
	}

	SetStatusProcessing(&realm.Status, realm)

	identityProvider, err := identityprovider.GetIdentityProviderByName(ctx, realm.Spec.IdentityProvider)
	if err != nil {
		if apierrors.IsNotFound(err) {
			contextutil.RecorderFromContextOrDie(ctx).
				Eventf(identityProvider, "Warning", "IdentityProviderNotFound",
					"IdentityProvider '%s' not found", realm.Spec.IdentityProvider.String())
			SetStatusBlocked(&realm.Status, realm)
			return nil
		}
		return err
	}
	idpSpec := identityprovider.ObfuscateIdentityProvider(identityProvider.Spec)
	logger.V(0).Info("Found IdentityProvider", "idp", idpSpec)

	var realmStatus = MapToRealmStatus(identityProvider, realm.Name)
	err = ValidateRealmStatus(&realmStatus)
	if err != nil {
		contextutil.RecorderFromContextOrDie(ctx).
			Eventf(identityProvider, "Warning", "IdentityProviderNotValid",
				"IdentityProvider '%s' not valid", realm.Spec.IdentityProvider.String())
		SetStatusWaiting(&realm.Status, realm)
		return errors.Wrap(err, "failed to validate IdentityProvider")
	}

	// Create a copy of the realmStatus so that we NEVER modify the original status
	// and accidentally write the secrets back to the cluster
	replacedRealmStatus := realmStatus.DeepCopy()
	replacedRealmStatus.AdminPassword, err = secrets.Get(ctx, realmStatus.AdminPassword)
	if err != nil {
		return errors.Wrap(err, "failed to retrieve password from secret manager")
	}

	realmClient, err := keycloak.GetClientFor(*replacedRealmStatus)
	if err != nil {
		return errors.Wrap(err, "failed to get keycloak client")
	}

	err = realmClient.CreateOrUpdateRealm(ctx, realm)
	if err != nil {
		return errors.Wrap(err, "failed to create or update realm")
	}

	SetStatusReady(&realmStatus, realm)
	var message = fmt.Sprintf("Realm %s is ready", realm.Name)
	logger.V(0).Info(message)
	return nil
}

func (h *HandlerRealm) Delete(ctx context.Context, realm *identityv1.Realm) error {

	logger := log.FromContext(ctx)
	logger.Info("RealmHandler Delete", "realm", realm)

	adminPassword, err := secrets.Get(ctx, realm.Status.AdminPassword)
	if err != nil {
		return errors.Wrap(err, "failed to retrieve password from secret manager")
	}

	realm.Status.AdminPassword = adminPassword

	realmClient, err := keycloak.GetClientFor(realm.Status)
	if err != nil {
		return errors.Wrap(err, "failed to get keycloak client")
	}

	err = realmClient.DeleteRealm(ctx, realm.Name)
	if err != nil {
		return errors.Wrap(err, "failed to delete realm")
	}

	return nil
}
