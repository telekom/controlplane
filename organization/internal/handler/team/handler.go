// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package team

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/handler"
	"github.com/telekom/controlplane/common/pkg/util/contextutil"
	organizationv1 "github.com/telekom/controlplane/organization/api/v1"
	"github.com/telekom/controlplane/organization/internal/handler/group"
	internalHandler "github.com/telekom/controlplane/organization/internal/handler/team/handler"
	"github.com/telekom/controlplane/organization/internal/handler/team/handler/gateway_consumer"
	"github.com/telekom/controlplane/organization/internal/handler/team/handler/identity_client"
	"github.com/telekom/controlplane/organization/internal/handler/team/handler/namespace"
	"github.com/telekom/controlplane/organization/internal/secret"
	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

var _ handler.Handler[*organizationv1.Team] = &TeamHandler{}

type TeamHandler struct {
}

type order int

const (
	creation order = iota
	deletion
)

func getInternalObjectHandlersInOrder(order order) []internalHandler.ObjectHandler {
	switch order {
	case creation:
		return []internalHandler.ObjectHandler{
			&namespace.NamespaceHandler{},
			&identity_client.IdentityClientHandler{},
			&gateway_consumer.GatewayConsumerHandler{},
		}
	case deletion:
		return []internalHandler.ObjectHandler{
			&identity_client.IdentityClientHandler{},
			&gateway_consumer.GatewayConsumerHandler{},
			&namespace.NamespaceHandler{},
		}
	default:
		return nil
	}
}

func (h *TeamHandler) CreateOrUpdate(ctx context.Context, teamObj *organizationv1.Team) error {
	logger := log.FromContext(ctx)
	internalObjHandler := getInternalObjectHandlersInOrder(creation)

	logger.V(1).Info(fmt.Sprintf("ℹ️ requesting group: %s", teamObj.Spec.Group))
	_, err := group.GetGroupByName(ctx, teamObj.Spec.Group)
	if err != nil {
		teamObj.SetCondition(condition.NewBlockedCondition("Group not found"))
		return nil
	}

	// CreateOrUpdate internal objects
	for i := range internalObjHandler {
		logger.V(1).Info("ℹ️ createOrUpdate sub-resource", "handler", internalObjHandler[i].Identifier())
		err = internalObjHandler[i].CreateOrUpdate(ctx, teamObj)
		if err != nil {
			teamObj.SetCondition(condition.NewBlockedCondition(fmt.Sprintf("Failed to handle %s", internalObjHandler[i].Identifier())))
			return errors.Wrap(err, fmt.Sprintf("failed to handle: %s", internalObjHandler[i].Identifier()))
		}
	}
	teamObj.SetCondition(condition.NewDoneProcessingCondition("Created Team"))
	teamObj.SetCondition(condition.NewReadyCondition("Ready", "Team is ready"))
	return nil
}

func (h *TeamHandler) Delete(ctx context.Context, teamObj *organizationv1.Team) error {
	logger := log.FromContext(ctx)
	logger.Info("TeamHandler Delete", "team", teamObj)
	internalObjHandler := getInternalObjectHandlersInOrder(deletion)

	for i := range internalObjHandler {
		logger.V(1).Info("ℹ️ delete sub-resource", "handler", internalObjHandler[i].Identifier())
		err := internalObjHandler[i].Delete(ctx, teamObj)
		if err != nil {
			if !k8sErrors.IsNotFound(err) {
				teamObj.SetCondition(condition.NewBlockedCondition(fmt.Sprintf("Failed to delete %s", internalObjHandler[i].Identifier())))
				return errors.Wrap(err, fmt.Sprintf("failed to delete: %s", internalObjHandler[i].Identifier()))
			}
		}
	}

	if err := secret.GetSecretManager().DeleteTeam(ctx, contextutil.EnvFromContextOrDie(ctx), teamObj.GetName()); err != nil {
		return errors.Wrap(err, "failed to delete team secrets from secret-manager")
	}

	return nil
}
