// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package in

import (
	"encoding/json"

	"github.com/pkg/errors"
	roverv1 "github.com/telekom/controlplane/rover/api/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"

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

		out.Event = mapEventSubscription(eventSub)

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

func mapEventSubscription(in api.EventSubscription) *roverv1.EventSubscription {
	out := &roverv1.EventSubscription{
		EventType: in.EventType,
	}

	// Map delivery configuration
	out.Delivery = roverv1.EventDelivery{
		Type:    roverv1.EventDeliveryType(in.DeliveryType),
		Payload: roverv1.EventPayloadType(in.PayloadType),
	}
	if in.Callback != "" {
		out.Delivery.Callback = in.Callback
	}
	if in.EventRetentionTime != "" {
		out.Delivery.EventRetentionTime = in.EventRetentionTime
	}
	if in.CircuitBreakerOptOut {
		out.Delivery.CircuitBreakerOptOut = in.CircuitBreakerOptOut
	}
	if in.RetryableStatusCodes != nil {
		out.Delivery.RetryableStatusCodes = in.RetryableStatusCodes
	}
	if in.RedeliveriesPerSecond != 0 {
		redeliveries := in.RedeliveriesPerSecond
		out.Delivery.RedeliveriesPerSecond = &redeliveries
	}
	if in.EnforceGetHttpRequestMethodForHealthCheck {
		out.Delivery.EnforceGetHttpRequestMethodForHealthCheck = in.EnforceGetHttpRequestMethodForHealthCheck
	}

	// Map trigger
	if in.Trigger.ResponseFilter != nil || in.Trigger.SelectionFilter != nil || in.Trigger.AdvancedSelectionFilter != nil {
		out.Trigger = mapEventTriggerForSubscription(in.Trigger)
	}

	// Map scopes
	if in.Scopes != nil {
		out.Scopes = in.Scopes
	}

	return out
}

func mapEventTriggerForSubscription(in api.EventTrigger) *roverv1.EventTrigger {
	out := &roverv1.EventTrigger{}

	if in.ResponseFilter != nil {
		out.ResponseFilter = &roverv1.EventResponseFilter{
			Paths: in.ResponseFilter,
			Mode:  roverv1.EventResponseFilterMode(in.ResponseFilterMode),
		}
	}

	if in.SelectionFilter != nil || in.AdvancedSelectionFilter != nil {
		out.SelectionFilter = &roverv1.EventSelectionFilter{}
		if in.SelectionFilter != nil {
			out.SelectionFilter.Attributes = in.SelectionFilter
		}
		if in.AdvancedSelectionFilter != nil {
			jsonBytes, err := json.Marshal(in.AdvancedSelectionFilter)
			if err == nil {
				out.SelectionFilter.Expression = &apiextensionsv1.JSON{Raw: jsonBytes}
			}
		}
	}

	return out
}
