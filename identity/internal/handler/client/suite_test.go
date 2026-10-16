// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package client

import (
	"context"
	"errors"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	identityv1 "github.com/telekom/controlplane/identity/api/v1"
	"github.com/telekom/controlplane/identity/pkg/keycloak"
)

// unusedFactory fails if a test unexpectedly requests a Keycloak service.
var unusedFactory = keycloak.ServiceFactoryFunc(func(context.Context, *identityv1.Realm, identityv1.RealmStatus) (keycloak.KeycloakService, func(), error) {
	return nil, nil, errors.New("unexpected keycloak service request")
})

func TestClientHandler(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Client Handler Suite")
}
