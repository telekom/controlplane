// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package in

import (
	"github.com/pkg/errors"
	roverv1 "github.com/telekom/controlplane/rover/api/v1"

	"github.com/telekom/controlplane/rover-server/internal/api"
)

func mapSubscription(in *api.Subscription, out *roverv1.Subscription) error {
	subType, err := in.Discriminator()
	if err != nil {
		return errors.Wrap(err, "failed to get subscription type")
	}
	switch subType {
	case "api":
		apiSub, err := in.AsApiSubscription()
		if err != nil {
			return errors.Wrap(err, "failed to convert to ApiSubscription")
		}
		out.Api = mapApiSubscription(apiSub)

	case "event":
		eventSub, err := in.AsEventSubscription()
		if err != nil {
			return errors.Wrap(err, "failed to convert to EventSubscription")
		}

		out.Event = &roverv1.EventSubscription{
			EventType: eventSub.EventType,
		}

	default:
		return errors.Errorf("unknown subscription type: %s", subType)

	}

	return nil
}

func mapApiSubscription(in api.ApiSubscription) *roverv1.ApiSubscription {
	out := &roverv1.ApiSubscription{}
	out.BasePath = in.BasePath
	out.Organization = ""

	mapSubscriptionSecurity(in, out)
	mapSubscriptionTransformation(in, out)
	mapSubscriptionTraffic(in, out)

	return out
}

func mapSubscriptionSecurity(in api.ApiSubscription, out *roverv1.ApiSubscription) {
	m2mSecurity := &roverv1.SubscriberMachine2MachineAuthentication{}

	secType, err := in.Security.Discriminator()
	if err != nil {
		return
	}

	switch secType {
	case "basicAuth":
		basicAuth, err := in.Security.AsBasicAuth()
		if err != nil {
			return
		}
		m2mSecurity.Basic = &roverv1.BasicAuthCredentials{
			Username: basicAuth.Username,
			Password: basicAuth.Password,
		}
	case "oauth2":
		oauth2, err := in.Security.AsOauth2()
		if err != nil {
			return
		}
		if oauth2.ClientId != "" {
			m2mSecurity.Client = &roverv1.OAuth2ClientCredentials{
				ClientId:     oauth2.ClientId,
				ClientSecret: oauth2.ClientSecret,
				ClientKey:    oauth2.ClientKey,
			}
		}
		if oauth2.Username != "" {
			m2mSecurity.Basic = &roverv1.BasicAuthCredentials{
				Username: oauth2.Username,
				Password: oauth2.Password,
			}
		}

		m2mSecurity.Scopes = oauth2.Scopes
	}

	if m2mSecurity.Basic != nil || m2mSecurity.Client != nil || m2mSecurity.Scopes != nil {
		out.Security = &roverv1.SubscriberSecurity{
			M2M: m2mSecurity,
		}
	}
}

func mapSubscriptionTransformation(in api.ApiSubscription, out *roverv1.ApiSubscription) {}

func mapSubscriptionTraffic(in api.ApiSubscription, out *roverv1.ApiSubscription) {
	if len(in.Failover.Zones) > 0 {
		out.Traffic.Failover = &roverv1.Failover{
			Zones: in.Failover.Zones,
		}
	}
}
