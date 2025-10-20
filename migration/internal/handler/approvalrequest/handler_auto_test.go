// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package approvalrequest

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	approvalv1 "github.com/telekom/controlplane/approval/api/v1"
	"github.com/telekom/controlplane/common/pkg/types"
	"github.com/telekom/controlplane/migration/internal/mapper"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// TestHandleAutoStrategy tests the Auto strategy handling logic
func TestHandleAutoStrategy(t *testing.T) {
	tests := []struct {
		name                string
		approvalRequest     *approvalv1.ApprovalRequest
		approval            *approvalv1.Approval // The Approval in new cluster
		legacyApproval      *approvalv1.Approval
		expectUpdate        bool
		expectedState       approvalv1.ApprovalState
		expectedAnnotations map[string]string
	}{
		{
			name: "should set Approval to Rejected when legacy is Auto+Suspended",
			approvalRequest: &approvalv1.ApprovalRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-request",
					Namespace: "controlplane--eni--hyperion",
				},
				Spec: approvalv1.ApprovalRequestSpec{
					Strategy: approvalv1.ApprovalStrategyAuto,
					State:    approvalv1.ApprovalStateGranted, // Auto requests are always granted
				},
				Status: approvalv1.ApprovalRequestStatus{
					Approval: types.ObjectRef{
						Name: "test-approval-12345", // Name of the created Approval
					},
				},
			},
			approval: &approvalv1.Approval{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-approval-12345", // Different from ApprovalRequest name
					Namespace: "controlplane--eni--hyperion",
				},
				Spec: approvalv1.ApprovalSpec{
					Strategy: approvalv1.ApprovalStrategyAuto,
					State:    approvalv1.ApprovalStateGranted, // Auto-created as Granted
				},
			},
			legacyApproval: &approvalv1.Approval{
				Spec: approvalv1.ApprovalSpec{
					Strategy: approvalv1.ApprovalStrategyAuto,
					State:    approvalv1.ApprovalStateSuspended,
				},
			},
			expectUpdate:  true,
			expectedState: approvalv1.ApprovalStateRejected,
			expectedAnnotations: map[string]string{
				"migration.cp.ei.telekom.de/last-migrated-state": "Rejected",
				"migration.cp.ei.telekom.de/reason":              "Auto strategy with Suspended state in legacy",
			},
		},
		{
			name: "should not update when Approval already Rejected",
			approvalRequest: &approvalv1.ApprovalRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-request",
					Namespace: "controlplane--eni--hyperion",
				},
				Spec: approvalv1.ApprovalRequestSpec{
					Strategy: approvalv1.ApprovalStrategyAuto,
					State:    approvalv1.ApprovalStateGranted,
				},
				Status: approvalv1.ApprovalRequestStatus{
					Approval: types.ObjectRef{
						Name: "test-approval-already-rejected",
					},
				},
			},
			approval: &approvalv1.Approval{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-approval-already-rejected",
					Namespace: "controlplane--eni--hyperion",
				},
				Spec: approvalv1.ApprovalSpec{
					Strategy: approvalv1.ApprovalStrategyAuto,
					State:    approvalv1.ApprovalStateRejected, // Already rejected
				},
			},
			legacyApproval: &approvalv1.Approval{
				Spec: approvalv1.ApprovalSpec{
					Strategy: approvalv1.ApprovalStrategyAuto,
					State:    approvalv1.ApprovalStateSuspended,
				},
			},
			expectUpdate:  false,
			expectedState: approvalv1.ApprovalStateRejected,
		},
		{
			name: "should skip when legacy is Auto+Granted",
			approvalRequest: &approvalv1.ApprovalRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-request",
					Namespace: "controlplane--eni--hyperion",
				},
				Spec: approvalv1.ApprovalRequestSpec{
					Strategy: approvalv1.ApprovalStrategyAuto,
					State:    approvalv1.ApprovalStateGranted,
				},
				Status: approvalv1.ApprovalRequestStatus{
					Approval: types.ObjectRef{
						Name: "test-approval-granted",
					},
				},
			},
			approval: &approvalv1.Approval{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-approval-granted",
					Namespace: "controlplane--eni--hyperion",
				},
				Spec: approvalv1.ApprovalSpec{
					Strategy: approvalv1.ApprovalStrategyAuto,
					State:    approvalv1.ApprovalStateGranted,
				},
			},
			legacyApproval: &approvalv1.Approval{
				Spec: approvalv1.ApprovalSpec{
					Strategy: approvalv1.ApprovalStrategyAuto,
					State:    approvalv1.ApprovalStateGranted,
				},
			},
			expectUpdate:  false,
			expectedState: approvalv1.ApprovalStateGranted,
		},
		{
			name: "should skip when legacy is not Auto strategy",
			approvalRequest: &approvalv1.ApprovalRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-request",
					Namespace: "controlplane--eni--hyperion",
				},
				Spec: approvalv1.ApprovalRequestSpec{
					Strategy: approvalv1.ApprovalStrategyAuto,
					State:    approvalv1.ApprovalStateGranted,
				},
				Status: approvalv1.ApprovalRequestStatus{
					Approval: types.ObjectRef{
						Name: "test-approval-simple",
					},
				},
			},
			approval: &approvalv1.Approval{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-approval-simple",
					Namespace: "controlplane--eni--hyperion",
				},
				Spec: approvalv1.ApprovalSpec{
					Strategy: approvalv1.ApprovalStrategyAuto,
					State:    approvalv1.ApprovalStateGranted,
				},
			},
			legacyApproval: &approvalv1.Approval{
				Spec: approvalv1.ApprovalSpec{
					Strategy: approvalv1.ApprovalStrategySimple,
					State:    approvalv1.ApprovalStateSuspended,
				},
			},
			expectUpdate:  false,
			expectedState: approvalv1.ApprovalStateGranted,
		},
		{
			name: "should skip when legacy is Auto but not Suspended",
			approvalRequest: &approvalv1.ApprovalRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-request",
					Namespace: "controlplane--eni--hyperion",
				},
				Spec: approvalv1.ApprovalRequestSpec{
					Strategy: approvalv1.ApprovalStrategyAuto,
					State:    approvalv1.ApprovalStateGranted,
				},
				Status: approvalv1.ApprovalRequestStatus{
					Approval: types.ObjectRef{
						Name: "test-approval-pending",
					},
				},
			},
			approval: &approvalv1.Approval{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-approval-pending",
					Namespace: "controlplane--eni--hyperion",
				},
				Spec: approvalv1.ApprovalSpec{
					Strategy: approvalv1.ApprovalStrategyAuto,
					State:    approvalv1.ApprovalStateGranted,
				},
			},
			legacyApproval: &approvalv1.Approval{
				Spec: approvalv1.ApprovalSpec{
					Strategy: approvalv1.ApprovalStrategyAuto,
					State:    approvalv1.ApprovalStatePending,
				},
			},
			expectUpdate:  false,
			expectedState: approvalv1.ApprovalStateGranted,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup fake client
			scheme := runtime.NewScheme()
			_ = approvalv1.AddToScheme(scheme)

			objects := []client.Object{tt.approvalRequest}
			if tt.approval != nil {
				objects = append(objects, tt.approval)
			}

			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(objects...).
				Build()

			// Create handler
			handler := &MigrationHandler{
				Client: fakeClient,
				Mapper: mapper.NewApprovalMapper(),
				Log:    logr.Discard(),
			}

			// Run the handler
			ctx := context.Background()
			err := handler.handleAutoStrategy(ctx, tt.approvalRequest, tt.legacyApproval, "test-approval")

			// Verify no error
			if err != nil {
				t.Fatalf("handleAutoStrategy() error = %v", err)
			}

			// If Approval exists, check it was updated correctly
			if tt.approval != nil {
				// Get the updated Approval
				updatedApproval := &approvalv1.Approval{}
				key := client.ObjectKey{
					Name:      tt.approval.Name,
					Namespace: tt.approval.Namespace,
				}
				if err := fakeClient.Get(ctx, key, updatedApproval); err != nil {
					t.Fatalf("Failed to get updated Approval: %v", err)
				}

				// Verify state
				if updatedApproval.Spec.State != tt.expectedState {
					t.Errorf("Expected Approval state %v, got %v", tt.expectedState, updatedApproval.Spec.State)
				}

				// Verify annotations if expected
				if tt.expectedAnnotations != nil {
					for key, expectedValue := range tt.expectedAnnotations {
						actualValue, exists := updatedApproval.Annotations[key]
						if !exists {
							t.Errorf("Expected annotation %s not found", key)
						} else if actualValue != expectedValue {
							t.Errorf("Expected annotation %s=%s, got %s", key, expectedValue, actualValue)
						}
					}
				}
			}
		})
	}
}
