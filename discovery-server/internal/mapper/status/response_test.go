// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package status

import (
	"testing"
	"time"

	applicationv1 "github.com/telekom/controlplane/application/api/v1"
	"github.com/telekom/controlplane/common/pkg/condition"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestMapResponse(t *testing.T) {
	t.Parallel()

	createdAt := metav1.NewTime(time.Unix(100, 0))
	processedAt := metav1.NewTime(time.Unix(200, 0))
	obj := &applicationv1.Application{}
	obj.CreationTimestamp = createdAt
	obj.Generation = 1
	obj.Status.Conditions = []metav1.Condition{
		{
			Type:               condition.ConditionTypeReady,
			Status:             metav1.ConditionTrue,
			Reason:             "Provisioned",
			ObservedGeneration: 1,
		},
		{
			Type:               condition.ConditionTypeProcessing,
			Status:             metav1.ConditionFalse,
			Reason:             "Done",
			ObservedGeneration: 1,
			LastTransitionTime: processedAt,
		},
	}

	resp := MapResponse(obj)

	if !resp.CreatedAt.Equal(createdAt.Time) {
		t.Fatalf("unexpected createdAt: %v", resp.CreatedAt)
	}
	if !resp.ProcessedAt.Equal(processedAt.Time) {
		t.Fatalf("unexpected processedAt: %v", resp.ProcessedAt)
	}
	if resp.OverallStatus != "complete" {
		t.Fatalf("unexpected overallStatus: %q", resp.OverallStatus)
	}
}
