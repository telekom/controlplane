// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package identityprovider

import (
	"context"
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	cc "github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/errors/ctrlerrors"
	"github.com/telekom/controlplane/common/pkg/handler"
	"github.com/telekom/controlplane/common/pkg/types"
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
	idp.SetCondition(condition.NewReadyCondition(condition.ReasonProvisioned, "IdentityProvider is ready"))

	message := fmt.Sprintf("IdentityProvider %s is ready", idp.Name)
	logger.V(1).Info(message, "IdentityProviderStatus", idp.Status)

	return nil
}

func (h *HandlerIdentityProvider) Delete(ctx context.Context, idp *identityv1.IdentityProvider) error {
	realms := &identityv1.RealmList{}
	if err := cc.ClientFromContextOrDie(ctx).List(ctx, realms,
		client.MatchingFields{"spec.identityProvider": types.ObjectRefFromObject(idp).String()},
	); err != nil {
		return fmt.Errorf("listing realms for IdentityProvider %q: %w", idp.Name, err)
	}
	if len(realms.Items) > 0 {
		return ctrlerrors.BlockedErrorf("IdentityProvider %q is still referenced by %d realm(s)", idp.Name, len(realms.Items))
	}
	return nil
}
