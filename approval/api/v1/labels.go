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
	ApprovalKeyLabelKey      = config.BuildLabelKey("approval.key")
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

// SetApprovalKeyLabel sets or removes the approval-key mirror label based on
// the spec key value. A non-empty key sets the label; an empty/absent key
// removes it. Unrelated labels are preserved.
func SetApprovalKeyLabel(obj types.Object, approvalKey string) {
	labels := obj.GetLabels()
	if labels == nil {
		labels = map[string]string{}
	}
	if approvalKey != "" {
		labels[ApprovalKeyLabelKey] = labelutil.NormalizeLabelValue(approvalKey)
	} else {
		delete(labels, ApprovalKeyLabelKey)
	}
	obj.SetLabels(labels)
}
