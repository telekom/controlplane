// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package mcpspecification

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

var _ handler.Handler[*roverv1.McpSpecification] = (*McpSpecificationHandler)(nil)

type McpSpecificationHandler struct{}

func (h *McpSpecificationHandler) CreateOrUpdate(ctx context.Context, spec *roverv1.McpSpecification) error {
	c := client.ClientFromContextOrDie(ctx)
	name := roverv1.MakeMcpSpecificationName(spec.Spec.BasePath)

	mcpServer := &agenticv1.McpServer{
		ObjectMeta: metav1.ObjectMeta{
			Name:      labelutil.NormalizeNameValue(name),
			Namespace: spec.Namespace,
		},
	}

	spec.Status.McpServer = *types.ObjectRefFromObject(mcpServer)

	mutator := func() error {
		err := controllerutil.SetControllerReference(spec, mcpServer, c.Scheme())
		if err != nil {
			return errors.Wrap(err, "failed to set controller reference")
		}

		mcpServer.Labels = map[string]string{
			agenticv1.AgenticBasePathLabelKey: labelutil.NormalizeLabelValue(spec.Spec.BasePath),
		}

		mcpServer.Spec = agenticv1.McpServerSpec{
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

	_, err := c.CreateOrUpdate(ctx, mcpServer, mutator)
	if err != nil {
		return errors.Wrap(err, "failed to create or update McpServer")
	}

	if c.AnyChanged() {
		spec.SetCondition(condition.NewProcessingCondition("Provisioning", "McpServer updated"))
		spec.SetCondition(condition.NewNotReadyCondition("Provisioning", "McpServer is not ready"))
	} else {
		spec.SetCondition(condition.NewDoneProcessingCondition("McpServer created"))
		spec.SetCondition(condition.NewReadyCondition("Provisioned", "McpServer is ready"))
	}

	return nil
}

func (h *McpSpecificationHandler) Delete(_ context.Context, _ *roverv1.McpSpecification) error {
	return nil
}
