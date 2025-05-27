// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package gateway

import (
	"context"

	"github.com/pkg/errors"
	"github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/types"
	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
	secrets "github.com/telekom/controlplane/secret-manager/api"
	"k8s.io/apimachinery/pkg/api/meta"
)

func GetGatewayByRef(ctx context.Context, ref types.ObjectRef, resolveSecrets bool) (bool, *gatewayv1.Gateway, error) {
	client := client.ClientFromContextOrDie(ctx)

	gateway := &gatewayv1.Gateway{}
	err := client.Get(ctx, ref.K8s(), gateway)
	if err != nil {
		return false, nil, errors.Wrapf(err, "failed to get gateway %s", ref.String())
	}
	if !meta.IsStatusConditionTrue(gateway.GetConditions(), condition.ConditionTypeReady) {
		return false, nil, nil
	}

	if resolveSecrets {
		gateway.Spec.Admin.ClientSecret, err = secrets.Get(ctx, gateway.Spec.Admin.ClientSecret)
		if err != nil {
			return false, nil, errors.Wrap(err, "failed to get gateway client secret")
		}

		gateway.Spec.Redis.Password, err = secrets.Get(ctx, gateway.Spec.Redis.Password)
		if err != nil {
			return false, nil, errors.Wrap(err, "failed to get gateway redis password")
		}
	}

	return true, gateway, nil
}
