// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package out

import (
	"reflect"

	"github.com/pkg/errors"
	roverv1 "github.com/telekom/controlplane/rover/api/v1"

	"github.com/telekom/controlplane/rover-server/internal/api"
)

func mapSubscription(in *roverv1.Subscription, out *api.Subscription) error {
	if in.Api != nil {
		if err := out.FromApiSubscription(mapApiSubscription(in.Api)); err != nil {
			return errors.Wrap(err, "failed to map api subscription")
		}

	} else if in.Event != nil {
		if err := out.FromEventSubscription(mapEventSubscription(in.Event)); err != nil {
			return errors.Wrap(err, "failed to map event subscription")
		}
	} else {
		return errors.Errorf("unknown subscription type: %s", in.Type())
	}

	return nil
}

func mapEventSubscription(in *roverv1.EventSubscription) api.EventSubscription {
	return api.EventSubscription{
		EventType: in.EventType,
	}
}

func mapApiSubscription(in *roverv1.ApiSubscription) api.ApiSubscription {
	apiSub := api.ApiSubscription{
		BasePath: in.BasePath,
	}

	mapSubscriptionSecurity(in, &apiSub)
	mapSubscriptionTransformation(in, &apiSub)
	mapSubscriptionTraffic(in, &apiSub)

	return apiSub
}

func mapSubscriptionSecurity(in *roverv1.ApiSubscription, out *api.ApiSubscription) {
	if in.Security == nil || in.Security.M2M == nil {
		return
	}

	m2m := in.Security.M2M
	if m2m.Basic != nil {
		basicAuth := api.BasicAuth{
			Username: m2m.Basic.Username,
			Password: m2m.Basic.Password,
		}
		out.Security = api.Security{}
		out.Security.FromBasicAuth(basicAuth)
		return
	}

	oauth2 := api.Oauth2{}

	if m2m.Client != nil {
		oauth2.ClientId = m2m.Client.ClientId
		oauth2.ClientSecret = m2m.Client.ClientSecret
		oauth2.ClientKey = m2m.Client.ClientKey
	}

	if len(m2m.Scopes) > 0 {
		oauth2.Scopes = m2m.Scopes
	}

	if !reflect.ValueOf(oauth2).IsZero() {
		out.Security = api.Security{}
		out.Security.FromOauth2(oauth2)
	}
}

func mapSubscriptionTransformation(in *roverv1.ApiSubscription, out *api.ApiSubscription) {
	// No implementation in the 'in' side either
}

func mapSubscriptionTraffic(in *roverv1.ApiSubscription, out *api.ApiSubscription) {
	if in.Traffic.Failover != nil {
		out.Failover = api.Failover{
			Zones: in.Traffic.Failover.Zones,
		}
	}

	// todo: ratelimit (ignore for now until implementation is clear)
}
