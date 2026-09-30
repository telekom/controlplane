// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package route

import (
	"context"

	"github.com/pkg/errors"
	"github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/controller"
	"github.com/telekom/controlplane/common/pkg/errors/ctrlerrors"
	"github.com/telekom/controlplane/common/pkg/types"
	v1 "github.com/telekom/controlplane/gateway/api/v1"
	"k8s.io/apimachinery/pkg/api/meta"
)

const basicWithScopesPolicyMessage = "Consumer username/password with scopes requires an external IDP grant type \"password\""

func GetRouteByRef(ctx context.Context, ref types.ObjectRef) (bool, *v1.Route, error) {
	client, _ := client.ClientFromContext(ctx)

	route := &v1.Route{}
	err := client.Get(ctx, ref.K8s(), route)
	if err != nil {
		return false, nil, errors.Wrap(err, "failed to get route")
	}
	if !meta.IsStatusConditionTrue(route.GetConditions(), condition.ConditionTypeReady) {
		return false, route, nil
	}
	return true, route, nil
}

func validateBasicWithScopes(route *v1.Route, consumers []v1.ConsumeRoute) error {
	if !route.IsPrimary() && !route.IsFailoverSecondary() {
		return nil
	}
	security := route.Spec.Security
	if route.IsFailoverSecondary() && route.Spec.Traffic.Failover != nil {
		security = route.Spec.Traffic.Failover.Security
	}
	if security.HasM2MExternalIDP() && security.M2M.ExternalIDP.GrantType == v1.GrantTypePassword {
		return nil
	}
	for index := range consumers {
		consumer := &consumers[index]
		if controller.IsBeingDeleted(consumer) {
			continue
		}
		if consumer.HasM2MBasic() && len(consumer.Spec.Security.M2M.Scopes) > 0 {
			route.SetCondition(condition.NewNotReadyCondition(condition.ReasonValidationFailed, basicWithScopesPolicyMessage))
			return ctrlerrors.BlockedErrorf("%s on route %s", basicWithScopesPolicyMessage, route.Name)
		}
	}
	return nil
}
