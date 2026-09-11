// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package agentspecification

import (
	"context"

	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	agenticv1 "github.com/telekom/controlplane/agentic/api/v1"
	"github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/handler"
	"github.com/telekom/controlplane/common/pkg/types"
	"github.com/telekom/controlplane/common/pkg/util/labelutil"
	roverv1 "github.com/telekom/controlplane/rover/api/v1"
)

var _ handler.Handler[*roverv1.AgentSpecification] = (*AgentSpecificationHandler)(nil)

type AgentSpecificationHandler struct{}

func (h *AgentSpecificationHandler) CreateOrUpdate(ctx context.Context, spec *roverv1.AgentSpecification) error {
	c := client.ClientFromContextOrDie(ctx)
	name := roverv1.MakeAgentSpecificationName(spec.Spec.BasePath)

	agentCard := &agenticv1.AgentCard{
		ObjectMeta: metav1.ObjectMeta{
			Name:      labelutil.NormalizeNameValue(name),
			Namespace: spec.Namespace,
		},
	}

	spec.Status.AgentCard = *types.ObjectRefFromObject(agentCard)

	mutator := func() error {
		err := controllerutil.SetControllerReference(spec, agentCard, c.Scheme())
		if err != nil {
			return errors.Wrap(err, "failed to set controller reference")
		}

		agentCard.Labels = map[string]string{
			agenticv1.AgentBasePathLabelKey: labelutil.NormalizeLabelValue(spec.Spec.BasePath),
		}

		agentCard.Spec = agenticv1.AgentCardSpec{
			BasePath:      spec.Spec.BasePath,
			Version:       spec.Spec.Version,
			Name:          spec.Spec.Name,
			Description:   spec.Spec.Description,
			Specification: spec.Spec.Specification,
			Category:      spec.Spec.Category,
			Oauth2Scopes:  spec.Spec.Oauth2Scopes,
		}

		return nil
	}

	_, err := c.CreateOrUpdate(ctx, agentCard, mutator)
	if err != nil {
		return errors.Wrap(err, "failed to create or update AgentCard")
	}

	if c.AnyChanged() {
		spec.SetCondition(condition.NewProcessingCondition("Provisioning", "AgentCard updated"))
		spec.SetCondition(condition.NewNotReadyCondition("Provisioning", "AgentCard is not ready"))
	} else {
		spec.SetCondition(condition.NewDoneProcessingCondition("AgentCard created"))
		spec.SetCondition(condition.NewReadyCondition("Provisioned", "AgentCard is ready"))
	}

	return nil
}

func (h *AgentSpecificationHandler) Delete(_ context.Context, _ *roverv1.AgentSpecification) error {
	return nil
}
