// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package in

import (
	"github.com/pkg/errors"
	"github.com/telekom/controlplane/common/pkg/config"
	roverv1 "github.com/telekom/controlplane/rover/api/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/telekom/controlplane/rover-server/internal/api"
	"github.com/telekom/controlplane/rover-server/internal/mapper"
)

func MapRequest(in *api.RoverUpdateRequest, id mapper.ResourceIdInfo) (res *roverv1.Rover, err error) {
	res = &roverv1.Rover{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Rover",
			APIVersion: "rover.cp.ei.telekom.de/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      id.Name,
			Namespace: id.Environment + "--" + id.Namespace,
			Labels: map[string]string{
				config.EnvironmentLabelKey: id.Environment,
			},
		},
		Spec: roverv1.RoverSpec{},
	}
	if err = MapRover(in, res); err != nil {
		return res, errors.Wrap(err, "Get All Rovers")
	}

	return
}

func MapRover(in *api.Rover, out *roverv1.Rover) error {
	if err := mapExposures(in, out); err != nil {
		return err
	}

	if err := mapSubscriptions(in, out); err != nil {
		return err
	}

	out.Spec.Zone = in.Zone
	if len(in.IpRestrictions.Allow) > 0 {
		out.Spec.IpRestrictions = &roverv1.IpRestrictions{
			Allow: in.IpRestrictions.Allow,
		}
	}
	return nil
}

func mapExposures(in *api.Rover, out *roverv1.Rover) error {
	if in == nil {
		return errors.New("input rover is nil")
	}
	out.Spec.Exposures = make([]roverv1.Exposure, len(in.Exposures))
	for i := range out.Spec.Exposures {
		err := mapExposure(&in.Exposures[i], &out.Spec.Exposures[i])
		if err != nil {
			return errors.Wrap(err, "failed to map exposure")
		}
	}
	return nil
}

func mapSubscriptions(in *api.Rover, out *roverv1.Rover) error {
	out.Spec.Subscriptions = make([]roverv1.Subscription, len(in.Subscriptions))
	for i := range out.Spec.Subscriptions {
		err := mapSubscription(&in.Subscriptions[i], &out.Spec.Subscriptions[i])
		if err != nil {
			return errors.Wrap(err, "failed to map subscription")
		}
	}
	return nil
}
