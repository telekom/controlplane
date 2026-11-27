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
		// The kind may have been switched away from ApiSpecification: drop a previously created mirror.
		if roadmap.Status.ApiRoadmap.Name != "" {
			if err := h.deleteMirror(ctx, roadmap); err != nil {
				return err
			}
			roadmap.Status.ApiRoadmap = types.ObjectRef{}
		}
		roadmap.SetCondition(condition.NewDoneProcessingCondition("Roadmap processed"))
		roadmap.SetCondition(condition.NewReadyCondition(condition.ReasonProvisioned, "Roadmap is ready"))
		return nil
	}

	c := client.ClientFromContextOrDie(ctx)

	apiSpec := &roverv1.ApiSpecification{}
	// The specification must live in the roadmap's own namespace (= its environment).
	err := c.Get(ctx, crclient.ObjectKey{Name: ref.Name, Namespace: roadmap.Namespace}, apiSpec)
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
		// audit:ignore 06-rover-child-labels/missing-ownership-labels shared per-base-path mirror, owner is not a Rover/Application
		if err := controllerutil.SetControllerReference(roadmap, mirror, c.Scheme()); err != nil {
			return errors.Wrap(err, "failed to set controller reference")
		}

		// audit:ignore 06-rover-child-labels/missing-ownership-labels mirror is owned by an ApiRoadmap (per base path), not a Rover/Application; identity label only, like the shared Api
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

	if c.AnyChanged() || !c.AllReady() {
		roadmap.SetCondition(condition.NewProcessingCondition(condition.ReasonProvisioning, "Roadmap updated"))
		roadmap.SetCondition(condition.NewNotReadyCondition(condition.ReasonProvisioning, "Roadmap is not ready"))
		return nil
	}

	roadmap.SetCondition(condition.NewDoneProcessingCondition("Roadmap processed"))
	roadmap.SetCondition(condition.NewReadyCondition(condition.ReasonProvisioned, "Roadmap is ready"))
	return nil
}

// Delete removes the mirror explicitly: the api controller adds the Api as a second owner,
// so garbage collection alone would keep the mirror alive after the rover object is gone.
// A mirror recorded in the status is deleted even if the roadmap was switched away from an
// ApiSpecification and the cleanup in CreateOrUpdate has not completed yet.
// Files in file-manager are cleaned up by the REST API layer, not by the operator.
func (h *RoadmapHandler) Delete(ctx context.Context, roadmap *roverv1.Roadmap) error {
	if roadmap.Spec.SpecificationRef.Kind != apiSpecificationKind && roadmap.Status.ApiRoadmap.Name == "" {
		return nil
	}
	return h.deleteMirror(ctx, roadmap)
}

func (h *RoadmapHandler) deleteMirror(ctx context.Context, roadmap *roverv1.Roadmap) error {
	c := client.ClientFromContextOrDie(ctx)
	mirror := &apiapi.ApiRoadmap{
		ObjectMeta: metav1.ObjectMeta{Name: roadmap.Name, Namespace: roadmap.Namespace},
	}
	if err := c.Delete(ctx, mirror); err != nil && !apierrors.IsNotFound(err) {
		return errors.Wrap(err, "failed to delete api ApiRoadmap")
	}
	return nil
}
