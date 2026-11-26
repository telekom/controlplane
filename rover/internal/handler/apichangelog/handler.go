// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package apichangelog

import (
	"context"

	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	apiapi "github.com/telekom/controlplane/api/api/v1"
	"github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/errors/ctrlerrors"
	"github.com/telekom/controlplane/common/pkg/handler"
	"github.com/telekom/controlplane/common/pkg/types"
	"github.com/telekom/controlplane/common/pkg/util/labelutil"
	roverv1 "github.com/telekom/controlplane/rover/api/v1"
)

var _ handler.Handler[*roverv1.ApiChangelog] = (*ApiChangelogHandler)(nil)

// ApiChangelogHandler mirrors a rover ApiChangelog into an api ApiChangelog.
// The mirror is owned by the rover object, so deleting the rover object removes it.
type ApiChangelogHandler struct{}

func (h *ApiChangelogHandler) CreateOrUpdate(ctx context.Context, changelog *roverv1.ApiChangelog) error {
	c := client.ClientFromContextOrDie(ctx)

	apiSpec := &roverv1.ApiSpecification{}
	ref := changelog.Spec.SpecificationRef
	// The specification must live in the changelog's own namespace (= its environment).
	err := c.Get(ctx, crclient.ObjectKey{Name: ref.Name, Namespace: changelog.Namespace}, apiSpec)
	if apierrors.IsNotFound(err) {
		return ctrlerrors.BlockedErrorf("ApiSpecification %q not found, apply it first", ref.Name)
	}
	if err != nil {
		return errors.Wrap(err, "failed to get ApiSpecification")
	}

	mirror := &apiapi.ApiChangelog{
		ObjectMeta: metav1.ObjectMeta{
			Name:      changelog.Name,
			Namespace: changelog.Namespace,
		},
	}

	mutator := func() error {
		if err := controllerutil.SetControllerReference(changelog, mirror, c.Scheme()); err != nil {
			return errors.Wrap(err, "failed to set controller reference")
		}

		// audit:ignore 06-rover-child-labels/missing-ownership-labels mirror is owned by an ApiChangelog (per base path), not a Rover/Application; identity label only, like the shared Api
		mirror.Labels = map[string]string{
			apiapi.BasePathLabelKey: labelutil.NormalizeLabelValue(apiSpec.Spec.BasePath),
		}
		mirror.Spec = apiapi.ApiChangelogSpec{
			BasePath: apiSpec.Spec.BasePath,
			Contents: changelog.Spec.Contents,
			Hash:     changelog.Spec.Hash,
		}
		return nil
	}

	if _, err := c.CreateOrUpdate(ctx, mirror, mutator); err != nil {
		return errors.Wrap(err, "failed to create or update api ApiChangelog")
	}

	changelog.Status.ApiChangelog = *types.ObjectRefFromObject(mirror)

	if c.AnyChanged() {
		changelog.SetCondition(condition.NewProcessingCondition(condition.ReasonProvisioning, "ApiChangelog updated"))
		changelog.SetCondition(condition.NewNotReadyCondition(condition.ReasonProvisioning, "ApiChangelog is not ready"))
		return nil
	}

	changelog.SetCondition(condition.NewDoneProcessingCondition("ApiChangelog created"))
	changelog.SetCondition(condition.NewReadyCondition(condition.ReasonProvisioned, "ApiChangelog is ready"))
	return nil
}

func (h *ApiChangelogHandler) Delete(ctx context.Context, changelog *roverv1.ApiChangelog) error {
	c := client.ClientFromContextOrDie(ctx)
	mirror := &apiapi.ApiChangelog{
		ObjectMeta: metav1.ObjectMeta{Name: changelog.Name, Namespace: changelog.Namespace},
	}
	if err := c.Delete(ctx, mirror); err != nil && !apierrors.IsNotFound(err) {
		return errors.Wrap(err, "failed to delete api ApiChangelog")
	}
	return nil
}
