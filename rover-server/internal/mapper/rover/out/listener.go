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
	return api.RoverListener{
		Consumer:    in.Consumer,
		Provider:    in.Provider,
		ApiBasePath: in.ApiBasePath,
	}
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
