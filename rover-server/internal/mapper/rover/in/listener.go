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
	out := roverv1.RoverListener{
		Consumer: in.Consumer,
		Provider: in.Provider,
	}

	if in.ApiBasePath != "" {
		out.ApiBasePath = in.ApiBasePath
	}
	if in.EventType != "" {
		out.EventType = in.EventType
	}
	if hasListenerFilterContent(&in.RequestFilter) {
		out.RequestFilter = mapListenerFilterIn(&in.RequestFilter)
	}
	if hasListenerFilterContent(&in.ResponseFilter) {
		out.ResponseFilter = mapListenerFilterIn(&in.ResponseFilter)
	}
	if hasListenerFilterContent(&in.EventFilter) {
		out.EventFilter = mapListenerFilterIn(&in.EventFilter)
	}

	return out
}

func hasListenerFilterContent(f *api.ListenerFilter) bool {
	return len(f.Trigger) > 0 || len(f.Payload) > 0
}

func mapListenerFilterIn(in *api.ListenerFilter) *roverv1.ListenerFilter {
	if len(in.Trigger) == 0 && len(in.Payload) == 0 {
		return nil
	}
	out := &roverv1.ListenerFilter{}
	if len(in.Trigger) > 0 {
		out.Trigger = in.Trigger
	}
	if len(in.Payload) > 0 {
		out.Payload = in.Payload
	}
	return out
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
