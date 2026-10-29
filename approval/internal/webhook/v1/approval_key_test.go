// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/rand"

	approvalv1 "github.com/telekom/controlplane/approval/api/v1"
	ctypes "github.com/telekom/controlplane/common/pkg/types"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("spec.approvalKey through the API server", func() {
	newApproval := func(key string) *approvalv1.Approval {
		return &approvalv1.Approval{
			ObjectMeta: metav1.ObjectMeta{Name: "key-" + rand.String(8), Namespace: "default"},
			Spec: approvalv1.ApprovalSpec{
				Target: ctypes.TypedObjectRef{
					TypeMeta:  metav1.TypeMeta{APIVersion: "testgroup.cp.ei.telekom.de/v1", Kind: "TestResource"},
					ObjectRef: ctypes.ObjectRef{Name: "target", Namespace: "default"},
				},
				ApprovalKey: key,
				Action:      "subscribe",
				Strategy:    approvalv1.ApprovalStrategySimple,
				State:       approvalv1.ApprovalStatePending,
				Requester:   approvalv1.Requester{TeamName: "team", TeamEmail: "team@example.com", Reason: "test"},
			},
		}
	}

	It("rejects changing the key with the CRD rule", func() {
		approval := newApproval("provider")
		Expect(k8sClient.Create(ctx, approval)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, approval) })

		approval.Spec.ApprovalKey = "consumer"
		err := k8sClient.Update(ctx, approval)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("spec.approvalKey is immutable after creation"))
	})
})
