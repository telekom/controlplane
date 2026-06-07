// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"context"
	"testing"

	approvalv1 "github.com/telekom/controlplane/approval/api/v1"
	"github.com/telekom/controlplane/controlplane-api/internal/resolvers/model"
	"github.com/telekom/controlplane/controlplane-api/internal/viewer"
)

func TestBuildDecision_StampsUserIdentity(t *testing.T) {
	ctx := viewer.NewContext(context.Background(), &viewer.Viewer{
		Admin:     true,
		UserName:  "Jane Doe",
		UserEmail: "jane@example.com",
	})
	comment := "LGTM"
	input := model.DecisionInput{
		Action:  model.ApprovalActionAllow,
		Comment: &comment,
	}

	d := buildDecision(ctx, input, approvalv1.ApprovalStateGranted)

	if d.Name != "Jane Doe" {
		t.Errorf("expected Name %q, got %q", "Jane Doe", d.Name)
	}
	if d.Email != "jane@example.com" {
		t.Errorf("expected Email %q, got %q", "jane@example.com", d.Email)
	}
	if d.Comment != "LGTM" {
		t.Errorf("expected Comment %q, got %q", "LGTM", d.Comment)
	}
	if d.ResultingState != approvalv1.ApprovalStateGranted {
		t.Errorf("expected ResultingState %q, got %q", approvalv1.ApprovalStateGranted, d.ResultingState)
	}
	if d.Timestamp == nil {
		t.Error("expected Timestamp to be set")
	}
}

func TestBuildDecision_NoViewer(t *testing.T) {
	input := model.DecisionInput{
		Action: model.ApprovalActionAllow,
	}

	d := buildDecision(context.Background(), input, approvalv1.ApprovalStateGranted)

	if d.Name != "" {
		t.Errorf("expected empty Name, got %q", d.Name)
	}
	if d.Email != "" {
		t.Errorf("expected empty Email, got %q", d.Email)
	}
}
