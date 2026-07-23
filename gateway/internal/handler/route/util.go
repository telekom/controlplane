// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package route

import (
	"context"

	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/api/meta"

	"github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/types"
	v1 "github.com/telekom/controlplane/gateway/api/v1"
)

func GetRouteByRef(ctx context.Context, ref types.ObjectRef) (bool, *v1.Route, error) {
	kubeClient, _ := client.ClientFromContext(ctx)

	route := &v1.Route{}
	err := kubeClient.Get(ctx, ref.K8s(), route)
	if err != nil {
		return false, nil, errors.Wrap(err, "failed to get route")
	}
	if !meta.IsStatusConditionTrue(route.GetConditions(), condition.ConditionTypeReady) {
		return false, route, nil
	}
	return true, route, nil
}
