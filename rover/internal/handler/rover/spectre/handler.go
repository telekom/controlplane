// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package spectre

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/errors/ctrlerrors"
	"github.com/telekom/controlplane/common/pkg/types"
	"github.com/telekom/controlplane/common/pkg/util/labelutil"
	roverv1 "github.com/telekom/controlplane/rover/api/v1"
	spectrev1 "github.com/telekom/controlplane/spectre/api/v1"
)

// HandleListeners creates or updates SpectreApplication and Listener CRs
// for each entry in the Rover's spec.listeners list.
func HandleListeners(ctx context.Context, c client.JanitorClient, rover *roverv1.Rover) error {
	rover.Status.SpectreApplications = make([]types.ObjectRef, 0)
	rover.Status.SpectreListeners = make([]types.ObjectRef, 0)

	if len(rover.Spec.Listeners) == 0 {
		return nil
	}

	if rover.Status.Application == nil {
		return ctrlerrors.BlockedErrorf("rover status.application is not yet set")
	}

	app, err := ensureSpectreApplication(ctx, c, rover)
	if err != nil {
		return err
	}
	rover.Status.SpectreApplications = []types.ObjectRef{
		{Name: app.Name, Namespace: app.Namespace},
	}

	rover.Status.SpectreListeners = make([]types.ObjectRef, 0, len(rover.Spec.Listeners))
	for _, rl := range rover.Spec.Listeners {
		listener, err := ensureListener(ctx, c, rover, app, rl)
		if err != nil {
			return err
		}
		rover.Status.SpectreListeners = append(rover.Status.SpectreListeners, types.ObjectRef{
			Name:      listener.Name,
			Namespace: listener.Namespace,
		})
	}

	return nil
}

// ensureSpectreApplication creates or updates a single SpectreApplication CR owned by the Rover.
func ensureSpectreApplication(ctx context.Context, c client.JanitorClient, rover *roverv1.Rover) (*spectrev1.SpectreApplication, error) {
	logger := log.FromContext(ctx)
	logger.V(1).Info("Ensuring SpectreApplication", "rover", rover.Name)

	app := &spectrev1.SpectreApplication{
		ObjectMeta: metav1.ObjectMeta{
			Name:      makeSpectreAppName(rover.Name),
			Namespace: rover.Namespace,
		},
	}

	mutator := func() error {
		if err := controllerutil.SetControllerReference(rover, app, c.Scheme()); err != nil {
			return errors.Wrap(err, "failed to set controller reference on SpectreApplication")
		}

		deliveryType := "server_sent_event"
		var callback string
		if rover.Spec.ListenerSubscription != nil {
			if rover.Spec.ListenerSubscription.DeliveryType != "" {
				deliveryType = rover.Spec.ListenerSubscription.DeliveryType
			}
			if rover.Spec.ListenerSubscription.DeliveryType == "callback" {
				callback = rover.Spec.ListenerSubscription.Callback
			}
		}

		app.Spec = spectrev1.SpectreApplicationSpec{
			Application: types.TypedObjectRef{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Application",
					APIVersion: "application.cp.ei.telekom.de/v1",
				},
				ObjectRef: *rover.Status.Application,
			},
			DeliveryType: deliveryType,
			Callback:     callback,
		}

		return nil
	}

	_, err := c.CreateOrUpdate(ctx, app, mutator)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create or update SpectreApplication")
	}

	return app, nil
}

// ensureListener creates or updates a single Listener CR owned by the Rover.
func ensureListener(ctx context.Context, c client.JanitorClient, rover *roverv1.Rover, app *spectrev1.SpectreApplication, rl roverv1.RoverListener) (*spectrev1.Listener, error) {
	logger := log.FromContext(ctx)
	logger.V(1).Info("Ensuring Listener", "rover", rover.Name, "listener", makeListenerName(rover.Name, rl))

	listener := &spectrev1.Listener{
		ObjectMeta: metav1.ObjectMeta{
			Name:      makeListenerName(rover.Name, rl),
			Namespace: rover.Namespace,
		},
	}

	mutator := func() error {
		if err := controllerutil.SetControllerReference(rover, listener, c.Scheme()); err != nil {
			return errors.Wrap(err, "failed to set controller reference on Listener")
		}

		listener.Spec = spectrev1.ListenerSpec{
			Consumer: types.TypedObjectRef{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Application",
					APIVersion: "application.cp.ei.telekom.de/v1",
				},
				ObjectRef: types.ObjectRef{
					Name:      rl.Consumer,
					Namespace: rover.Namespace,
				},
			},
			Provider: types.TypedObjectRef{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Application",
					APIVersion: "application.cp.ei.telekom.de/v1",
				},
				ObjectRef: types.ObjectRef{
					Name:      rl.Provider,
					Namespace: rover.Namespace,
				},
			},
			Application: types.ObjectRef{
				Name:      app.Name,
				Namespace: app.Namespace,
			},
		}

		// Set ApiListener when ApiBasePath is provided
		if rl.ApiBasePath != "" {
			listener.Spec.ApiListener = &spectrev1.ApiListener{
				ApiBasePath:    rl.ApiBasePath,
				RequestFilter:  mapListenerFilter(rl.RequestFilter),
				ResponseFilter: mapListenerFilter(rl.ResponseFilter),
			}
		}

		// Set EventListener when EventType is provided
		if rl.EventType != "" {
			listener.Spec.EventListener = &spectrev1.EventListener{
				EventType: rl.EventType,
				Filter:    mapListenerFilter(rl.EventFilter),
			}
		}

		return nil
	}

	_, err := c.CreateOrUpdate(ctx, listener, mutator)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create or update Listener")
	}

	return listener, nil
}

// mapListenerFilter converts a rover ListenerFilter to a spectre ListenerFilter.
func mapListenerFilter(f *roverv1.ListenerFilter) *spectrev1.ListenerFilter {
	if f == nil {
		return nil
	}
	return &spectrev1.ListenerFilter{
		Trigger: f.Trigger,
		Payload: f.Payload,
	}
}

// makeSpectreAppName generates a deterministic name for the SpectreApplication.
func makeSpectreAppName(roverName string) string {
	return fmt.Sprintf("%s--spectre-app", roverName)
}

// makeListenerName generates a deterministic name for a Listener based on content identity.
func makeListenerName(roverName string, rl roverv1.RoverListener) string {
	var key string
	if rl.ApiBasePath != "" {
		key = rl.Consumer + "--" + rl.ApiBasePath
	} else {
		key = rl.Consumer + "--" + rl.EventType
	}
	return labelutil.NormalizeNameValue(roverName + "--" + key)
}
