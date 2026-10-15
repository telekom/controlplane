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

	if apierrors.IsNotFound(err) {
		clientSecret, secretErr := roverClientSecret(ctx, hc)
		if secretErr != nil {
			return nil, secretErr
		}
		return applyRoverClient(ctx, hc, key, "", clientSecret)
	}

	if existing.Spec.ClientSecret == "" {
		return nil, ctrlerrors.BlockedErrorf("rover admin client %s has no client secret", key)
	}
	if !cconfig.FeatureSecretManager.IsEnabled() || existing.Spec.ClientSecret != roverLookupRef(hc) {
		return applyRoverClient(ctx, hc, key, "", "")
	}

	// The Client holds the checksum-less lookup reference; replace it with the canonical
	// reference of the same stored secret.
	canonical, err := resolveRoverSecretRef(ctx, hc)
	if err != nil {
		return nil, err
	}
	logr.FromContextOrDiscard(ctx).Info("Normalizing rover admin client secret reference", "secretPath", roverSecretManagerPath(hc.Zone.Name))
	return applyRoverClient(ctx, hc, key, existing.Spec.ClientSecret, canonical)
}

// roverLookupRef is the checksum-less reference used only to look up the zone's stored
// rover client secret.
func roverLookupRef(hc *HandlingContext) string {
	return environmentSecretRef(hc.Environment.Name, roverSecretManagerPath(hc.Zone.Name))
}

// resolveRoverSecretRef returns the canonical reference of the stored rover client secret.
// A missing secret is returned as secretsapi.ErrNotFound.
func resolveRoverSecretRef(ctx context.Context, hc *HandlingContext) (string, error) {
	resolved, err := secretsapi.API().Resolve(ctx, roverLookupRef(hc))
	if errors.Is(err, secretsapi.ErrNotFound) {
		return "", err
	}
	if err != nil {
		return "", ctrlerrors.RetryableErrorf("failed to read rover client secret of zone %q: %s", hc.Zone.Name, err)
	}
	if !secretsapi.IsRef(resolved.Ref) {
		return "", ctrlerrors.RetryableErrorf("rover client secret reference of zone %q not returned by secret-manager", hc.Zone.Name)
	}
	return resolved.Ref, nil
}

// roverClientSecret returns the secret for a new rover Client: the canonical reference of
// the value already stored in the secret-manager, a newly stored value, or an inline
// generated value when the secret-manager is disabled.
func roverClientSecret(ctx context.Context, hc *HandlingContext) (string, error) {
	if !cconfig.FeatureSecretManager.IsEnabled() {
		generated, err := secretsapi.GenerateSecret()
		if err != nil {
			return "", fmt.Errorf("generating rover client secret: %w", err)
		}
		return generated, nil
	}

	path := roverSecretManagerPath(hc.Zone.Name)
	ref, err := resolveRoverSecretRef(ctx, hc)
	if err == nil {
		logr.FromContextOrDiscard(ctx).V(1).Info("Reusing stored rover admin client secret", "secretPath", path)
		return ref, nil
	}
	if !errors.Is(err, secretsapi.ErrNotFound) {
		return "", err
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

// applyRoverClient creates the Client or reconciles its non-secret settings. The secret is
// set to clientSecret only when the Client has none or still holds expectedSecret (if
// non-empty); any other secret of an existing Client is kept. If the Client appeared since
// it was read as missing, the create fails with AlreadyExists and the reconciliation is retried.
func applyRoverClient(ctx context.Context, hc *HandlingContext, key client.ObjectKey, expectedSecret, clientSecret string) (*identityapi.Client, error) {
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
		if secret == "" || (expectedSecret != "" && secret == expectedSecret) {
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
