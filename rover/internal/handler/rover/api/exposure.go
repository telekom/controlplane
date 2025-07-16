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

	rover "github.com/telekom/controlplane/rover/api/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
			Approval:    apiapi.ApprovalStrategy(exp.Approval.Strategy),
			Zone:        zoneRef,
			Upstreams:   make([]apiapi.Upstream, len(exp.Upstreams)),
		}

		if exp.Transformation != nil {
			apiExposure.Spec.Transformation = &apiapi.Transformation{
				Request: apiapi.RequestResponseTransformation{
					Headers: apiapi.HeaderTransformation{
						Remove: exp.Transformation.Request.Headers.Remove,
					},
				},
			}
		}

		if exp.Security != nil {
			if exp.Security.M2M != nil {
				apiExposure.Spec.Security = &apiapi.Security{
					M2M: &apiapi.Machine2MachineAuthentication{
						Scopes: exp.Security.M2M.Scopes,
					},
				}
				if exp.Security.M2M.ExternalIDP != nil {
					apiExposure.Spec.Security.M2M.ExternalIDP = &apiapi.ExternalIdentityProvider{
						TokenEndpoint: exp.Security.M2M.ExternalIDP.TokenEndpoint,
						TokenRequest:  exp.Security.M2M.ExternalIDP.TokenRequest,
						GrantType:     exp.Security.M2M.ExternalIDP.GrantType,
						Basic:         toApiBasic(exp.Security.M2M.ExternalIDP.Basic),
						Client:        toApiClient(exp.Security.M2M.ExternalIDP.Client),
					}
				}
			}
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
