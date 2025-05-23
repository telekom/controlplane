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

const (
	TeamToken    = "teamToken"
	ClientSecret = "clientSecret"

	KeywordRotate = api.KeywordRotate
)

var FindSecretId = api.FindSecretId
