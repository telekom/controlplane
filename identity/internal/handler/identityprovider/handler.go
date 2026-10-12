// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package identityprovider

import (
	"context"
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/errors/ctrlerrors"
	"github.com/telekom/controlplane/common/pkg/handler"
	identityv1 "github.com/telekom/controlplane/identity/api/v1"
	"github.com/telekom/controlplane/identity/pkg/keycloak"
)

var _ handler.Handler[*identityv1.IdentityProvider] = &HandlerIdentityProvider{}

type HandlerIdentityProvider struct {
	// CA is the shared CA cache that also serves client acquisition.
	CA keycloak.CAResolver
}

func (h *HandlerIdentityProvider) CreateOrUpdate(ctx context.Context, idp *identityv1.IdentityProvider) error {
	logger := log.FromContext(ctx)
	if idp == nil {
		return fmt.Errorf("IdentityProvider is nil")
	}

	if ref := idp.Spec.TLS.Ref(); ref != nil {
		if err := keycloak.RequireHTTPS(idp.Spec.AdminUrl); err != nil {
			blocked := ctrlerrors.BlockedErrorf("IdentityProvider %s: %s", idp.Name, err)
			idp.SetCondition(identityv1.NewTrustNotReadyCondition(blocked.Error()))
			return blocked
		}
	}
	if err := keycloak.ValidateProviderCA(ctx, h.CA, idp.Namespace, idp.Spec.TLS.Ref()); err != nil {
		idp.SetCondition(identityv1.NewTrustNotReadyCondition(err.Error()))
		return err
	}
	idp.SetCondition(identityv1.NewTrustReadyCondition("Provider trust resolved"))

	idp.Status.AdminUrl = idp.Spec.AdminUrl
	idp.Status.AdminTokenUrl = keycloak.DetermineAdminTokenUrlFrom(idp.Spec.AdminUrl, keycloak.MasterRealm)
	idp.Status.AdminConsoleUrl = keycloak.DetermineAdminConsoleUrlFrom(idp.Spec.AdminUrl, keycloak.MasterRealm)

	idp.SetCondition(condition.NewDoneProcessingCondition("Created IdentityProvider"))
	idp.SetCondition(condition.NewReadyCondition(condition.ReasonProvisioned, "IdentityProvider is ready"))

	message := fmt.Sprintf("IdentityProvider %s is ready", idp.Name)
	logger.V(1).Info(message, "IdentityProviderStatus", idp.Status)

	return nil
}

func (h *HandlerIdentityProvider) Delete(ctx context.Context, obj *identityv1.IdentityProvider) error {
	return nil
}
