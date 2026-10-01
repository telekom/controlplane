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
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/telekom/controlplane/admin/internal/handler/util/naming"
	cclient "github.com/telekom/controlplane/common/pkg/client"
	cconfig "github.com/telekom/controlplane/common/pkg/config"
	ctrlerrors "github.com/telekom/controlplane/common/pkg/errors/ctrlerrors"
	"github.com/telekom/controlplane/common/pkg/types"
	identityapi "github.com/telekom/controlplane/identity/api/v1"
	secretsapi "github.com/telekom/controlplane/secret-manager/api"
)

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
// An existing Client keeps its credentials; only its non-secret settings are reconciled.
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
	if err == nil {
		if existing.Spec.ClientSecret == "" {
			return nil, ctrlerrors.BlockedErrorf("rover admin client %s has no client secret", key)
		}
		clientSecret = existing.Spec.ClientSecret
	} else {
		clientSecret, err = roverClientSecret(ctx, hc)
		if err != nil {
			return nil, err
		}
	}

	return applyRoverClient(ctx, hc, key, clientSecret)
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
	ref := environmentSecretRef(hc.Environment.Name, path)
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
	available, err := secretsapi.API().UpsertEnvironment(ctx, hc.Environment.Name,
		secretsapi.WithMergeStrategy(),
		secretsapi.WithSecretValue(path, generated))
	if err != nil {
		return "", ctrlerrors.RetryableErrorf("failed to store rover client secret of zone %q: %s", hc.Zone.Name, err)
	}
	stored, found := secretsapi.FindSecretId(available, path)
	if !found {
		return "", ctrlerrors.RetryableErrorf("rover client secret reference of zone %q not returned by secret-manager", hc.Zone.Name)
	}
	logr.FromContextOrDiscard(ctx).Info("Stored new rover admin client secret", "secretPath", path)
	return stored, nil
}

// applyRoverClient creates the Client or reconciles its non-secret settings. The secret of
// an existing Client is never replaced; if the Client appeared since it was read as missing,
// the create fails with AlreadyExists and the reconciliation is retried.
func applyRoverClient(ctx context.Context, hc *HandlingContext, key client.ObjectKey, clientSecret string) (*identityapi.Client, error) {
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
		if secret == "" {
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
