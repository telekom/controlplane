// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package identityprovider

import (
	"github.com/telekom/controlplane/common/pkg/condition"

	"github.com/telekom/controlplane/identity/pkg/keycloak"

	identityv1 "github.com/telekom/controlplane/identity/api/v1"
)

var (
	// Ready
	doneProcessingCondition = condition.NewDoneProcessingCondition("Created IdentityProvider")
	readyCondition          = condition.NewReadyCondition("Ready", "IdentityProvider is ready")
)

func MapToIdpStatus(idpSpec *identityv1.IdentityProviderSpec) identityv1.IdentityProviderStatus {
	return identityv1.IdentityProviderStatus{
		AdminUrl:        idpSpec.AdminUrl,
		AdminTokenUrl:   keycloak.DetermineAdminTokenUrlFrom(idpSpec.AdminUrl, keycloak.MasterRealm),
		AdminConsoleUrl: keycloak.DetermineAdminConsoleUrlFrom(idpSpec.AdminUrl, keycloak.MasterRealm),
	}
}

func SetStatusReady(currentStatus *identityv1.IdentityProviderStatus, idp *identityv1.IdentityProvider) {
	idp.Status = *currentStatus
	idp.SetCondition(doneProcessingCondition)
	idp.SetCondition(readyCondition)
}
