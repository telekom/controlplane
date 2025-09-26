// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package client

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/handler"
	"github.com/telekom/controlplane/common/pkg/util/contextutil"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/log"

	identityv1 "github.com/telekom/controlplane/identity/api/v1"
	realmHandler "github.com/telekom/controlplane/identity/internal/handler/realm"
	"github.com/telekom/controlplane/identity/pkg/keycloak"
	"github.com/telekom/controlplane/identity/pkg/keycloak/mapper"
	secrets "github.com/telekom/controlplane/secret-manager/api"
)

var _ handler.Handler[*identityv1.Client] = &HandlerClient{}

type HandlerClient struct{}

func (h *HandlerClient) CreateOrUpdate(ctx context.Context, client *identityv1.Client) (err error) {
	logger := log.FromContext(ctx)
	if client == nil {
		return fmt.Errorf("client is nil")
	}

	SetStatusProcessing(client)

	// Get secret-values from secret-manager
	oldSecretRef := client.Spec.ClientSecret
	defer func() {
		client.Spec.ClientSecret = oldSecretRef
	}()
	client.Spec.ClientSecret, err = secrets.Get(ctx, client.Spec.ClientSecret)
	if err != nil {
		return errors.Wrap(err, "failed to get client secret from secret-manager")
	}

	ready, realm, err := realmHandler.GetRealmByName(ctx, client.Spec.Realm, true)
	if err != nil {
		if apierrors.IsNotFound(err) {
			contextutil.RecorderFromContextOrDie(ctx).
				Eventf(client, "Warning", "RealmNotFound",
					"Realm '%s' not found", client.Spec.Realm.String())
			SetStatusBlocked(client)
			return nil
		}
		return err
	}

	if !ready {
		realm.SetCondition(condition.NewBlockedCondition("Realm not ready"))
		realm.SetCondition(condition.NewNotReadyCondition("RealmNotReady", "Realm not ready"))
		return nil
	}

	realmStatus := realmHandler.ObfuscateRealm(realm.Status)
	logger.V(0).Info("Found Realm", "realm", realmStatus)

	MapToClientStatus(&realm.Status, &client.Status)
	err = realmHandler.ValidateRealmStatus(&realm.Status)
	if err != nil {
		contextutil.RecorderFromContextOrDie(ctx).
			Eventf(client, "Warning", "RealmNotValid",
				"Realm '%s' not valid", client.Spec.Realm.String())
		SetStatusWaiting(client)
		return errors.Wrap(err, "❌ failed to validate realm")
	}

	realmClient, err := keycloak.GetClientFor(realm.Status)
	if err != nil {
		return errors.Wrap(err, "❌ failed to get keycloak client")
	}

	err = realmClient.CreateOrUpdateRealmClient(ctx, realm, client)
	if err != nil {
		return errors.Wrap(err, "❌ failed to create or update client")
	}

	SetStatusReady(client)
	var message = fmt.Sprintf("✅ RealmClient %s is ready", client.Spec.ClientId)
	logger.V(1).Info(message, "IssuerUrl", &client.Status.IssuerUrl)

	return nil
}

func (h *HandlerClient) Delete(ctx context.Context, obj *identityv1.Client) error {
	logger := log.FromContext(ctx)
	logger.Info("ClientHandler Delete", "client", obj)

	ready, realm, err := realmHandler.GetRealmByName(ctx, obj.Spec.Realm, true)
	if err != nil {
		return err
	}

	if realm == nil {
		return errors.Wrap(err, "realm does not exist, skipping deletion")
	}

	if !ready {
		return errors.Wrap(err, "realm not ready!")
	}

	realmClient, err := keycloak.GetClientFor(realm.Status)
	if err != nil {
		return errors.Wrap(err, "failed to get keycloak client")
	}

	getRealmClients, err := realmClient.GetRealmClients(ctx, realm.Name, obj)
	if err != nil {
		return errors.Wrap(err, "failed to get realm clients")
	}

	existingClient, err := mapper.GetClient(*getRealmClients)
	if err != nil {
		return errors.Wrap(err, "failed to get realm client")
	}

	err = realmClient.DeleteRealmClient(ctx, realm.Name, *existingClient.Id)
	if err != nil {
		return errors.Wrap(err, "failed to delete realm client")
	}

	return nil
}
