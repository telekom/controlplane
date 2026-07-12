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

func (h *McpSpecificationHandler) CreateOrUpdate(ctx context.Context, mcpSpec *roverv1.McpSpecification) error {
	c := client.ClientFromContextOrDie(ctx)
	name := roverv1.MakeMcpSpecificationName(mcpSpec.Spec.BasePath)

	mcpServer := &agenticv1.McpServer{
		ObjectMeta: metav1.ObjectMeta{
			Name:      labelutil.NormalizeNameValue(name),
			Namespace: mcpSpec.Namespace,
		},
	}

	mcpSpec.Status.McpServer = *types.ObjectRefFromObject(mcpServer)

	mutator := func() error {
		err := controllerutil.SetControllerReference(mcpSpec, mcpServer, c.Scheme())
		if err != nil {
			return errors.Wrap(err, "failed to set controller reference")
		}

		mcpServer.Labels = map[string]string{
			agenticv1.McpBasePathLabelKey: labelutil.NormalizeLabelValue(mcpSpec.Spec.BasePath),
		}

		mcpServer.Spec = agenticv1.McpServerSpec{
			BasePath:      mcpSpec.Spec.BasePath,
			Version:       mcpSpec.Spec.Version,
			Name:          mcpSpec.Spec.Name,
			Description:   mcpSpec.Spec.Description,
			Specification: mcpSpec.Spec.Specification,
			Oauth2Scopes:  mcpSpec.Spec.Oauth2Scopes,
		}

		return nil
	}

	_, err := c.CreateOrUpdate(ctx, mcpServer, mutator)
	if err != nil {
		return errors.Wrap(err, "failed to create or update McpServer")
	}

	if c.AnyChanged() {
		mcpSpec.SetCondition(condition.NewProcessingCondition("Provisioning", "McpServer updated"))
		mcpSpec.SetCondition(condition.NewNotReadyCondition("Provisioning", "McpServer is not ready"))
	} else {
		mcpSpec.SetCondition(condition.NewDoneProcessingCondition("McpServer created"))
		mcpSpec.SetCondition(condition.NewReadyCondition("Provisioned", "McpServer is ready"))
	}

	return nil
}

func (h *McpSpecificationHandler) Delete(_ context.Context, _ *roverv1.McpSpecification) error {
	return nil
}
