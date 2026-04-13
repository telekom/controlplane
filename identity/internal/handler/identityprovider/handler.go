// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package identityprovider

import (
	"context"
	"fmt"

	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"

	identityv1 "github.com/telekom/controlplane/identity/api/v1"
	"github.com/telekom/controlplane/identity/pkg/keycloak"
)

var _ handler.Handler[*identityv1.IdentityProvider] = &HandlerIdentityProvider{}

type HandlerIdentityProvider struct{}

func (h *HandlerIdentityProvider) CreateOrUpdate(ctx context.Context, idp *identityv1.IdentityProvider) error {
	logger := log.FromContext(ctx)
	if idp == nil {
		return fmt.Errorf("IdentityProvider is nil")
	}

	idp.Status.AdminUrl = idp.Spec.AdminUrl
	idp.Status.AdminTokenUrl = keycloak.DetermineAdminTokenUrlFrom(idp.Spec.AdminUrl, keycloak.MasterRealm)
	idp.Status.AdminConsoleUrl = keycloak.DetermineAdminConsoleUrlFrom(idp.Spec.AdminUrl, keycloak.MasterRealm)

	idp.SetCondition(condition.NewDoneProcessingCondition("Created IdentityProvider"))
	idp.SetCondition(condition.NewReadyCondition("Ready", "IdentityProvider is ready"))

	var message = fmt.Sprintf("IdentityProvider %s is ready", idp.Name)
	logger.V(1).Info(message, "IdentityProviderStatus", idp.Status)

	return nil
}

func (h *HandlerIdentityProvider) Delete(ctx context.Context, obj *identityv1.IdentityProvider) error {
	return nil
}
