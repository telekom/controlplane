// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
	apierrors "k8s.io/apimachinery/pkg/api/errors"

	"github.com/telekom/controlplane/common/pkg/config"
	"github.com/telekom/controlplane/common/pkg/controller"
	"github.com/telekom/controlplane/common/pkg/util/labelutil"
	roverv1 "github.com/telekom/controlplane/rover/api/v1"
	secretsapi "github.com/telekom/controlplane/secret-manager/api"
)

func makeKey(basePath, secretName string) string {
	return fmt.Sprintf("%s/%s/%s", "externalSecrets", labelutil.NormalizeValue(basePath), secretName)
}

func basePathFromJSONPath(data []byte, path string) string {
	if idx := strings.Index(path, ".security."); idx >= 0 {
		return gjson.GetBytes(data, path[:idx]+".basePath").String()
	}
	return ""
}

// secretJsonPath maps a secret name (the key suffix used towards the Secret
// Manager) to the JSON path where the plaintext value lives in the Rover.
type secretJsonPath struct {
	SecretName string
	JsonPath   string
}

// secretPathTemplates define the secrets relative to a resource variant.
// The %[1]s placeholder is replaced with the variant (e.g. "api" or "ai"),
// so the paths only have to be declared once for all variants.
//
//nolint:gosec // G101: these are JSON path expressions, not hardcoded credentials
var secretPathTemplates = []secretJsonPath{
	{"clientSecret", "spec.subscriptions.#.%[1]s.security.m2m.client.clientSecret"},
	{"refreshToken", "spec.subscriptions.#.%[1]s.security.m2m.client.refreshToken"},
	{"password", "spec.subscriptions.#.%[1]s.security.m2m.basic.password"},
	{"externalIDP/clientSecret", "spec.exposures.#.%[1]s.security.m2m.externalIDP.client.clientSecret"},
	{"externalIDP/refreshToken", "spec.exposures.#.%[1]s.security.m2m.externalIDP.client.refreshToken"},
	{"externalIDP/password", "spec.exposures.#.%[1]s.security.m2m.externalIDP.basic.password"},
	{"basicAuth/password", "spec.exposures.#.%[1]s.security.m2m.basic.password"},
}

// secretVariants are the resource variants that carry secrets.
var secretVariants = []string{"api", "ai"}

// secretJsonPaths is the concrete list of secrets across all variants,
// expanded from secretPathTemplates.
var secretJsonPaths = buildSecretJsonPaths(secretVariants...)

func buildSecretJsonPaths(variants ...string) []secretJsonPath {
	paths := make([]secretJsonPath, 0, len(secretPathTemplates)*len(variants))
	for _, variant := range variants {
		for _, tmpl := range secretPathTemplates {
			paths = append(paths, secretJsonPath{
				SecretName: tmpl.SecretName,
				JsonPath:   fmt.Sprintf(tmpl.JsonPath, variant),
			})
		}
	}
	return paths
}

// GetExtractSecrets generically extracts all non-empty, non-ref secret values
// from the Rover object using the defined JSON paths.
func GetExternalSecrets(_ context.Context, rover *roverv1.Rover) (map[string]string, error) {
	b, err := json.Marshal(rover)
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal rover")
	}

	secrets := make(map[string]string)
	for _, sp := range secretJsonPaths {
		result := gjson.GetBytes(b, sp.JsonPath)
		if result.IsArray() {
			for _, path := range result.Paths(string(b)) {
				val := gjson.GetBytes(b, path).String()
				if val != "" && !secretsapi.IsRef(val) {
					basePath := basePathFromJSONPath(b, path)
					secrets[makeKey(basePath, sp.SecretName)] = val
				}
			}
		}
	}
	return secrets, nil
}

func SetExternalSecrets(ctx context.Context, rover *roverv1.Rover, availableSecrets map[string]string) error {
	log := logr.FromContextOrDiscard(ctx)
	log.V(1).Info("Setting external secrets for rover", "availableSecrets", availableSecrets)

	b, err := json.Marshal(rover)
	if err != nil {
		return errors.Wrap(err, "failed to marshal rover")
	}

	for _, sp := range secretJsonPaths {
		result := gjson.GetBytes(b, sp.JsonPath)
		//nolint:nestif // sequential guard clauses within array expansion
		if result.IsArray() {
			for _, path := range result.Paths(string(b)) {
				val := gjson.GetBytes(b, path).String()
				if val != "" {
					basePath := basePathFromJSONPath(b, path)
					secretRef, ok := secretsapi.FindSecretId(availableSecrets, makeKey(basePath, sp.SecretName))
					if !ok {
						log.V(1).Info("secret not found in available secrets", "key", sp.SecretName, "basePath", basePath)
						continue
					}
					b, err = sjson.SetBytes(b, path, secretRef)
					if err != nil {
						return errors.Wrapf(err, "failed to set secret ref at path %s", path)
					}
				}
			}
		}
	}

	return json.Unmarshal(b, rover)
}

func OnboardApplication(ctx context.Context, rover *roverv1.Rover, secretManager secretsapi.SecretManager) error {
	log := logr.FromContextOrDiscard(ctx)

	if !config.FeatureSecretManager.IsEnabled() {
		log.Info("Secret Manager integration is disabled, skipping onboarding")
		return nil
	}

	if secretManager == nil {
		return nil
	}

	envName, ok := controller.GetEnvironment(rover)
	if !ok {
		return apierrors.NewBadRequest("environment label is required")
	}

	// TODO: Get team ID from rover or context
	parts := strings.SplitN(rover.GetNamespace(), "--", 2)
	teamId := parts[1]
	appId := rover.GetName()

	options := []secretsapi.OnboardingOption{}
	if rover.Spec.ClientSecret != "" && !secretsapi.IsRef(rover.Spec.ClientSecret) {
		log.V(1).Info("Setting clientSecret for application")
		options = append(options, secretsapi.WithSecretValue("clientSecret", rover.Spec.ClientSecret))
	}
	externalSecrets, err := GetExternalSecrets(ctx, rover)
	if err != nil {
		log.Error(err, "Failed to external secrets", "envName", envName, "teamId", teamId, "appId", appId)
		return apierrors.NewInternalError(errors.New("failed to load external secrets"))
	}
	if len(externalSecrets) > 0 {
		for key, value := range externalSecrets {
			if secretsapi.IsRef(value) {
				log.V(1).Info("Skipping external secret as it is a reference", "key", key)
				continue
			}
			log.V(1).Info("Setting external secret for application", "key", key)
			options = append(options, secretsapi.WithSecretValue(key, value))
		}
	}

	log.V(0).Info("Onboarding application", "envName", envName, "teamId", teamId, "appId", appId, "externalSecrets", len(externalSecrets))

	availableSecrets, err := secretManager.UpsertApplication(ctx, envName, teamId, appId, options...)
	if err != nil {
		log.Error(err, "Failed to onboard application", "envName", envName, "teamId", teamId, "appId", appId)
		return apierrors.NewInternalError(errors.New("failed to onboard application"))
	}

	rover.Spec.ClientSecret, ok = secretsapi.FindSecretId(availableSecrets, "clientSecret")
	if !ok {
		log.Info("clientSecret not found in available secrets", "availableSecrets", availableSecrets)
		return apierrors.NewInternalError(errors.New("clientSecret not found in available secrets"))
	}

	if err := SetExternalSecrets(ctx, rover, availableSecrets); err != nil {
		log.Error(err, "Failed to set external secrets for application", "availableSecrets", availableSecrets)
		return apierrors.NewInternalError(errors.New("failed to set external secrets for application"))
	}

	log.V(0).Info("Successfully onboarded application", "envName", envName, "teamId", teamId, "appId", appId)

	return nil
}
