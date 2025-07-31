// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package out

import (
	"context"

	"github.com/pkg/errors"
	roverv1 "github.com/telekom/controlplane/rover/api/v1"

	"github.com/telekom/controlplane/rover-server/internal/api"
	"github.com/telekom/controlplane/rover-server/internal/mapper"
	"github.com/telekom/controlplane/rover-server/internal/mapper/status"
)

func MapRoverResponse(ctx context.Context, in *roverv1.Rover) (res api.RoverResponse, err error) {
	tmp := api.Rover{}
	if err = MapRover(in, &tmp); err != nil {
		return res, err
	}

	if err = mapper.CopyFromTo(tmp, &res); err != nil {
		return res, err
	}

	res.Name = in.Name
	res.Id = mapper.MakeResourceId(in)
	res.Status = status.MapRoverStatus(ctx, in)

	return
}

func MapRover(in *roverv1.Rover, out *api.Rover) error {
	if err := mapExposures(in, out); err != nil {
		return err
	}

	if err := mapSubscriptions(in, out); err != nil {
		return err
	}

	out.Zone = in.Spec.Zone
	return nil
}

func mapExposures(in *roverv1.Rover, out *api.Rover) error {
	if in == nil {
		return errors.New("input rover is nil")
	}
	if len(in.Spec.Exposures) == 0 {
		return nil
	}
	l := make([]api.Exposure, len(in.Spec.Exposures))
	out.Exposures = l
	for i := range l {
		err := mapExposure(&in.Spec.Exposures[i], &l[i])
		if err != nil {
			return errors.Wrap(err, "failed to map exposure")
		}
	}
	return nil
}

func mapSubscriptions(in *roverv1.Rover, out *api.Rover) error {
	l := make([]api.Subscription, len(in.Spec.Subscriptions))
	if len(in.Spec.Subscriptions) == 0 {
		return nil
	}
	out.Subscriptions = l
	for i := range l {
		err := mapSubscription(&in.Spec.Subscriptions[i], &l[i])
		if err != nil {
			return errors.Wrap(err, "failed to map subscription")
		}
	}
	return nil
}
