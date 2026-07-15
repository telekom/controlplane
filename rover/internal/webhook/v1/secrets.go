// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"context"
	"fmt"
	"strings"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"

	"github.com/telekom/controlplane/common/pkg/config"
	"github.com/telekom/controlplane/common/pkg/controller"
	"github.com/telekom/controlplane/common/pkg/util/labelutil"
	roverv1 "github.com/telekom/controlplane/rover/api/v1"
	secretsapi "github.com/telekom/controlplane/secret-manager/api"
)

// TODO: this list of secrets is not complete, it should be extended with all secrets that are used in the rover spec
// Secrets:
// spec.clientSecret: ClientSecret for the application (secretManager-name="clientSecret")
//
// spec.subscriptions.#.api.security.m2m.client.clientSecret: Consumer clientSecret for externalIDP
// spec.subscriptions.#.api.security.m2m.basic.password: Consumer password for externalIDP or basicAuth
//
// spec.exposures.#.api.security.m2m.externalIDP.client.clientSecret: Default clientSecret for externalIDP
// spec.exposures.#.api.security.m2m.externalIDP.basic.password: Default password for externalIDP
// spec.exposures.#.api.security.m2m.basic.password: Default password for basicAuth

func makeKey(basePath, secretName string) string {
	return fmt.Sprintf("%s/%s/%s", "externalSecrets", labelutil.NormalizeValue(basePath), secretName)
}

// TODO: refactor this to make it more generic and reusable
func GetExternalSecrets(ctx context.Context, rover *roverv1.Rover) map[string]string {
	secretMap := make(map[string]string)

	for _, subscription := range rover.Spec.Subscriptions {
		addSubscriptionExternalSecrets(secretMap, subscription)
	}

	for _, exposure := range rover.Spec.Exposures {
		addExposureExternalSecrets(secretMap, exposure)
	}

	return secretMap
}

// TODO: refactor this to make it more generic and reusable
func SetExternalSecrets(ctx context.Context, rover *roverv1.Rover, availableSecrets map[string]string) error {
	log := logr.FromContextOrDiscard(ctx)
	log.V(1).Info("Setting external secrets for rover", "availableSecrets", availableSecrets)

	for i := range rover.Spec.Subscriptions {
		setSubscriptionExternalSecrets(log, &rover.Spec.Subscriptions[i], availableSecrets)
	}

	for i := range rover.Spec.Exposures {
		setExposureExternalSecrets(log, &rover.Spec.Exposures[i], availableSecrets)
	}

	return nil
}

func addSubscriptionExternalSecrets(secretMap map[string]string, subscription roverv1.Subscription) {
	if subscription.Api == nil || !subscription.Api.HasM2M() {
		return
	}
	if subscription.Api.Security.M2M.Client != nil && subscription.Api.Security.M2M.Client.ClientSecret != "" {
		secretMap[makeKey(subscription.Api.BasePath, "clientSecret")] = subscription.Api.Security.M2M.Client.ClientSecret
	}
	if subscription.Api.Security.M2M.Basic != nil && subscription.Api.Security.M2M.Basic.Password != "" {
		secretMap[makeKey(subscription.Api.BasePath, "password")] = subscription.Api.Security.M2M.Basic.Password
	}
}

func addExposureExternalSecrets(secretMap map[string]string, exposure roverv1.Exposure) {
	if exposure.Api == nil || !exposure.Api.HasM2M() {
		return
	}
	if exposure.Api.Security.M2M.ExternalIDP != nil {
		if exposure.Api.Security.M2M.ExternalIDP.Client != nil && exposure.Api.Security.M2M.ExternalIDP.Client.ClientSecret != "" {
			secretMap[makeKey(exposure.Api.BasePath, "externalIDP/clientSecret")] = exposure.Api.Security.M2M.ExternalIDP.Client.ClientSecret
		}
		if exposure.Api.Security.M2M.ExternalIDP.Basic != nil && exposure.Api.Security.M2M.ExternalIDP.Basic.Password != "" {
			secretMap[makeKey(exposure.Api.BasePath, "externalIDP/password")] = exposure.Api.Security.M2M.ExternalIDP.Basic.Password
		}
	}
	if exposure.Api.Security.M2M.Basic != nil && exposure.Api.Security.M2M.Basic.Password != "" {
		secretMap[makeKey(exposure.Api.BasePath, "basicAuth/password")] = exposure.Api.Security.M2M.Basic.Password
	}
}

func setSubscriptionExternalSecrets(log logr.Logger, subscription *roverv1.Subscription, availableSecrets map[string]string) {
	if subscription.Api == nil || !subscription.Api.HasM2M() {
		return
	}
	if subscription.Api.Security.M2M.Client != nil && subscription.Api.Security.M2M.Client.ClientSecret != "" {
		updateSecretRef(log, availableSecrets, makeKey(subscription.Api.BasePath, "clientSecret"), "clientSecret not found in available secrets", func(secretRef string) {
			subscription.Api.Security.M2M.Client.ClientSecret = secretRef
		})
	}
	if subscription.Api.Security.M2M.Basic != nil && subscription.Api.Security.M2M.Basic.Password != "" {
		updateSecretRef(log, availableSecrets, makeKey(subscription.Api.BasePath, "password"), "password not found in available secrets", func(secretRef string) {
			subscription.Api.Security.M2M.Basic.Password = secretRef
		})
	}
}

func setExposureExternalSecrets(log logr.Logger, exposure *roverv1.Exposure, availableSecrets map[string]string) {
	if exposure.Api == nil || !exposure.Api.HasM2M() {
		return
	}
	if exposure.Api.Security.M2M.ExternalIDP != nil {
		if exposure.Api.Security.M2M.ExternalIDP.Client != nil && exposure.Api.Security.M2M.ExternalIDP.Client.ClientSecret != "" {
			updateSecretRef(log, availableSecrets, makeKey(exposure.Api.BasePath, "externalIDP/clientSecret"), "externalIDP clientSecret not found in available secrets", func(secretRef string) {
				exposure.Api.Security.M2M.ExternalIDP.Client.ClientSecret = secretRef
			})
		}
		if exposure.Api.Security.M2M.ExternalIDP.Basic != nil && exposure.Api.Security.M2M.ExternalIDP.Basic.Password != "" {
			updateSecretRef(log, availableSecrets, makeKey(exposure.Api.BasePath, "externalIDP/password"), "externalIDP password not found in available secrets", func(secretRef string) {
				exposure.Api.Security.M2M.ExternalIDP.Basic.Password = secretRef
			})
		}
	}
	if exposure.Api.Security.M2M.Basic != nil && exposure.Api.Security.M2M.Basic.Password != "" {
		updateSecretRef(log, availableSecrets, makeKey(exposure.Api.BasePath, "basicAuth/password"), "basicAuth password not found in available secrets", func(secretRef string) {
			exposure.Api.Security.M2M.Basic.Password = secretRef
		})
	}
}

func updateSecretRef(log logr.Logger, availableSecrets map[string]string, key, missingMessage string, setSecret func(string)) {
	secretRef, ok := secretsapi.FindSecretId(availableSecrets, key)
	if !ok {
		log.V(1).Info(missingMessage)
		return
	}

	setSecret(secretRef)
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
	externalSecrets := GetExternalSecrets(ctx, rover)
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
