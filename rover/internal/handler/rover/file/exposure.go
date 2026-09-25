// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package file

import (
	"context"
	"fmt"
	"strings"

	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/config"
	"github.com/telekom/controlplane/common/pkg/errors/ctrlerrors"
	"github.com/telekom/controlplane/common/pkg/types"
	"github.com/telekom/controlplane/common/pkg/util/contextutil"
	"github.com/telekom/controlplane/common/pkg/util/labelutil"
	filev1 "github.com/telekom/controlplane/file/api/v1"
	organizationv1 "github.com/telekom/controlplane/organization/api/v1"
	roverv1 "github.com/telekom/controlplane/rover/api/v1"
)

// HandleExposure creates or updates a file-domain FileExposure owned by the Rover.
func HandleExposure(ctx context.Context, c client.JanitorClient, owner *roverv1.Rover, exp *roverv1.FileExposure) error {
	logger := log.FromContext(ctx)
	logger.V(1).Info("Handle FileExposure", "fileType", exp.FileType)

	fileExposure := &filev1.FileExposure{
		ObjectMeta: metav1.ObjectMeta{
			Name:      MakeName(exp.FileType, owner.Name),
			Namespace: owner.Namespace,
		},
	}

	environment := contextutil.EnvFromContextOrDie(ctx)
	zoneRef := types.ObjectRef{
		Name:      owner.Spec.Zone,
		Namespace: environment,
	}

	// Resolve the owning team up front; it is added to the trusted teams below.
	// The owner team is required here, so block (and requeue) if it cannot be resolved.
	ownerTeam, err := organizationv1.FindTeamForObject(ctx, owner)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return ctrlerrors.BlockedErrorf("owner team not found for application %s", owner.Name)
		}
		return err
	}

	mutator := func() error {
		if err := controllerutil.SetControllerReference(owner, fileExposure, c.Scheme()); err != nil {
			return errors.Wrap(err, "failed to set controller reference")
		}

		fileExposure.Labels = map[string]string{
			config.BuildLabelKey("zone"):        labelutil.NormalizeLabelValue(zoneRef.Name),
			config.BuildLabelKey("application"): labelutil.NormalizeLabelValue(owner.Name),
		}

		fileExposure.Spec = filev1.FileExposureSpec{
			Approval:   filev1.Approval{Strategy: filev1.ApprovalStrategy(exp.Approval.Strategy)},
			Visibility: filev1.Visibility(exp.Visibility.String()),
			FileType:   exp.FileType,
			SFTP:       mapSFTP(exp.SFTP),
			Provider: types.TypedObjectRef{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Application",
					APIVersion: "application.cp.ei.telekom.de/v1",
				},
				ObjectRef: *owner.Status.Application,
			},
			Variant: strings.ToLower(string(exp.Variant)),
			Zone:    &zoneRef,
		}

		var err error
		fileExposure.Spec.Approval.TrustedTeams, err = mapTrustedTeams(ctx, exp.Approval.TrustedTeams)
		if err != nil {
			return errors.Wrap(err, "failed to map trusted teams")
		}

		// add owner to trusted teams (already resolved above)
		fileExposure.Spec.Approval.TrustedTeams = append(fileExposure.Spec.Approval.TrustedTeams, ownerTeam.GetName())
		return nil
	}

	if _, err := c.CreateOrUpdate(ctx, fileExposure, mutator); err != nil {
		return errors.Wrap(err, "failed to create or update FileExposure")
	}

	owner.Status.FileExposures = append(owner.Status.FileExposures, types.ObjectRef{
		Name:      fileExposure.Name,
		Namespace: fileExposure.Namespace,
	})
	return nil
}

func mapTrustedTeams(ctx context.Context, teams []roverv1.TrustedTeam) ([]string, error) {
	logger := log.FromContext(ctx)
	if len(teams) == 0 {
		return nil, nil
	}

	apiTrustedTeams := make([]string, 0, len(teams))
	for _, team := range teams {
		namespace := contextutil.EnvFromContextOrDie(ctx) + "--" + team.Group + "--" + team.Team
		t, err := organizationv1.FindTeamForNamespace(ctx, namespace)
		switch {
		case err != nil && apierrors.IsNotFound(err):
			logger.Info(fmt.Sprintf("Trusted team %s/%s not found", team.Group, team.Team))
		case err != nil:
			return nil, err
		default:
			apiTrustedTeams = append(apiTrustedTeams, t.GetName())
		}
	}

	return apiTrustedTeams, nil
}
