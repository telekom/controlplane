// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package identityprovider

import (
	"context"

	"github.com/pkg/errors"
	"github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/errors/ctrlerrors"
	common "github.com/telekom/controlplane/common/pkg/types"

	identityv1 "github.com/telekom/controlplane/identity/api/v1"
)

func GetIdentityProviderByName(
	ctx context.Context,
	identityProviderRef *common.ObjectRef) (*identityv1.IdentityProvider, error) {
	clientFromContext := client.ClientFromContextOrDie(ctx)

	identityProvider := &identityv1.IdentityProvider{}
	err := clientFromContext.Get(ctx, identityProviderRef.K8s(), identityProvider)
	if err != nil {
		return nil,
			errors.Wrapf(err, "failed to get identityProvider %s", identityProviderRef.String())
	}

	if err := condition.EnsureReady(identityProvider); err != nil {
		return nil, ctrlerrors.BlockedErrorf("IDP %q is not ready", identityProviderRef.String())
	}

	return identityProvider, nil
}
