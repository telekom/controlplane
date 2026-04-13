// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package client

import (
	"context"
	"fmt"
	"time"

	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/errors/ctrlerrors"
	"github.com/telekom/controlplane/common/pkg/handler"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/log"

	identityv1 "github.com/telekom/controlplane/identity/api/v1"
	realmHandler "github.com/telekom/controlplane/identity/internal/handler/realm"
	"github.com/telekom/controlplane/identity/pkg/keycloak"
	secrets "github.com/telekom/controlplane/secret-manager/api"
)

var _ handler.Handler[*identityv1.Client] = &HandlerClient{}

type HandlerClient struct {
	ServiceFactory keycloak.ServiceFactory
}

// NewHandlerClient creates a HandlerClient with the given ClientFactory.
func NewHandlerClient(factory keycloak.ServiceFactory) *HandlerClient {
	return &HandlerClient{ServiceFactory: factory}
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

	realmClient, err := h.ServiceFactory.ServiceFor(realm.Status)
	if err != nil {
		return fmt.Errorf("failed to get keycloak client: %w", err)
	}

	// Use a copy of the client with the resolved secret so the original
	// client.Spec.ClientSecret (which may be a secret-manager reference)
	// is never overwritten with plaintext.
	clientCopy := client.DeepCopy()
	clientCopy.Spec.ClientSecret = resolvedSecret

	err = realmClient.CreateOrReplaceClient(ctx, realm.Name, clientCopy)
	if err != nil {
		return fmt.Errorf("failed to create or update client: %w", err)
	}

	// --- Graceful rotation: check for a rotated (old) secret ---
	// After ensuring the primary client is up-to-date in Keycloak, check
	// whether a rotated secret exists (indicating a grace period is active).
	rotatedInfo, rotErr := realmClient.GetRotatedClientSecret(ctx, realm.Name, clientCopy)
	if rotErr != nil {
		return fmt.Errorf("failed to check rotated client secret: %w", rotErr)
	}

	if rotatedInfo != nil {
		// if the original client secret was a reference, we should not expose the rotated secret in the status, as it may be sensitive.
		if !secrets.IsRef(client.Spec.ClientSecret) {
			client.Status.RotatedClientSecret = rotatedInfo.Secret
		} else {
			client.Status.RotatedClientSecret = ""
		}

		// Use the expiration timestamp directly from Keycloak's client
		// attributes (epoch seconds). This is the final expiry — no grace
		// period arithmetic is needed on our side.
		if rotatedInfo.ExpiresAt != nil {
			expiresAt := time.Unix(*rotatedInfo.ExpiresAt, 0)
			client.Status.RotatedSecretExpiresAt = &metav1.Time{Time: expiresAt.UTC()}
		} else {
			client.Status.RotatedSecretExpiresAt = nil
		}
		if rotatedInfo.CreatedAt != nil {
			createdAt := time.Unix(*rotatedInfo.CreatedAt, 0)
			client.SetCondition(identityv1.NewSecretRotatedCondition(createdAt))
		}

	} else {
		// No rotation in progress — clear the status fields.
		client.Status.RotatedClientSecret = ""
		client.Status.RotatedSecretExpiresAt = nil
	}

	client.Status.ClientUid = clientCopy.Status.ClientUid
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

	svc, err := h.ServiceFactory.ServiceFor(realm.Status)
	if err != nil {
		return fmt.Errorf("failed to get keycloak client: %w", err)
	}

	err = svc.DeleteClient(ctx, realm.Name, obj)
	if err != nil {
		return fmt.Errorf("failed to delete client: %w", err)
	}

	return nil
}

func mapToClientStatus(realmStatus *identityv1.RealmStatus, clientStatus *identityv1.ClientStatus) {
	if clientStatus == nil {
		clientStatus = &identityv1.ClientStatus{}
	}

	clientStatus.IssuerUrl = realmStatus.IssuerUrl
}
