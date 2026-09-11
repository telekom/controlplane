// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package secret

import (
	"sync"

	applicationv1 "github.com/telekom/controlplane/application/api/v1"
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

var WithSecretValue = api.WithSecretValue

const (
	ClientSecret        = "clientSecret"
	RotatedClientSecret = "rotatedClientSecret"

	KeywordRotate = api.KeywordRotate

	// SecretRotationConditionType is the condition type used to track secret rotation state on the Application CR.
	SecretRotationConditionType = applicationv1.SecretRotationConditionType
	// SecretRotationReasonInProgress indicates a rotation has been initiated but not yet fully propagated.
	SecretRotationReasonInProgress = applicationv1.SecretRotationReasonInProgress
	// SecretRotationReasonSuccess indicates a rotation has been fully propagated to all sub-resources.
	SecretRotationReasonSuccess = applicationv1.SecretRotationReasonSuccess

	// AnnotationGracefulRotation is set to "true" on the Application when a graceful
	// secret rotation (with grace period) was initiated by the webhook.
	AnnotationGracefulRotation = applicationv1.AnnotationGracefulRotation
)

var FindSecretId = api.FindSecretId
