// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package status

import (
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/discovery-server/internal/api"
)

func cond(t, status, reason, msg string) metav1.Condition {
	return metav1.Condition{
		Type:               t,
		Status:             metav1.ConditionStatus(status),
		Reason:             reason,
		Message:            msg,
		ObservedGeneration: 1,
		LastTransitionTime: metav1.NewTime(time.Unix(1000, 0)),
	}
}

func TestMapStatus(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		conditions []metav1.Condition
		generation int64
		wantState  api.State
		wantProc   api.ProcessingState
	}{
		{
			name:       "missing processing",
			conditions: nil,
			generation: 1,
			wantState:  api.StateNone,
			wantProc:   api.ProcessingStateNone,
		},
		{
			name: "stale processing",
			conditions: []metav1.Condition{
				cond(condition.ConditionTypeProcessing, "False", reasonDone, "done"),
				cond(condition.ConditionTypeReady, "True", "Provisioned", "ok"),
			},
			generation: 2,
			wantState:  api.StateNone,
			wantProc:   api.ProcessingStatePending,
		},
		{
			name: "missing ready",
			conditions: []metav1.Condition{
				cond(condition.ConditionTypeProcessing, "False", reasonDone, "done"),
			},
			generation: 1,
			wantState:  api.StateNone,
			wantProc:   api.ProcessingStateNone,
		},
		{
			name: "processing in progress",
			conditions: []metav1.Condition{
				cond(condition.ConditionTypeProcessing, "True", "Running", "processing"),
				cond(condition.ConditionTypeReady, "False", "Provisioning", "wait"),
			},
			generation: 1,
			wantState:  api.StateBlocked,
			wantProc:   api.ProcessingStateProcessing,
		},
		{
			name: "done and ready true",
			conditions: []metav1.Condition{
				cond(condition.ConditionTypeProcessing, "False", reasonDone, "done"),
				cond(condition.ConditionTypeReady, "True", "Provisioned", "ok"),
			},
			generation: 1,
			wantState:  api.StateComplete,
			wantProc:   api.ProcessingStateDone,
		},
		{
			name: "done and ready false",
			conditions: []metav1.Condition{
				cond(condition.ConditionTypeProcessing, "False", reasonDone, "done"),
				cond(condition.ConditionTypeReady, "False", "Broken", "not ready"),
			},
			generation: 1,
			wantState:  api.StateBlocked,
			wantProc:   api.ProcessingStateDone,
		},
		{
			name: "blocked reason",
			conditions: []metav1.Condition{
				cond(condition.ConditionTypeProcessing, "False", reasonBlocked, "blocked"),
				cond(condition.ConditionTypeReady, "False", "Blocked", "blocked"),
			},
			generation: 1,
			wantState:  api.StateBlocked,
			wantProc:   api.ProcessingStateDone,
		},
		{
			name: "failed processing",
			conditions: []metav1.Condition{
				cond(condition.ConditionTypeProcessing, "False", "Failure", "failed"),
				cond(condition.ConditionTypeReady, "False", "Broken", "broken"),
			},
			generation: 1,
			wantState:  api.StateInvalid,
			wantProc:   api.ProcessingStateFailed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MapStatus(tt.conditions, tt.generation)
			if got.State != tt.wantState || got.ProcessingState != tt.wantProc {
				t.Fatalf("expected state=%q proc=%q, got state=%q proc=%q", tt.wantState, tt.wantProc, got.State, got.ProcessingState)
			}
		})
	}
}

func TestIsProcessingStale_NoProcessingCondition(t *testing.T) {
	t.Parallel()

	if isProcessingStale(nil, 1) {
		t.Fatal("expected false when processing condition is absent")
	}
}

func TestCalculateOverallStatus(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		s    api.State
		ps   api.ProcessingState
		want string
	}{
		{name: "processing", s: api.StateNone, ps: api.ProcessingStateProcessing, want: "processing"},
		{name: "failed", s: api.StateInvalid, ps: api.ProcessingStateFailed, want: "failed"},
		{name: "blocked", s: api.StateBlocked, ps: api.ProcessingStateDone, want: "blocked"},
		{name: "pending", s: api.StateNone, ps: api.ProcessingStatePending, want: "pending"},
		{name: "complete", s: api.StateComplete, ps: api.ProcessingStateDone, want: "complete"},
		{name: "none", s: api.StateNone, ps: api.ProcessingStateNone, want: "none"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := CalculateOverallStatus(tt.s, tt.ps); got != tt.want {
				t.Fatalf("expected %q, got %q", tt.want, got)
			}
		})
	}
}
