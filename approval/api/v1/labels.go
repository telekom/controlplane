// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"github.com/telekom/controlplane/common/pkg/config"
	"github.com/telekom/controlplane/common/pkg/types"
	"github.com/telekom/controlplane/common/pkg/util/labelutil"
)

// Label keys for filtering ApprovalRequest and Approval resources.
var (
	TargetKindLabelKey       = config.BuildLabelKey("target.kind")
	TargetNameLabelKey       = config.BuildLabelKey("target.name")
	RequesterTeamLabelKey    = config.BuildLabelKey("requester.team")
	DeciderTeamLabelKey      = config.BuildLabelKey("decider.team")
	ActionLabelKey           = config.BuildLabelKey("action")
	ApprovalStrategyLabelKey = config.BuildLabelKey("approval.strategy")
)

// SetApprovalLabels sets filtering labels on an ApprovalRequest or Approval resource.
// These labels allow discovering resources without knowing their exact (hashed) name.
func SetApprovalLabels(obj types.Object, target types.TypedObjectRef, requester, decider, action, strategy string) {
	labels := obj.GetLabels()
	if labels == nil {
		labels = map[string]string{}
	}
	labels[TargetKindLabelKey] = labelutil.NormalizeLabelValue(target.Kind)
	labels[TargetNameLabelKey] = labelutil.NormalizeLabelValue(target.Name)
	labels[RequesterTeamLabelKey] = labelutil.NormalizeLabelValue(requester)
	labels[DeciderTeamLabelKey] = labelutil.NormalizeLabelValue(decider)
	labels[ActionLabelKey] = labelutil.NormalizeLabelValue(action)
	labels[ApprovalStrategyLabelKey] = labelutil.NormalizeLabelValue(strategy)
	obj.SetLabels(labels)
}
