// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package gateway

import (
	"context"

	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/api/meta"

	cclient "github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/types"
	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
	secrets "github.com/telekom/controlplane/secret-manager/api"
)

// GetGatewayByRef returns the Gateway and whether it is ready. A not-ready Gateway is
// returned without resolved secrets.
func GetGatewayByRef(ctx context.Context, ref types.ObjectRef, resolveSecrets bool) (bool, *gatewayv1.Gateway, error) {
	client := cclient.ClientFromContextOrDie(ctx)

	gateway := &gatewayv1.Gateway{}
	err := client.Get(ctx, ref.K8s(), gateway)
	if err != nil {
		return false, nil, errors.Wrapf(err, "failed to get gateway %s", ref.String())
	}
	ready := meta.IsStatusConditionTrue(gateway.GetConditions(), condition.ConditionTypeReady)
	if !ready {
		// Deletion needs the Gateway even when it is not ready.
		return false, gateway, nil
	}

	if resolveSecrets {
		gateway.Spec.Admin.ClientSecret, err = secrets.Get(ctx, gateway.Spec.Admin.ClientSecret)
		if err != nil {
			return false, nil, errors.Wrap(err, "failed to get gateway client secret")
		}

		if gateway.Spec.Redis != nil {
			gateway.Spec.Redis.Password, err = secrets.Get(ctx, gateway.Spec.Redis.Password)
			if err != nil {
				return false, nil, errors.Wrap(err, "failed to get gateway redis password")
			}
		}
	}

	return true, gateway, nil
}
