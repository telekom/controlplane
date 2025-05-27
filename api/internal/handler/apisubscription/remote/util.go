// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package remote

import (
	"context"
	"strings"

	apiapi "github.com/telekom/controlplane/api/api/v1"
)

type ApprovalResult string

const (
	ApprovalResultContinue ApprovalResult = "Continue"
	ApprovalResultBlock    ApprovalResult = "Block"
	ApprovalResultCleanup  ApprovalResult = "Cleanup"
)

func IsApproved(ctx context.Context, req *apiapi.RemoteApiSubscription) (res ApprovalResult) {
	if req.Status.Approval == nil || req.Status.ApprovalRequest == nil {
		return ApprovalResultCleanup
	}

	approvalGranted := strings.EqualFold("granted", req.Status.Approval.ApprovalState)
	approvalRequestGranted := strings.EqualFold("granted", req.Status.ApprovalRequest.ApprovalState)

	if !approvalGranted {
		return ApprovalResultCleanup
	}

	if approvalGranted && approvalRequestGranted {
		return ApprovalResultContinue
	}

	return ApprovalResultBlock
}
