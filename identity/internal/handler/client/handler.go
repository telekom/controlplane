// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package client

import (
	"context"
	"fmt"

	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/errors/ctrlerrors"
	"github.com/telekom/controlplane/common/pkg/handler"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/log"

	identityv1 "github.com/telekom/controlplane/identity/api/v1"
	realmHandler "github.com/telekom/controlplane/identity/internal/handler/realm"
	"github.com/telekom/controlplane/identity/pkg/keycloak"
	"github.com/telekom/controlplane/identity/pkg/keycloak/mapper"
	secrets "github.com/telekom/controlplane/secret-manager/api"
)

var _ handler.Handler[*identityv1.Client] = &HandlerClient{}

type HandlerClient struct {
	ClientFactory keycloak.ClientFactory
}

// NewHandlerClient creates a HandlerClient with the given ClientFactory.
func NewHandlerClient(factory keycloak.ClientFactory) *HandlerClient {
	return &HandlerClient{ClientFactory: factory}
}

func (h *HandlerClient) CreateOrUpdate(ctx context.Context, client *identityv1.Client) (err error) {
	if client == nil {
		return fmt.Errorf("client is nil")
	}

	resolvedSecret, err := secrets.Get(ctx, client.Spec.ClientSecret)
	if err != nil {
		return fmt.Errorf("failed to get client secret from secret-manager: %w", err)
	}

	ready, realm, err := realmHandler.GetRealmByName(ctx, client.Spec.Realm, true)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return ctrlerrors.BlockedErrorf("Realm %q not found", client.Spec.Realm.String())
		}
		return err
	}

	if !ready {
		return ctrlerrors.BlockedErrorf("Realm %q is not ready", client.Spec.Realm.String())
	}

	mapToClientStatus(&realm.Status, &client.Status)
	err = realmHandler.ValidateRealmStatus(&realm.Status)
	if err != nil {
		return ctrlerrors.BlockedErrorf("Realm %q is not valid: %s", client.Spec.Realm.String(), err)
	}

	realmClient, err := h.ClientFactory.ClientFor(realm.Status)
	if err != nil {
		return fmt.Errorf("failed to get keycloak client: %w", err)
	}

	// Use a copy of the client with the resolved secret so the original
	// client.Spec.ClientSecret (which may be a secret-manager reference)
	// is never overwritten with plaintext.
	clientCopy := client.DeepCopy()
	clientCopy.Spec.ClientSecret = resolvedSecret

	err = realmClient.CreateOrUpdateRealmClient(ctx, realm, clientCopy)
	if err != nil {
		return fmt.Errorf("failed to create or update client: %w", err)
	}

	client.SetCondition(condition.NewDoneProcessingCondition("Created Client"))
	client.SetCondition(condition.NewReadyCondition("Ready", "Client is ready"))

	return nil
}

func (h *HandlerClient) Delete(ctx context.Context, obj *identityv1.Client) error {
	logger := log.FromContext(ctx)
	logger.Info("ClientHandler Delete", "client", obj)

	ready, realm, err := realmHandler.GetRealmByName(ctx, obj.Spec.Realm, true)
	if err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("Realm not found, skipping Keycloak client deletion", "realm", obj.Spec.Realm.String())
			return nil
		}
		return err
	}

	if realm == nil || !ready {
		logger.Info("Realm not ready, skipping Keycloak client deletion", "realm", obj.Spec.Realm.String())
		return nil
	}

	realmClient, err := h.ClientFactory.ClientFor(realm.Status)
	if err != nil {
		return fmt.Errorf("failed to get keycloak client: %w", err)
	}

	getRealmClients, err := realmClient.GetRealmClients(ctx, realm.Name, obj)
	if err != nil {
		return fmt.Errorf("failed to get realm clients: %w", err)
	}

	existingClient, err := mapper.GetClient(*getRealmClients)
	if err != nil {
		return fmt.Errorf("failed to get realm client: %w", err)
	}

	err = realmClient.DeleteRealmClient(ctx, realm.Name, *existingClient.Id)
	if err != nil {
		return fmt.Errorf("failed to delete realm client: %w", err)
	}

	return nil
}

func mapToClientStatus(realmStatus *identityv1.RealmStatus, clientStatus *identityv1.ClientStatus) {
	if clientStatus == nil {
		clientStatus = &identityv1.ClientStatus{}
	}

	clientStatus.IssuerUrl = realmStatus.IssuerUrl
}
