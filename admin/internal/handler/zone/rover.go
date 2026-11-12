// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package zone

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/telekom/controlplane/admin/internal/handler/util/naming"
	cclient "github.com/telekom/controlplane/common/pkg/client"
	cconfig "github.com/telekom/controlplane/common/pkg/config"
	ctrlerrors "github.com/telekom/controlplane/common/pkg/errors/ctrlerrors"
	"github.com/telekom/controlplane/common/pkg/types"
	"github.com/telekom/controlplane/common/pkg/util/contextutil"
	identityapi "github.com/telekom/controlplane/identity/api/v1"
	secretsapi "github.com/telekom/controlplane/secret-manager/api"
)

const sharedSecretScope = "shared"

// roverSecretManagerPath is the secret-manager path of the zone's rover client secret.
func roverSecretManagerPath(zoneName string) string {
	return fmt.Sprintf("zones/%s/admin/rover/clientSecret", zoneName)
}

// environmentSecretRef constructs the secret-manager reference of an environment secret
// in the "env:team:app:path:checksum" ID format, with empty team, app and checksum.
// It performs no lookup; without a checksum the reference resolves to the current value
// stored at path.
func environmentSecretRef(envName, path string) string {
	return secretsapi.ToRef(strings.Join([]string{envName, "", "", path, ""}, ":"))
}

// ensureRoverClient provisions the zone's single rover admin Client shared by every gateway.
//
// An existing Client keeps its credentials unless a legacy reference is migrated to
// the shared Secret-Manager scope or a follower adopts its leader's inline secret.
// Otherwise the secret already stored in the secret-manager is reused, and a new one is
// generated only when the secret-manager definitively reports it as missing. This keeps a
// stale cache read from replacing credentials that are already in use.
func ensureRoverClient(ctx context.Context, hc *HandlingContext) (*identityapi.Client, error) {
	c := cclient.ClientFromContextOrDie(ctx)
	key := client.ObjectKey{Namespace: hc.Namespace.Name, Name: naming.ForGatewayAdminClient(hc.IdentityProvider.Name)}

	existing := &identityapi.Client{}
	err := c.Get(ctx, key, existing)
	if err != nil && !apierrors.IsNotFound(err) {
		return nil, ctrlerrors.RetryableErrorf("failed to read rover admin client %s: %s", key, err)
	}

	var clientSecret string
	replaceSecret := false
	if err == nil {
		if existing.Spec.ClientSecret == "" {
			return nil, ctrlerrors.BlockedErrorf("rover admin client %s has no client secret", key)
		}
		clientSecret, replaceSecret, err = existingRoverSecret(ctx, hc, existing)
	} else {
		clientSecret, err = roverClientSecret(ctx, hc)
	}
	if err != nil {
		return nil, err
	}

	return applyRoverClient(ctx, hc, key, clientSecret, replaceSecret)
}

// existingRoverSecret returns the secret an existing rover Client should use and whether it
// replaces the current one: the shared reference after migration with Secret-Manager enabled,
// or the leader's inline secret for a follower with Secret-Manager disabled.
func existingRoverSecret(ctx context.Context, hc *HandlingContext, existing *identityapi.Client) (string, bool, error) {
	if cconfig.FeatureSecretManager.IsEnabled() {
		return migrateRoverSecret(ctx, hc, existing.Spec.ClientSecret)
	}
	if !existing.IsFollower() {
		return existing.Spec.ClientSecret, false, nil
	}

	leaderRef := existing.Status.Leader
	leaderClient := &identityapi.Client{}
	if err := hc.KubeClient.Get(ctx, leaderRef.K8s(), leaderClient); err != nil {
		if apierrors.IsNotFound(err) {
			return "", false, ctrlerrors.BlockedErrorf("rover Client leader %s is not found", leaderRef.String())
		}
		return "", false, ctrlerrors.RetryableErrorf("failed to read rover Client leader %s: %s", leaderRef.String(), err)
	}
	if secretsapi.IsRef(leaderClient.Spec.ClientSecret) {
		return "", false, ctrlerrors.BlockedErrorf("rover Client leader %s has a Secret-Manager reference while Secret-Manager is disabled", leaderRef.String())
	}
	return leaderClient.Spec.ClientSecret, leaderClient.Spec.ClientSecret != existing.Spec.ClientSecret, nil
}

// roverClientSecret returns the secret for a new rover Client: the reference of the value
// already stored in the secret-manager, a newly stored value, or an inline generated value
// when the secret-manager is disabled.
func roverClientSecret(ctx context.Context, hc *HandlingContext) (string, error) {
	if !cconfig.FeatureSecretManager.IsEnabled() {
		generated, err := secretsapi.GenerateSecret()
		if err != nil {
			return "", fmt.Errorf("generating rover client secret: %w", err)
		}
		return generated, nil
	}

	path := roverSecretManagerPath(hc.Zone.Name)
	ref := environmentSecretRef(sharedSecretScope, path)
	_, err := secretsapi.API().Get(ctx, ref)
	if err == nil {
		logr.FromContextOrDiscard(ctx).V(1).Info("Reusing stored rover admin client secret", "secretPath", path)
		return ref, nil
	}
	if !errors.Is(err, secretsapi.ErrNotFound) {
		return "", ctrlerrors.RetryableErrorf("failed to read rover client secret of zone %q: %s", hc.Zone.Name, err)
	}

	generated, err := secretsapi.GenerateSecret()
	if err != nil {
		return "", fmt.Errorf("generating rover client secret: %w", err)
	}
	if nsErr := ensureSharedNamespace(ctx, hc.KubeClient); nsErr != nil {
		return "", nsErr
	}
	_, err = secretsapi.API().UpsertEnvironment(ctx, sharedSecretScope,
		secretsapi.WithMergeStrategy(),
		secretsapi.WithSecretValue(path, generated))
	if err != nil {
		return "", ctrlerrors.RetryableErrorf("failed to store rover client secret of zone %q: %s", hc.Zone.Name, err)
	}
	logr.FromContextOrDiscard(ctx).Info("Stored new rover admin client secret", "secretPath", path)
	return ref, nil
}

// applyRoverClient creates the Client or reconciles its settings. The secret of an existing
// Client changes only when replaceSecret is true.
func applyRoverClient(ctx context.Context, hc *HandlingContext, key client.ObjectKey, clientSecret string, replaceSecret bool) (*identityapi.Client, error) {
	c := cclient.ClientFromContextOrDie(ctx)
	adminClient := &identityapi.Client{
		ObjectMeta: metav1.ObjectMeta{Name: key.Name, Namespace: key.Namespace},
	}

	mutator := func() error {
		if adminClient.Labels == nil {
			adminClient.Labels = make(map[string]string)
		}
		adminClient.Labels[cconfig.EnvironmentLabelKey] = hc.Environment.Name
		adminClient.Labels[cconfig.BuildLabelKey(zoneLabelName)] = hc.Zone.Name
		adminClient.Labels[cconfig.DomainLabelKey] = domainName

		secret := adminClient.Spec.ClientSecret
		if secret == "" || replaceSecret {
			secret = clientSecret
		}
		adminClient.Spec = identityapi.ClientSpec{
			Realm:        types.ObjectRefFromObject(hc.InternalIdentityRealm),
			ClientId:     naming.ForGatewayAdminClientId(),
			ClientSecret: secret,
		}
		return nil
	}

	if _, err := c.CreateOrUpdate(ctx, adminClient, mutator); err != nil {
		return nil, ctrlerrors.RetryableErrorf("failed to create or update rover admin client %s in zone %s: %s", adminClient.Name, hc.Zone.Name, err)
	}
	return adminClient, nil
}

// migrateRoverSecret moves a rover Client from its legacy environment-scoped secret reference
// to the shared one. It returns the reference to use and whether it replaces existingRef.
func migrateRoverSecret(ctx context.Context, hc *HandlingContext, existingRef string) (string, bool, error) {
	path := roverSecretManagerPath(hc.Zone.Name)
	if !isEnvironmentSecretRef(existingRef, hc.Environment.Name, path) {
		return existingRef, false, nil
	}

	legacyValue, err := secretsapi.Get(ctx, existingRef)
	if err != nil {
		return "", false, ctrlerrors.RetryableErrorf("failed to resolve legacy rover client secret of zone %q: %s", hc.Zone.Name, err)
	}
	sharedRef := environmentSecretRef(sharedSecretScope, path)
	sharedValue, err := secretsapi.API().Get(ctx, sharedRef)
	if err == nil {
		if sharedValue != legacyValue {
			contextutil.RecorderFromContextOrDie(ctx).Eventf(
				hc.Zone, corev1.EventTypeWarning, "SharedRoverSecretMismatch",
				"rover secret of zone %s differs from shared Secret-Manager entry; keeping environment-scoped secret",
				hc.Zone.Name,
			)
			return existingRef, false, nil
		}
		return sharedRef, true, nil
	}
	if !errors.Is(err, secretsapi.ErrNotFound) {
		return "", false, ctrlerrors.RetryableErrorf("failed to read shared rover client secret of zone %q: %s", hc.Zone.Name, err)
	}

	if err := ensureSharedNamespace(ctx, hc.KubeClient); err != nil {
		return "", false, err
	}
	if _, err := secretsapi.API().UpsertEnvironment(ctx, sharedSecretScope,
		secretsapi.WithMergeStrategy(),
		secretsapi.WithSecretValue(path, legacyValue)); err != nil {
		return "", false, ctrlerrors.RetryableErrorf("failed to seed shared rover client secret of zone %q: %s", hc.Zone.Name, err)
	}
	return sharedRef, true, nil
}

// isEnvironmentSecretRef reports whether ref points to path in the environment scope,
// with or without a checksum.
func isEnvironmentSecretRef(ref, envName, path string) bool {
	id, ok := secretsapi.FromRef(ref)
	if !ok {
		return false
	}
	scopedPath, _ := secretsapi.FromRef(environmentSecretRef(envName, path))
	return strings.HasPrefix(id, scopedPath) && !strings.Contains(strings.TrimPrefix(id, scopedPath), ":")
}

func ensureSharedNamespace(ctx context.Context, kubeClient client.Client) error {
	namespace := &corev1.Namespace{}
	err := kubeClient.Get(ctx, client.ObjectKey{Name: sharedSecretScope}, namespace)
	if err == nil {
		return nil
	}
	if !apierrors.IsNotFound(err) {
		return ctrlerrors.RetryableErrorf("failed to read shared Secret-Manager namespace: %s", err)
	}
	if err := kubeClient.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: sharedSecretScope}}); err != nil && !apierrors.IsAlreadyExists(err) {
		return ctrlerrors.RetryableErrorf("failed to create shared Secret-Manager namespace: %s", err)
	}
	return nil
}
