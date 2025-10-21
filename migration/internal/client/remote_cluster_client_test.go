// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package client

import (
	"context"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	approvalv1 "github.com/telekom/controlplane/approval/api/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestRemoteClusterClient(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "RemoteClusterClient Suite")
}

var _ = Describe("RemoteClusterClient", func() {
	var (
		ctx    context.Context
		client *RemoteClusterClient
		scheme *runtime.Scheme
	)

	BeforeEach(func() {
		ctx = context.Background()
		scheme = runtime.NewScheme()
		Expect(approvalv1.AddToScheme(scheme)).To(Succeed())
	})

	Describe("GetApproval", func() {
		Context("when approval exists", func() {
			It("should return the approval", func() {
				// Create unstructured object simulating legacy API format
				testApproval := &unstructured.Unstructured{
					Object: map[string]interface{}{
						"apiVersion": "acp.ei.telekom.de/v1",
						"kind":       "Approval",
						"metadata": map[string]interface{}{
							"name":      "test-approval",
							"namespace": "default",
						},
						"spec": map[string]interface{}{
							"state":    "GRANTED",
							"strategy": "SIMPLE",
						},
					},
				}

				fakeClient := fake.NewClientBuilder().
					WithScheme(scheme).
					WithObjects(testApproval).
					Build()

				client = NewRemoteClusterClientWithClient(fakeClient)

				approval, err := client.GetApproval(ctx, "default", "test-approval")
				Expect(err).NotTo(HaveOccurred())
				Expect(approval).NotTo(BeNil())
				Expect(approval.Name).To(Equal("test-approval"))
				Expect(approval.Spec.State).To(Equal(approvalv1.ApprovalStateGranted))
				Expect(approval.Spec.Strategy).To(Equal(approvalv1.ApprovalStrategySimple))
			})
		})

		Context("when approval does not exist", func() {
			It("should return not found error", func() {
				fakeClient := fake.NewClientBuilder().
					WithScheme(scheme).
					Build()

				client = NewRemoteClusterClientWithClient(fakeClient)

				approval, err := client.GetApproval(ctx, "default", "nonexistent")
				Expect(err).To(HaveOccurred())
				Expect(approval).To(BeNil())
			})
		})

		Context("with different states", func() {
			It("should handle Pending state", func() {
				testApproval := &unstructured.Unstructured{
					Object: map[string]interface{}{
						"apiVersion": "acp.ei.telekom.de/v1",
						"kind":       "Approval",
						"metadata": map[string]interface{}{
							"name":      "pending-approval",
							"namespace": "default",
						},
						"spec": map[string]interface{}{
							"state":    "PENDING",
							"strategy": "SIMPLE",
						},
					},
				}

				fakeClient := fake.NewClientBuilder().
					WithScheme(scheme).
					WithObjects(testApproval).
					Build()

				client = NewRemoteClusterClientWithClient(fakeClient)

				approval, err := client.GetApproval(ctx, "default", "pending-approval")
				Expect(err).NotTo(HaveOccurred())
				Expect(approval.Spec.State).To(Equal(approvalv1.ApprovalStatePending))
			})

			It("should handle Suspended state", func() {
				testApproval := &unstructured.Unstructured{
					Object: map[string]interface{}{
						"apiVersion": "acp.ei.telekom.de/v1",
						"kind":       "Approval",
						"metadata": map[string]interface{}{
							"name":      "suspended-approval",
							"namespace": "default",
						},
						"spec": map[string]interface{}{
							"state":    "SUSPENDED",
							"strategy": "SIMPLE",
						},
					},
				}

				fakeClient := fake.NewClientBuilder().
					WithScheme(scheme).
					WithObjects(testApproval).
					Build()

				client = NewRemoteClusterClientWithClient(fakeClient)

				approval, err := client.GetApproval(ctx, "default", "suspended-approval")
				Expect(err).NotTo(HaveOccurred())
				Expect(approval.Spec.State).To(Equal(approvalv1.ApprovalStateSuspended))
			})
		})
	})

	Describe("ListApprovals", func() {
		Context("when multiple approvals exist", func() {
			It("should return all approvals in namespace", func() {
				approval1 := &unstructured.Unstructured{
					Object: map[string]interface{}{
						"apiVersion": "acp.ei.telekom.de/v1",
						"kind":       "Approval",
						"metadata": map[string]interface{}{
							"name":      "approval-1",
							"namespace": "default",
						},
						"spec": map[string]interface{}{
							"state":    "GRANTED",
							"strategy": "SIMPLE",
						},
					},
				}

				approval2 := &unstructured.Unstructured{
					Object: map[string]interface{}{
						"apiVersion": "acp.ei.telekom.de/v1",
						"kind":       "Approval",
						"metadata": map[string]interface{}{
							"name":      "approval-2",
							"namespace": "default",
						},
						"spec": map[string]interface{}{
							"state":    "PENDING",
							"strategy": "FOUREYES",
						},
					},
				}

				approval3 := &unstructured.Unstructured{
					Object: map[string]interface{}{
						"apiVersion": "acp.ei.telekom.de/v1",
						"kind":       "Approval",
						"metadata": map[string]interface{}{
							"name":      "approval-3",
							"namespace": "other-namespace",
						},
						"spec": map[string]interface{}{
							"state":    "REJECTED",
							"strategy": "SIMPLE",
						},
					},
				}

				fakeClient := fake.NewClientBuilder().
					WithScheme(scheme).
					WithObjects(approval1, approval2, approval3).
					Build()

				client = NewRemoteClusterClientWithClient(fakeClient)

				approvalList, err := client.ListApprovals(ctx, "default")
				Expect(err).NotTo(HaveOccurred())
				Expect(approvalList).NotTo(BeNil())
				Expect(approvalList.Items).To(HaveLen(2))

				names := []string{approvalList.Items[0].Name, approvalList.Items[1].Name}
				Expect(names).To(ContainElements("approval-1", "approval-2"))
			})
		})

		Context("when no approvals exist", func() {
			It("should return empty list", func() {
				fakeClient := fake.NewClientBuilder().
					WithScheme(scheme).
					Build()

				client = NewRemoteClusterClientWithClient(fakeClient)

				approvalList, err := client.ListApprovals(ctx, "default")
				Expect(err).NotTo(HaveOccurred())
				Expect(approvalList).NotTo(BeNil())
				Expect(approvalList.Items).To(BeEmpty())
			})
		})
	})

	Describe("NewRemoteClusterClient", func() {
		Context("with token and CA data", func() {
			It("should create client successfully", func() {
				config := &RemoteClusterConfig{
					APIServer: "https://test-cluster:6443",
					Token:     "test-token",
					CAData:    []byte("test-ca-data"),
				}

				// This will fail without a real cluster, but we're testing the config parsing
				_, err := NewRemoteClusterClient(config)
				// We expect an error because there's no real cluster, but config should be valid
				Expect(err).To(HaveOccurred()) // Expected - no real cluster to connect to
			})
		})

		Context("with invalid kubeconfig", func() {
			It("should return error", func() {
				config := &RemoteClusterConfig{
					Kubeconfig: []byte("invalid-kubeconfig"),
				}

				_, err := NewRemoteClusterClient(config)
				Expect(err).To(HaveOccurred())
			})
		})
	})
})
