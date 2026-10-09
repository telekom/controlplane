// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package in

import (
	roverv1 "github.com/telekom/controlplane/rover/api/v1"

	"github.com/telekom/controlplane/rover-server/internal/api"
)

func mapListenersIn(in *api.Rover, out *roverv1.Rover) {
	if len(in.Listeners) == 0 {
		return
	}
	out.Spec.Listeners = make([]roverv1.RoverListener, len(in.Listeners))
	for i := range in.Listeners {
		out.Spec.Listeners[i] = mapListenerIn(&in.Listeners[i])
	}
}

func mapListenerIn(in *api.RoverListener) roverv1.RoverListener {
	return roverv1.RoverListener{
		Consumer:       in.Consumer,
		Provider:       in.Provider,
		ApiBasePath:    in.ApiBasePath,
		RequestFilter:  mapListenerFilterIn(in.RequestFilter),
		ResponseFilter: mapListenerFilterIn(in.ResponseFilter),
	}
}

func mapListenerFilterIn(in api.ListenerFilter) *roverv1.ListenerFilter {
	if in.Trigger == nil && in.Payload == nil {
		return nil
	}
	return &roverv1.ListenerFilter{
		Trigger: in.Trigger,
		Payload: in.Payload,
	}
}

func mapListenerSubscriptionIn(in *api.Rover, out *roverv1.Rover) {
	if in.ListenerSubscription.DeliveryType == "" && in.ListenerSubscription.Callback == "" {
		return
	}
	ls := &roverv1.ListenerSubscription{}
	if in.ListenerSubscription.DeliveryType != "" {
		ls.DeliveryType = string(in.ListenerSubscription.DeliveryType)
	}
	if in.ListenerSubscription.Callback != "" {
		ls.Callback = in.ListenerSubscription.Callback
	}
	out.Spec.ListenerSubscription = ls
}
