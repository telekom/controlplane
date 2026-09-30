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
	"github.com/telekom/controlplane/gateway/internal/features"
	"k8s.io/apimachinery/pkg/api/meta"
)

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

func usesPasswordGrant(security *v1.Security) bool {
	return security.HasM2MExternalIDP() && security.M2M.ExternalIDP.GrantType == v1.GrantTypePassword
}

func hasProviderPasswordFallback(security *v1.Security) bool {
	if !usesPasswordGrant(security) {
		return false
	}
	credentials := security.M2M.ExternalIDP.Basic
	return credentials != nil && credentials.Username != "" && credentials.Password != ""
}

func defaultConsumersLackPasswordCredentials(route *v1.Route, passwordGrant, providerFallback bool) bool {
	if !passwordGrant || providerFallback {
		return false
	}
	// On primary routes without failover, last-mile security restores the original
	// consumer token before backend authentication. The gateway group only authorizes
	// mesh transport; other default consumers still need provider credentials.
	for _, consumer := range route.Spec.Security.DefaultConsumers {
		if consumer == v1.GatewayConsumerName && route.IsPrimary() && route.Spec.Traffic.Failover == nil {
			continue
		}
		return true
	}
	return false
}

func consumerRequiresPasswordGrant(consumer *v1.ConsumeRoute) bool {
	return consumer.HasM2MBasic() && len(consumer.Spec.Security.M2M.Scopes) > 0
}

func hasUsablePasswordCredentials(consumer *v1.ConsumeRoute, providerFallback bool) bool {
	if !consumer.HasM2M() {
		return providerFallback
	}
	m2m := consumer.Spec.Security.M2M
	if m2m.Client != nil {
		return false
	}
	return providerFallback || (m2m.Basic != nil && m2m.Basic.Username != "" && m2m.Basic.Password != "")
}

func validatePasswordGrantConsumers(ctx context.Context, route *v1.Route, builder features.FeaturesBuilder, consumers []v1.ConsumeRoute) (bool, error) {
	if !route.IsPrimary() && !route.IsFailoverSecondary() {
		return false, nil
	}
	security := route.Spec.Security
	if route.IsFailoverSecondary() && route.Spec.Traffic.Failover != nil {
		security = route.Spec.Traffic.Failover.Security
	}
	passwordGrant := usesPasswordGrant(&security)
	providerFallback := hasProviderPasswordFallback(&security)
	if passwordGrant && !providerFallback && route.Spec.Security.DisableAccessControl {
		return passwordGrant, invalidateRoute(ctx, route, builder, "Defaultless password routes require access control", false)
	}
	for index := range consumers {
		consumer := &consumers[index]
		if controller.IsBeingDeleted(consumer) {
			continue
		}
		if consumerRequiresPasswordGrant(consumer) && !passwordGrant {
			return passwordGrant, invalidateRoute(ctx, route, builder, "Consumer credentials require an external IDP password grant", false)
		}
		if passwordGrant && !hasUsablePasswordCredentials(consumer, providerFallback) {
			return passwordGrant, invalidateRoute(ctx, route, builder, "Password route has consumers without usable credentials", false)
		}
	}
	if defaultConsumersLackPasswordCredentials(route, passwordGrant, providerFallback) {
		return passwordGrant, invalidateRoute(ctx, route, builder, "Password route has consumers without usable credentials", false)
	}
	return passwordGrant, nil
}

func invalidateRoute(ctx context.Context, route *v1.Route, builder features.FeaturesBuilder, message string, retryable bool) error {
	reason := condition.ReasonValidationFailed
	if retryable {
		reason = condition.ReasonError
	}
	ready := condition.NewNotReadyCondition(reason, message)
	ready.ObservedGeneration = route.Generation
	route.SetCondition(ready)
	if err := builder.GetKongClient().DeleteRoute(ctx, route); err != nil {
		return ctrlerrors.RetryableErrorf("could not confirm invalidation of route %s: %v", route.Name, err)
	}
	route.Status.Consumers = nil
	if retryable {
		return ctrlerrors.RetryableErrorf("route %s publication failed and was invalidated", route.Name)
	}
	return ctrlerrors.BlockedErrorf("%s on route %s", message, route.Name)
}
