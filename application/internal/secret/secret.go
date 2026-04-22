// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package secret

import (
	"sync"

	"github.com/telekom/controlplane/secret-manager/api"
)

var (
	once          sync.Once
	secretManager api.SecretManager
)

var GetSecretManager = func() api.SecretManager {
	once.Do(func() {
		secretManager = api.New()
	})
	return secretManager
}

var WithSecretValue = func(name string, value any) api.OnboardingOption {
	return api.WithSecretValue(name, value)
}

const (
	ClientSecret        = "clientSecret"
	RotatedClientSecret = "rotatedClientSecret"

	KeywordRotate = api.KeywordRotate

	// SecretRotationConditionType is the condition type used to track secret rotation state on the Application CR.
	SecretRotationConditionType = "SecretRotation"
	// SecretRotationReasonInProgress indicates a rotation has been initiated but not yet fully propagated.
	SecretRotationReasonInProgress = "InProgress"
	// SecretRotationReasonSuccess indicates a rotation has been fully propagated to all sub-resources.
	SecretRotationReasonSuccess = "Success"

	// AnnotationGracefulRotation is set to "true" on the Application when a graceful
	// secret rotation (with grace period) was initiated by the webhook.
	AnnotationGracefulRotation = "application.cp.ei.telekom.de/graceful-secret-rotation"
)

var FindSecretId = api.FindSecretId
