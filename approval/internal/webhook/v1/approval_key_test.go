// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/rand"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	approvalv1 "github.com/telekom/controlplane/approval/api/v1"
	ctypes "github.com/telekom/controlplane/common/pkg/types"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("spec.approvalKey through the API server", func() {
	meta := func() metav1.ObjectMeta {
		return metav1.ObjectMeta{Name: "key-" + rand.String(8), Namespace: "default"}
	}
	target := ctypes.TypedObjectRef{
		TypeMeta:  metav1.TypeMeta{APIVersion: "testgroup.cp.ei.telekom.de/v1", Kind: "TestResource"},
		ObjectRef: ctypes.ObjectRef{Name: "target", Namespace: "default"},
	}
	requester := approvalv1.Requester{TeamName: "team", TeamEmail: "team@example.com", Reason: "test"}

	newObject := func(kind, key string) *unstructured.Unstructured {
		var typed ctrlclient.Object = &approvalv1.Approval{
			ObjectMeta: meta(),
			Spec: approvalv1.ApprovalSpec{
				Target: target, Action: "subscribe", Requester: requester,
				Strategy: approvalv1.ApprovalStrategySimple, State: approvalv1.ApprovalStatePending,
			},
		}
		if kind == "ApprovalRequest" {
			typed = &approvalv1.ApprovalRequest{
				ObjectMeta: meta(),
				Spec: approvalv1.ApprovalRequestSpec{
					Target: target, Action: "subscribe", Requester: requester,
					Strategy: approvalv1.ApprovalStrategySimple, State: approvalv1.ApprovalStatePending,
				},
			}
		}
		raw, err := runtime.DefaultUnstructuredConverter.ToUnstructured(typed)
		Expect(err).NotTo(HaveOccurred())
		obj := &unstructured.Unstructured{Object: raw}
		obj.SetGroupVersionKind(approvalv1.GroupVersion.WithKind(kind))
		setKey(obj, key)
		return obj
	}

	DescribeTable("keeps the key immutable after creation",
		func(kind, oldKey, newKey string, changeReason, accepted bool) {
			obj := newObject(kind, oldKey)
			Expect(k8sClient.Create(ctx, obj)).To(Succeed())
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, obj) })

			setKey(obj, newKey)
			if changeReason {
				Expect(unstructured.SetNestedField(obj.Object, "changed", "spec", "requester", "reason")).To(Succeed())
			}
			err := k8sClient.Update(ctx, obj)
			if accepted {
				Expect(err).NotTo(HaveOccurred())
				return
			}
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("spec.approvalKey is immutable after creation"))
		},
		func(kind, oldKey, newKey string, changeReason, accepted bool) string {
			verb := "rejects"
			if accepted {
				verb = "accepts"
			}
			if changeReason {
				return fmt.Sprintf("%s %s keeping %q while changing another field", kind, verb, oldKey)
			}
			return fmt.Sprintf("%s %s %q -> %q", kind, verb, oldKey, newKey)
		},
		Entry(nil, "Approval", absent, approvalv1.ApprovalKeyProvider, false, false),
		Entry(nil, "Approval", "", approvalv1.ApprovalKeyProvider, false, false),
		Entry(nil, "Approval", approvalv1.ApprovalKeyProvider, absent, false, false),
		Entry(nil, "Approval", approvalv1.ApprovalKeyProvider, "", false, false),
		Entry(nil, "Approval", approvalv1.ApprovalKeyConsumer, approvalv1.ApprovalKeyProvider, false, false),
		Entry(nil, "Approval", absent, "", false, true),
		Entry(nil, "Approval", "", absent, false, true),
		Entry(nil, "Approval", approvalv1.ApprovalKeyProvider, approvalv1.ApprovalKeyProvider, true, true),
		Entry(nil, "ApprovalRequest", absent, approvalv1.ApprovalKeyProvider, false, false),
		Entry(nil, "ApprovalRequest", "", approvalv1.ApprovalKeyProvider, false, false),
		Entry(nil, "ApprovalRequest", approvalv1.ApprovalKeyProvider, absent, false, false),
		Entry(nil, "ApprovalRequest", approvalv1.ApprovalKeyProvider, "", false, false),
		Entry(nil, "ApprovalRequest", approvalv1.ApprovalKeyConsumer, approvalv1.ApprovalKeyProvider, false, false),
		Entry(nil, "ApprovalRequest", absent, "", false, true),
		Entry(nil, "ApprovalRequest", "", absent, false, true),
		Entry(nil, "ApprovalRequest", approvalv1.ApprovalKeyProvider, approvalv1.ApprovalKeyProvider, true, true),
	)

	DescribeTable("accepts only an empty, provider or consumer key",
		func(kind, key string, accepted bool) {
			obj := newObject(kind, key)
			err := k8sClient.Create(ctx, obj)
			if accepted {
				Expect(err).NotTo(HaveOccurred())
				DeferCleanup(func() { _ = k8sClient.Delete(ctx, obj) })
				return
			}
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("spec.approvalKey must be empty, provider or consumer"))
		},
		Entry("Approval with an empty key", "Approval", "", true),
		Entry("Approval with the provider key", "Approval", approvalv1.ApprovalKeyProvider, true),
		Entry("Approval with the consumer key", "Approval", approvalv1.ApprovalKeyConsumer, true),
		Entry("Approval with another key", "Approval", "foo", false),
		Entry("ApprovalRequest with an empty key", "ApprovalRequest", "", true),
		Entry("ApprovalRequest with the provider key", "ApprovalRequest", approvalv1.ApprovalKeyProvider, true),
		Entry("ApprovalRequest with the consumer key", "ApprovalRequest", approvalv1.ApprovalKeyConsumer, true),
		Entry("ApprovalRequest with another key", "ApprovalRequest", "foo", false),
	)
})

// absent leaves spec.approvalKey out of the request; "" sends it explicitly.
const absent = "<absent>"

// setKey sends key explicitly, as omitempty drops an empty key from typed objects.
func setKey(obj *unstructured.Unstructured, key string) {
	if key == absent {
		unstructured.RemoveNestedField(obj.Object, "spec", "approvalKey")
		return
	}
	Expect(unstructured.SetNestedField(obj.Object, key, "spec", "approvalKey")).To(Succeed())
}
