// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package out

import (
	roverv1 "github.com/telekom/controlplane/rover/api/v1"

	"github.com/telekom/controlplane/rover-server/internal/api"
)

func mapListeners(in *roverv1.Rover, out *api.Rover) {
	if len(in.Spec.Listeners) == 0 {
		return
	}
	out.Listeners = make([]api.RoverListener, len(in.Spec.Listeners))
	for i := range in.Spec.Listeners {
		out.Listeners[i] = mapListener(&in.Spec.Listeners[i])
	}
}

func mapListener(in *roverv1.RoverListener) api.RoverListener {
	out := api.RoverListener{
		Consumer: in.Consumer,
		Provider: in.Provider,
	}

	if in.ApiBasePath != "" {
		out.ApiBasePath = in.ApiBasePath
	}
	if in.EventType != "" {
		out.EventType = in.EventType
	}
	if in.RequestFilter != nil {
		out.RequestFilter = mapListenerFilter(in.RequestFilter)
	}
	if in.ResponseFilter != nil {
		out.ResponseFilter = mapListenerFilter(in.ResponseFilter)
	}
	if in.EventFilter != nil {
		out.EventFilter = mapListenerFilter(in.EventFilter)
	}

	return out
}

func mapListenerFilter(in *roverv1.ListenerFilter) api.ListenerFilter {
	out := api.ListenerFilter{}
	if len(in.Trigger) > 0 {
		out.Trigger = in.Trigger
	}
	if len(in.Payload) > 0 {
		out.Payload = in.Payload
	}
	return out
}

func mapListenerSubscription(in *roverv1.Rover, out *api.Rover) {
	if in.Spec.ListenerSubscription == nil {
		return
	}
	ls := in.Spec.ListenerSubscription
	out.ListenerSubscription = api.ListenerSubscription{}
	if ls.DeliveryType != "" {
		out.ListenerSubscription.DeliveryType = api.ListenerSubscriptionDeliveryType(ls.DeliveryType)
	}
	if ls.Callback != "" {
		out.ListenerSubscription.Callback = ls.Callback
	}
}
