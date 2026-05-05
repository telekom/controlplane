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
	rovermapper "github.com/telekom/controlplane/rover-server/internal/mapper/rover"
	"github.com/telekom/controlplane/rover-server/internal/mapper/status"
	"github.com/telekom/controlplane/rover-server/pkg/store"
)

func MapResponse(ctx context.Context, in *roverv1.Rover, stores *store.Stores) (res api.RoverResponse, err error) {
	tmp := api.Rover{}
	if err = MapRover(in, &tmp); err != nil {
		return res, err
	}

	if err = mapper.CopyFromTo(tmp, &res); err != nil {
		return res, err
	}

	res.Name = in.Name
	res.Id = mapper.MakeResourceId(in)
	res.Status, err = status.MapRoverStatus(ctx, in, stores)

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
	mapAuthentication(in, out)
	return nil
}

// tokenRequestToAPI maps rover CRD tokenRequest values to rover-server API enum values.
var tokenRequestToAPI = map[string]api.AuthenticationClientAuthMethod{
	rovermapper.TokenRequestClientSecretBasic: api.BASIC,
	rovermapper.TokenRequestClientSecretPost:  api.POST,
}

func mapAuthentication(in *roverv1.Rover, out *api.Rover) {
	if in.Spec.Authentication == nil || in.Spec.Authentication.M2M == nil {
		return
	}
	tokenRequest := in.Spec.Authentication.M2M.TokenRequest
	if tokenRequest == "" {
		return
	}
	method, ok := tokenRequestToAPI[tokenRequest]
	if !ok {
		return
	}
	out.Authentication = api.Authentication{
		ClientAuthMethod: method,
	}
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
