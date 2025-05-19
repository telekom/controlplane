// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import "github.com/telekom/controlplane/secret-manager/pkg/backend"

type Controller interface {
	SecretsController
	OnboardController
}

type controller struct {
	SecretsController
	OnboardController
}

func NewController[T backend.SecretId, S backend.Secret[T]](b backend.Backend[T, S], o backend.Onboarder) Controller {
	return &controller{
		SecretsController: NewSecretsController(b),
		OnboardController: NewOnboardController(o),
	}
}
