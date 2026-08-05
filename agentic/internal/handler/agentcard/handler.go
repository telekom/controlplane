// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package agentcard

import (
	"cmp"
	"context"
	"fmt"
	"slices"

	"github.com/pkg/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	agenticv1 "github.com/telekom/controlplane/agentic/api/v1"
	cclient "github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/handler"
	ctypes "github.com/telekom/controlplane/common/pkg/types"
	"github.com/telekom/controlplane/common/pkg/util/labelutil"
)

var _ handler.Handler[*agenticv1.AgentCard] = &AgentCardHandler{}

type AgentCardHandler struct{}

func (h *AgentCardHandler) CreateOrUpdate(ctx context.Context, obj *agenticv1.AgentCard) error {
	logger := log.FromContext(ctx)
	c := cclient.ClientFromContextOrDie(ctx)

	// List all AgentCards with the same basePath
	cardList := &agenticv1.AgentCardList{}
	if err := c.List(ctx, cardList, client.MatchingLabels{
		agenticv1.AgentBasePathLabelKey: labelutil.NormalizeLabelValue(obj.Spec.BasePath),
	}); err != nil {
		return errors.Wrapf(err, "failed to list AgentCards for basePath %q", obj.Spec.BasePath)
	}

	// Filter to exact basePath match
	var candidates []agenticv1.AgentCard
	for i := range cardList.Items {
		if cardList.Items[i].Spec.BasePath == obj.Spec.BasePath {
			candidates = append(candidates, cardList.Items[i])
		}
	}

	// Determine active: oldest-wins semantics.
	slices.SortStableFunc(candidates, func(a, b agenticv1.AgentCard) int {
		c := a.CreationTimestamp.Compare(b.CreationTimestamp.Time)
		if c == 0 {
			return cmp.Compare(a.GetNamespace(), b.GetNamespace())
		}
		return c
	})

	if ctypes.Equals(&candidates[0], obj) {
		obj.Status.Active = true
		obj.SetCondition(condition.NewReadyCondition("AgentCardActive", "AgentCard is active"))
		obj.SetCondition(condition.NewDoneProcessingCondition("AgentCard is processed"))
		logger.Info("AgentCard is processed")
	} else {
		obj.Status.Active = false
		obj.SetCondition(condition.NewNotReadyCondition("AgentCardNotActive", "AgentCard is not active"))
		obj.SetCondition(condition.NewBlockedCondition(
			fmt.Sprintf("AgentCard is blocked, another AgentCard with the same BasePath %q is active. "+
				"It will be automatically processed, if the other AgentCard will be deleted.", obj.Spec.BasePath),
		))
		logger.Info("AgentCard is blocked, another AgentCard with the same BasePath is already active.")
	}

	return nil
}

func (h *AgentCardHandler) Delete(ctx context.Context, obj *agenticv1.AgentCard) error {
	// No owned resources to clean up.
	// Other AgentCards for the same basePath will be re-reconciled via
	// MapAgentCardToAgentCard watch, allowing standby to become active.
	return nil
}
