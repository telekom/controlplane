// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package identityprovider

import (
	"context"

	"github.com/pkg/errors"
	"github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/condition"
	common "github.com/telekom/controlplane/common/pkg/types"
	"k8s.io/apimachinery/pkg/api/meta"

	identityv1 "github.com/telekom/controlplane/identity/api/v1"
)

func GetIdentityProviderByName(
	ctx context.Context,
	identityProviderRef *common.ObjectRef) (*identityv1.IdentityProvider, error) {
	clientFromContext := client.ClientFromContextOrDie(ctx)

	identityProvider := &identityv1.IdentityProvider{}
	err := clientFromContext.Get(context.Background(), identityProviderRef.K8s(), identityProvider)
	if err != nil {
		return nil,
			errors.Wrapf(err, "failed to get identityProvider %s", identityProviderRef.String())
	}
	if !meta.IsStatusConditionTrue(identityProvider.GetConditions(), condition.ConditionTypeReady) {
		return nil, nil
	}
	return identityProvider, nil
}

func ObfuscateIdentityProvider(spec identityv1.IdentityProviderSpec) identityv1.IdentityProviderSpec {
	// Create a copy of the spec to avoid modifying the original
	obfuscatedSpec := spec

	// Obfuscate sensitive fields
	if obfuscatedSpec.AdminUserName != "" {
		obfuscatedSpec.AdminUserName = "****"
	}
	if obfuscatedSpec.AdminPassword != "" {
		obfuscatedSpec.AdminPassword = "****"
	}

	return obfuscatedSpec
}
