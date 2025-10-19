// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package approvalrequest

import (
	"context"
	"testing"

	approvalv1 "github.com/telekom/controlplane/approval/api/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
)

func TestMigrationHandler_ComputeLegacyIdentifier(t *testing.T) {
	tests := []struct {
		name            string
		approvalRequest *approvalv1.ApprovalRequest
		wantNamespace   string
		wantName        string
		wantSkip        bool
		wantErr         bool
	}{
		{
			name: "should skip Auto strategy",
			approvalRequest: &approvalv1.ApprovalRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-request",
					Namespace: "controlplane--eni--hyperion",
				},
				Spec: approvalv1.ApprovalRequestSpec{
					Strategy: approvalv1.ApprovalStrategyAuto,
				},
			},
			wantSkip: true,
		},
		{
			name: "should skip when no owner references",
			approvalRequest: &approvalv1.ApprovalRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-request",
					Namespace: "controlplane--eni--hyperion",
				},
				Spec: approvalv1.ApprovalRequestSpec{
					Strategy: approvalv1.ApprovalStrategySimple,
				},
			},
			wantSkip: true,
		},
		{
			name: "should compute legacy identifier with component swap",
			approvalRequest: &approvalv1.ApprovalRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-request",
					Namespace: "controlplane--eni--hyperion",
					OwnerReferences: []metav1.OwnerReference{
						{
							Kind: "ApiSubscription",
							Name: "rover-name--api-name",
						},
					},
				},
				Spec: approvalv1.ApprovalRequestSpec{
					Strategy: approvalv1.ApprovalStrategySimple,
				},
			},
			wantNamespace: "eni--hyperion",
			wantName:      "apisubscription--api-name--rover-name",
			wantSkip:      false,
		},
		{
			name: "should handle namespace without environment prefix",
			approvalRequest: &approvalv1.ApprovalRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-request",
					Namespace: "eni--hyperion",
					OwnerReferences: []metav1.OwnerReference{
						{
							Kind: "ApiSubscription",
							Name: "simple-name",
						},
					},
				},
				Spec: approvalv1.ApprovalRequestSpec{
					Strategy: approvalv1.ApprovalStrategySimple,
				},
			},
			wantNamespace: "eni--hyperion",
			wantName:      "apisubscription--simple-name",
			wantSkip:      false,
		},
	}

	mapper := NewApprovalMapper()
	handler := NewMigrationHandler(mapper, ctrl.Log.WithName("test"))

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()

			gotNamespace, gotName, gotSkip, err := handler.ComputeLegacyIdentifier(ctx, tt.approvalRequest)

			if (err != nil) != tt.wantErr {
				t.Errorf("ComputeLegacyIdentifier() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if gotSkip != tt.wantSkip {
				t.Errorf("ComputeLegacyIdentifier() gotSkip = %v, want %v", gotSkip, tt.wantSkip)
			}

			if !gotSkip {
				if gotNamespace != tt.wantNamespace {
					t.Errorf("ComputeLegacyIdentifier() gotNamespace = %v, want %v", gotNamespace, tt.wantNamespace)
				}
				if gotName != tt.wantName {
					t.Errorf("ComputeLegacyIdentifier() gotName = %v, want %v", gotName, tt.wantName)
				}
			}
		})
	}
}

func TestMigrationHandler_HasChanged(t *testing.T) {
	tests := []struct {
		name            string
		approvalRequest *approvalv1.ApprovalRequest
		approval        *approvalv1.Approval
		want            bool
	}{
		{
			name: "should return true when no annotation exists",
			approvalRequest: &approvalv1.ApprovalRequest{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{},
				},
				Spec: approvalv1.ApprovalRequestSpec{
					State: approvalv1.ApprovalStatePending,
				},
			},
			approval: &approvalv1.Approval{
				Spec: approvalv1.ApprovalSpec{
					State: approvalv1.ApprovalStateGranted,
				},
			},
			want: true,
		},
		{
			name: "should return false when state unchanged",
			approvalRequest: &approvalv1.ApprovalRequest{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"migration.cp.ei.telekom.de/last-migrated-state": "Granted",
					},
				},
				Spec: approvalv1.ApprovalRequestSpec{
					State: approvalv1.ApprovalStateGranted,
				},
			},
			approval: &approvalv1.Approval{
				Spec: approvalv1.ApprovalSpec{
					State: approvalv1.ApprovalStateGranted,
				},
			},
			want: false,
		},
		{
			name: "should return true when state changed",
			approvalRequest: &approvalv1.ApprovalRequest{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"migration.cp.ei.telekom.de/last-migrated-state": "Pending",
					},
				},
				Spec: approvalv1.ApprovalRequestSpec{
					State: approvalv1.ApprovalStatePending,
				},
			},
			approval: &approvalv1.Approval{
				Spec: approvalv1.ApprovalSpec{
					State: approvalv1.ApprovalStateGranted,
				},
			},
			want: true,
		},
		{
			name: "should handle Suspended -> Rejected mapping",
			approvalRequest: &approvalv1.ApprovalRequest{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"migration.cp.ei.telekom.de/last-migrated-state": "Rejected",
					},
				},
				Spec: approvalv1.ApprovalRequestSpec{
					State: approvalv1.ApprovalStateRejected,
				},
			},
			approval: &approvalv1.Approval{
				Spec: approvalv1.ApprovalSpec{
					State: approvalv1.ApprovalStateSuspended,
				},
			},
			want: false, // Suspended maps to Rejected, so no change
		},
	}

	mapper := NewApprovalMapper()
	handler := NewMigrationHandler(mapper, ctrl.Log.WithName("test"))

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()

			got := handler.HasChanged(ctx, tt.approvalRequest, tt.approval)

			if got != tt.want {
				t.Errorf("HasChanged() = %v, want %v", got, tt.want)
			}
		})
	}
}
