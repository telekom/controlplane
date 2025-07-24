// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"context"

	"github.com/pkg/errors"
	apiapi "github.com/telekom/controlplane/api/api/v1"
	"github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/config"
	"github.com/telekom/controlplane/common/pkg/types"
	"github.com/telekom/controlplane/common/pkg/util/contextutil"
	"github.com/telekom/controlplane/common/pkg/util/labelutil"
	organizationv1 "github.com/telekom/controlplane/organization/api/v1"

	rover "github.com/telekom/controlplane/rover/api/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

func HandleExposure(ctx context.Context, c client.JanitorClient, owner *rover.Rover, exp *rover.ApiExposure) error {

	log := log.FromContext(ctx)
	log.V(1).Info("Handle APIExposure", "basePath", exp.BasePath)

	name := MakeName(owner.Name, exp.BasePath, "")

	apiExposure := &apiapi.ApiExposure{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: owner.Namespace,
		},
	}

	environment := contextutil.EnvFromContextOrDie(ctx)
	zoneRef := types.ObjectRef{
		Name:      owner.Spec.Zone,
		Namespace: environment,
	}

	mutator := func() error {
		err := controllerutil.SetControllerReference(owner, apiExposure, c.Scheme())
		if err != nil {
			return errors.Wrap(err, "failed to set controller reference")
		}
		apiExposure.Labels = map[string]string{
			apiapi.BasePathLabelKey:             labelutil.NormalizeValue(exp.BasePath),
			config.BuildLabelKey("zone"):        labelutil.NormalizeValue(zoneRef.Name),
			config.BuildLabelKey("application"): labelutil.NormalizeValue(owner.Name),
		}

		apiExposure.Spec = apiapi.ApiExposureSpec{
			ApiBasePath: exp.BasePath,
			Visibility:  apiapi.Visibility(exp.Visibility.String()),
			Approval: apiapi.Approval{
				Strategy: apiapi.ApprovalStrategy(exp.Approval.Strategy),
			},
			Zone:           zoneRef,
			Upstreams:      make([]apiapi.Upstream, len(exp.Upstreams)),
			Security:       mapSecurityToApiSecurity(exp.Security),
			Transformation: mapTransformationtoApiTransformation(exp.Transformation),
		}

		apiExposure.Spec.Approval.TrustedTeams, err = mapTrustedTeamsToApiTrustedTeams(ctx, c, exp.Approval.TrustedTeams)
		if err != nil {
			return errors.Wrap(err, "failed to map trusted teams")
		}

		failoverZones, hasFailover := getFailoverZones(environment, exp.Traffic.Failover)
		if hasFailover {
			apiExposure.Spec.Traffic = apiapi.Traffic{
				Failover: &apiapi.Failover{
					Zones: failoverZones,
				},
			}
		}

		for i, upstream := range exp.Upstreams {
			apiExposure.Spec.Upstreams[i] = apiapi.Upstream{
				Url:    upstream.URL,
				Weight: upstream.Weight,
			}
		}

		return nil
	}

	_, err := c.CreateOrUpdate(ctx, apiExposure, mutator)
	if err != nil {
		return errors.Wrap(err, "failed to create or update ApiExposure")
	}

	owner.Status.ApiExposures = append(owner.Status.ApiExposures, types.ObjectRef{
		Name:      apiExposure.Name,
		Namespace: apiExposure.Namespace,
	})
	return err
}

func mapTrustedTeamsToApiTrustedTeams(ctx context.Context, c client.JanitorClient, teams []rover.TrustedTeam) ([]types.ObjectRef, error) {
	if len(teams) == 0 {
		return nil, nil
	}

	apiTrustedTeams := make([]types.ObjectRef, 0, len(teams))
	for _, team := range teams {
		foundTeam := &organizationv1.Team{}
		err := c.Get(ctx, k8sclient.ObjectKey{Namespace: contextutil.EnvFromContextOrDie(ctx), Name: team.Group + "--" + team.Team}, foundTeam)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to get trusted team '%s' in namespace '%s'", team.Group+"--"+team.Team, contextutil.EnvFromContextOrDie(ctx))
		}

		apiTrustedTeams = append(apiTrustedTeams, *types.ObjectRefFromObject(foundTeam))
	}

	return apiTrustedTeams, nil
}

func mapSecurityToApiSecurity(roverSecurity *rover.Security) *apiapi.Security {
	if roverSecurity == nil {
		return nil
	}

	security := &apiapi.Security{}

	if roverSecurity.M2M != nil {
		security.M2M = &apiapi.Machine2MachineAuthentication{
			Scopes: roverSecurity.M2M.Scopes,
		}

		if roverSecurity.M2M.ExternalIDP != nil {
			security.M2M.ExternalIDP = &apiapi.ExternalIdentityProvider{
				TokenEndpoint: roverSecurity.M2M.ExternalIDP.TokenEndpoint,
				TokenRequest:  roverSecurity.M2M.ExternalIDP.TokenRequest,
				GrantType:     roverSecurity.M2M.ExternalIDP.GrantType,
				Client:        toApiClient(roverSecurity.M2M.ExternalIDP.Client),
				Basic:         toApiBasic(roverSecurity.M2M.ExternalIDP.Basic),
			}
		}

		if roverSecurity.M2M.Basic != nil {
			security.M2M.Basic = &apiapi.BasicAuthCredentials{
				Username: roverSecurity.M2M.Basic.Username,
				Password: roverSecurity.M2M.Basic.Password,
			}
		}
	}

	return security

}

func mapTransformationtoApiTransformation(roverTransformation *rover.Transformation) *apiapi.Transformation {
	if roverTransformation == nil {
		return nil
	}

	apiTransformation := &apiapi.Transformation{}

	if len(roverTransformation.Request.Headers.Remove) > 0 {
		apiTransformation.Request.Headers.Remove = roverTransformation.Request.Headers.Remove
	}

	return apiTransformation
}
