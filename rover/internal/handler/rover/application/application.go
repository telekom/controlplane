// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package application

import (
	"context"
	"strings"

	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/uuid"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/config"
	"github.com/telekom/controlplane/common/pkg/types"
	"github.com/telekom/controlplane/common/pkg/util/contextutil"
	"github.com/telekom/controlplane/common/pkg/util/labelutil"

	applicationv1 "github.com/telekom/controlplane/application/api/v1"
	organizationv1 "github.com/telekom/controlplane/organization/api/v1"
	roverv1 "github.com/telekom/controlplane/rover/api/v1"
)

func HandleApplication(ctx context.Context, c client.JanitorClient, owner *roverv1.Rover) error {

	environment := contextutil.EnvFromContextOrDie(ctx)
	zoneRef := types.ObjectRef{
		Name:      owner.Spec.Zone,
		Namespace: environment,
	}

	application := &applicationv1.Application{
		ObjectMeta: metav1.ObjectMeta{
			Name:      owner.Name,
			Namespace: owner.Namespace,
		},
	}

	team, err := findTeam(ctx, c, owner)
	if err != nil {
		return err
	}

	needsClient := len(owner.Spec.Subscriptions) > 0

	mutator := func() error {
		err := controllerutil.SetControllerReference(owner, application, c.Scheme())
		if err != nil {
			return errors.Wrap(err, "failed to set controller reference")
		}

		// TODO: This should be solved using a default-webhook
		secret := application.Spec.Secret
		if secret == "" {
			secret = string(uuid.NewUUID())
		}

		application.Spec = applicationv1.ApplicationSpec{
			Team:          team.Name,
			TeamEmail:     team.Spec.Email,
			Zone:          zoneRef,
			NeedsClient:   needsClient,
			NeedsConsumer: needsClient,
			Secret:        secret,
		}

		application.Labels = map[string]string{
			config.BuildLabelKey("zone"):        labelutil.NormalizeValue(zoneRef.Name),
			config.BuildLabelKey("application"): labelutil.NormalizeValue(owner.Name),
		}

		return nil
	}

	_, err = c.CreateOrUpdate(ctx, application, mutator)
	if err != nil {
		return errors.Wrap(err, "failed to create or update application")
	}

	owner.Status.Application = &types.ObjectRef{
		Name:      application.Name,
		Namespace: application.Namespace,
	}

	return err
}

// findTeam finds the team for the given owner identified by the resource namespace.
func findTeam(ctx context.Context, c client.JanitorClient, owner *roverv1.Rover) (*organizationv1.Team, error) {

	// find owners team with help of resource namespace <environment>--<group>--<team>
	roverNamespaceParts := strings.Split(owner.Namespace, "--")

	if len(roverNamespaceParts) != 3 {
		return nil, errors.New("invalid rover resource namespace")
	}

	team := &organizationv1.Team{}
	teamRef := types.ObjectRef{
		Name:      roverNamespaceParts[1] + "--" + roverNamespaceParts[2],
		Namespace: roverNamespaceParts[0],
	}

	err := c.Get(ctx, teamRef.K8s(), team)

	if err != nil {
		return nil, errors.Wrapf(err, "failed to get team %s", teamRef.Name)
	}

	return team, nil
}
