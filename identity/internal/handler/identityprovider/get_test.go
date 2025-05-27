// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package identityprovider

import (
	"testing"

	"github.com/stretchr/testify/assert"

	identityv1 "github.com/telekom/controlplane/identity/api/v1"
)

func TestObfuscateIdentityProviderMapsCorrectly(t *testing.T) {
	idpSpec := identityv1.IdentityProviderSpec{
		AdminUrl:      "https://admin.example.com",
		AdminClientId: "admin-client-id",
		AdminUserName: "admin",
		AdminPassword: "password",
	}

	obfuscatedSpec := ObfuscateIdentityProvider(idpSpec)

	assert.Equal(t, "https://admin.example.com", obfuscatedSpec.AdminUrl)
	assert.Equal(t, "admin-client-id", obfuscatedSpec.AdminClientId)
	assert.Equal(t, "****", obfuscatedSpec.AdminUserName)
	assert.Equal(t, "****", obfuscatedSpec.AdminPassword)
}
