// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package client

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/errors/ctrlerrors"
	"github.com/telekom/controlplane/common/pkg/handler"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
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

	_, realm, err := realmHandler.GetRealmByName(ctx, client.Spec.Realm, true)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return ctrlerrors.BlockedErrorf("Realm %q not found", client.Spec.Realm.String())
		}
		return err
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

	supportsRotation := realm.SupportsGracefulSecretRotation() && client.SupportsSecretRotation()

	// Determine whether forceSecretRotation was already triggered for this
	// generation. If the SecretRotation condition is True with reason
	// "Accepted" and its ObservedGeneration matches the current generation,
	// a previous reconciliation already called forceSecretRotation but the
	// subsequent PUT failed. In that case we skip the force-rotation to
	// avoid evicting the original secret from Keycloak's rotated slot.
	skipForceRotation := false
	if supportsRotation {
		if cond := meta.FindStatusCondition(client.GetConditions(), identityv1.SecretRotationConditionType); cond != nil {
			if cond.Reason == identityv1.SecretRotationReasonAccepted && cond.ObservedGeneration == client.Generation {
				skipForceRotation = true
			}
		}
		// Mark rotation as accepted before calling CreateOrReplaceClient.
		// If the handler returns an error after forceSecretRotation, the
		// controller persists this condition so that the next reconciliation
		// can detect the retry and skip the destructive rotation step.
		if !skipForceRotation {
			client.SetCondition(identityv1.NewSecretRotationAcceptedCondition())
		}
	}

	err = realmClient.CreateOrReplaceClient(ctx, realm.Name, clientCopy, keycloak.ClientUpdateOptions{
		SupportsGracefulRotation: supportsRotation,
		SkipForceRotation:        skipForceRotation,
	})
	if err != nil {
		return fmt.Errorf("failed to create or update client: %w", err)
	}

	// --- Graceful rotation: check for a rotated (old) secret ---
	// Only query rotation state when both the realm and client support it.
	if supportsRotation {
		rotationInfo, rotErr := realmClient.GetClientSecretRotationInfo(ctx, realm.Name, clientCopy)
		if rotErr != nil {
			return fmt.Errorf("failed to check client secret rotation info: %w", rotErr)
		}

		if rotationInfo.RotatedSecret != "" {
			// if the original client secret was a reference, we should not expose the rotated secret in the status, as it may be sensitive.
			if !secrets.IsRef(client.Spec.ClientSecret) {
				client.Status.RotatedClientSecret = rotationInfo.RotatedSecret
			} else {
				client.Status.RotatedClientSecret = ""
			}

			// Use the expiration timestamp directly from Keycloak's client
			// attributes (epoch seconds). This is the final expiry — no grace
			// period arithmetic is needed on our side.
			if rotationInfo.RotatedExpiresAt != nil {
				expiresAt := time.Unix(*rotationInfo.RotatedExpiresAt, 0)
				client.Status.RotatedSecretExpiresAt = &metav1.Time{Time: expiresAt.UTC()}
			} else {
				client.Status.RotatedSecretExpiresAt = nil
			}
			if rotationInfo.RotatedCreatedAt != nil {
				createdAt := time.Unix(*rotationInfo.RotatedCreatedAt, 0)
				client.SetCondition(identityv1.NewSecretRotatedCondition(createdAt))
			}

		} else {
			// No rotation in progress — clear the status fields.
			client.Status.RotatedClientSecret = ""
			client.Status.RotatedSecretExpiresAt = nil
			// Only mark as Completed if a rotation was previously active
			// (Accepted or Rotated). Avoid setting a misleading condition
			// on clients that were never rotated.
			existing := meta.FindStatusCondition(client.Status.Conditions, identityv1.SecretRotationConditionType)
			if existing != nil && (existing.Reason == identityv1.SecretRotationReasonAccepted || existing.Reason == identityv1.SecretRotationReasonRotated) {
				client.SetCondition(identityv1.NewSecretRotationCompletedCondition())
			}
		}

		// --- Current secret expiry: compute when Keycloak will auto-expire it ---
		if rotationInfo.SecretCreationTime != nil && realm.Spec.SecretRotation != nil {
			expiresAt := time.Unix(*rotationInfo.SecretCreationTime, 0).Add(realm.Spec.SecretRotation.ExpirationPeriod.Duration)
			client.Status.SecretExpiresAt = &metav1.Time{Time: expiresAt.UTC()}
		} else {
			client.Status.SecretExpiresAt = nil
		}
	} else {
		// Rotation not supported (or was just disabled) — clear any stale
		// rotation status fields from a previous opt-in.
		client.Status.RotatedClientSecret = ""
		client.Status.RotatedSecretExpiresAt = nil
		client.Status.SecretExpiresAt = nil
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
		// If the realm exists but is not ready (BlockedError), skip deletion
		// rather than blocking the finalizer — the Keycloak client will be
		// orphaned but the CR can be cleaned up.
		var be ctrlerrors.BlockedError
		if errors.As(err, &be) {
			logger.Info("Realm not ready, skipping Keycloak client deletion", "realm", obj.Spec.Realm.String())
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
