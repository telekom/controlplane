// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package roadmap

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

const apiSpecificationKind = "ApiSpecification"

var _ handler.Handler[*roverv1.Roadmap] = (*RoadmapHandler)(nil)

// RoadmapHandler mirrors a rover Roadmap that describes an ApiSpecification into an api ApiRoadmap.
// Roadmaps for other specification kinds (e.g. EventSpecification) are not mirrored.
// The mirror is owned by the rover object, so deleting the rover object removes it.
type RoadmapHandler struct{}

func (h *RoadmapHandler) CreateOrUpdate(ctx context.Context, roadmap *roverv1.Roadmap) error {
	ref := roadmap.Spec.SpecificationRef
	if ref.Kind != apiSpecificationKind {
		roadmap.SetCondition(condition.NewDoneProcessingCondition("Roadmap processed"))
		roadmap.SetCondition(condition.NewReadyCondition(condition.ReasonProvisioned, "Roadmap is ready"))
		return nil
	}

	c := client.ClientFromContextOrDie(ctx)

	apiSpec := &roverv1.ApiSpecification{}
	err := c.Get(ctx, crclient.ObjectKey{Name: ref.Name, Namespace: ref.Namespace}, apiSpec)
	if apierrors.IsNotFound(err) {
		return ctrlerrors.BlockedErrorf("ApiSpecification %q not found, apply it first", ref.Name)
	}
	if err != nil {
		return errors.Wrap(err, "failed to get ApiSpecification")
	}

	mirror := &apiapi.ApiRoadmap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      roadmap.Name,
			Namespace: roadmap.Namespace,
		},
	}

	mutator := func() error {
		if err := controllerutil.SetControllerReference(roadmap, mirror, c.Scheme()); err != nil {
			return errors.Wrap(err, "failed to set controller reference")
		}

		mirror.Labels = map[string]string{
			apiapi.BasePathLabelKey: labelutil.NormalizeLabelValue(apiSpec.Spec.BasePath),
		}
		mirror.Spec = apiapi.ApiRoadmapSpec{
			BasePath: apiSpec.Spec.BasePath,
			Contents: roadmap.Spec.Contents,
			Hash:     roadmap.Spec.Hash,
		}
		return nil
	}

	if _, err := c.CreateOrUpdate(ctx, mirror, mutator); err != nil {
		return errors.Wrap(err, "failed to create or update api ApiRoadmap")
	}

	roadmap.Status.ApiRoadmap = *types.ObjectRefFromObject(mirror)

	if c.AnyChanged() {
		roadmap.SetCondition(condition.NewProcessingCondition(condition.ReasonProvisioning, "Roadmap updated"))
		roadmap.SetCondition(condition.NewNotReadyCondition(condition.ReasonProvisioning, "Roadmap is not ready"))
		return nil
	}

	roadmap.SetCondition(condition.NewDoneProcessingCondition("Roadmap processed"))
	roadmap.SetCondition(condition.NewReadyCondition(condition.ReasonProvisioned, "Roadmap is ready"))
	return nil
}

// Delete is a no-op: the mirror is garbage-collected via its owner reference.
// Files in file-manager are cleaned up by the REST API layer, not by the operator.
func (h *RoadmapHandler) Delete(_ context.Context, _ *roverv1.Roadmap) error {
	return nil
}
