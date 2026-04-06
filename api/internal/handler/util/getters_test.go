// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package util

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apiv1 "github.com/telekom/controlplane/api/api/v1"
	approvalbuilder "github.com/telekom/controlplane/approval/api/v1/builder"
	"github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/config"
	"github.com/telekom/controlplane/common/pkg/types"
	"github.com/telekom/controlplane/common/pkg/util/labelutil"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

var _ = Describe("HasAnySubscriptionWithFailover", func() {
	var (
		ctx        context.Context
		scheme     *runtime.Scheme
		apiExp     *apiv1.ApiExposure
		basePath   string
		namespace  string
		zoneRef    types.ObjectRef
		appRef     types.ObjectRef
		approvalId string
	)

	BeforeEach(func() {
		ctx = context.Background()
		basePath = "/api/v1"
		namespace = "test-namespace"
		approvalId = "test-approval-id"

		zoneRef = types.ObjectRef{
			Name:      "test-zone",
			Namespace: "test-env",
		}

		appRef = types.ObjectRef{
			Name:      "test-app",
			Namespace: namespace,
		}

		// Create scheme with api types
		scheme = runtime.NewScheme()
		Expect(apiv1.AddToScheme(scheme)).To(Succeed())

		// Create a basic ApiExposure for testing
		apiExp = &apiv1.ApiExposure{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-exposure",
				Namespace: namespace,
			},
			Spec: apiv1.ApiExposureSpec{
				ApiBasePath: basePath,
				Zone:        zoneRef,
			},
		}
	})

	It("should return true when an approved subscription has failover", func() {
		subscription := &apiv1.ApiSubscription{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "sub-with-failover",
				Namespace: namespace,
				Labels: map[string]string{
					apiv1.BasePathLabelKey:     labelutil.NormalizeLabelValue(basePath),
					config.EnvironmentLabelKey: namespace,
				},
			},
			Spec: apiv1.ApiSubscriptionSpec{
				ApiBasePath: basePath,
				Zone:        zoneRef,
				Requestor: apiv1.Requestor{
					Application: appRef,
				},
				Traffic: apiv1.SubscriberTraffic{
					Failover: &apiv1.Failover{
						Zones: []types.ObjectRef{
							{Name: "zone1", Namespace: "test-env"},
							{Name: "zone2", Namespace: "test-env"},
						},
					},
				},
			},
			Status: apiv1.ApiSubscriptionStatus{
				Conditions: []metav1.Condition{
					{
						Type:   approvalbuilder.ConditionTypeApprovalGranted,
						Status: metav1.ConditionTrue,
					},
				},
				Approval: &types.ObjectRef{Name: approvalId, Namespace: namespace},
			},
		}

		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(subscription).
			Build()

		ctx = client.WithClient(ctx, client.NewJanitorClient(client.NewScopedClient(fakeClient, namespace)))

		hasFailover, err := HasAnySubscriptionWithFailover(ctx, apiExp)
		Expect(err).NotTo(HaveOccurred())
		Expect(hasFailover).To(BeTrue())
	})

	It("should return false when subscriptions exist but none have failover", func() {
		subscription := &apiv1.ApiSubscription{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "sub-without-failover",
				Namespace: namespace,
				Labels: map[string]string{
					apiv1.BasePathLabelKey:     labelutil.NormalizeLabelValue(basePath),
					config.EnvironmentLabelKey: namespace,
				},
			},
			Spec: apiv1.ApiSubscriptionSpec{
				ApiBasePath: basePath,
				Zone:        zoneRef,
				Requestor: apiv1.Requestor{
					Application: appRef,
				},
				Traffic: apiv1.SubscriberTraffic{
					Failover: nil, // No failover
				},
			},
			Status: apiv1.ApiSubscriptionStatus{
				Conditions: []metav1.Condition{
					{
						Type:   approvalbuilder.ConditionTypeApprovalGranted,
						Status: metav1.ConditionTrue,
					},
				},
				Approval: &types.ObjectRef{Name: approvalId, Namespace: namespace},
			},
		}

		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(subscription).
			Build()

		ctx = client.WithClient(ctx, client.NewJanitorClient(client.NewScopedClient(fakeClient, namespace)))

		hasFailover, err := HasAnySubscriptionWithFailover(ctx, apiExp)
		Expect(err).NotTo(HaveOccurred())
		Expect(hasFailover).To(BeFalse())
	})

	It("should ignore unapproved subscriptions with failover", func() {
		approvedSub := &apiv1.ApiSubscription{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "approved-sub",
				Namespace: namespace,
				Labels: map[string]string{
					apiv1.BasePathLabelKey:     labelutil.NormalizeLabelValue(basePath),
					config.EnvironmentLabelKey: namespace,
				},
			},
			Spec: apiv1.ApiSubscriptionSpec{
				ApiBasePath: basePath,
				Zone:        zoneRef,
				Requestor: apiv1.Requestor{
					Application: appRef,
				},
				Traffic: apiv1.SubscriberTraffic{
					Failover: nil, // No failover
				},
			},
			Status: apiv1.ApiSubscriptionStatus{
				Conditions: []metav1.Condition{
					{
						Type:   approvalbuilder.ConditionTypeApprovalGranted,
						Status: metav1.ConditionTrue,
					},
				},
				Approval: &types.ObjectRef{Name: approvalId, Namespace: namespace},
			},
		}

		unapprovedSub := &apiv1.ApiSubscription{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "unapproved-sub",
				Namespace: namespace,
				Labels: map[string]string{
					apiv1.BasePathLabelKey:     labelutil.NormalizeLabelValue(basePath),
					config.EnvironmentLabelKey: namespace,
				},
			},
			Spec: apiv1.ApiSubscriptionSpec{
				ApiBasePath: basePath,
				Zone:        zoneRef,
				Requestor: apiv1.Requestor{
					Application: appRef,
				},
				Traffic: apiv1.SubscriberTraffic{
					Failover: &apiv1.Failover{
						Zones: []types.ObjectRef{
							{Name: "zone1", Namespace: "test-env"},
						},
					},
				},
			},
			Status: apiv1.ApiSubscriptionStatus{
				Conditions: []metav1.Condition{
					{
						Type:   approvalbuilder.ConditionTypeApprovalGranted,
						Status: metav1.ConditionFalse, // Not approved
					},
				},
			},
		}

		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(approvedSub, unapprovedSub).
			Build()

		ctx = client.WithClient(ctx, client.NewJanitorClient(client.NewScopedClient(fakeClient, namespace)))

		hasFailover, err := HasAnySubscriptionWithFailover(ctx, apiExp)
		Expect(err).NotTo(HaveOccurred())
		Expect(hasFailover).To(BeFalse())
	})

	It("should ignore subscriptions marked for deletion", func() {
		now := metav1.Now()
		deletedSub := &apiv1.ApiSubscription{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "deleted-sub",
				Namespace:         namespace,
				DeletionTimestamp: &now,
				Finalizers:        []string{"test-finalizer"},
				Labels: map[string]string{
					apiv1.BasePathLabelKey:     labelutil.NormalizeLabelValue(basePath),
					config.EnvironmentLabelKey: namespace,
				},
			},
			Spec: apiv1.ApiSubscriptionSpec{
				ApiBasePath: basePath,
				Zone:        zoneRef,
				Requestor: apiv1.Requestor{
					Application: appRef,
				},
				Traffic: apiv1.SubscriberTraffic{
					Failover: &apiv1.Failover{
						Zones: []types.ObjectRef{
							{Name: "zone1", Namespace: "test-env"},
						},
					},
				},
			},
			Status: apiv1.ApiSubscriptionStatus{
				Conditions: []metav1.Condition{
					{
						Type:   approvalbuilder.ConditionTypeApprovalGranted,
						Status: metav1.ConditionTrue,
					},
				},
				Approval: &types.ObjectRef{Name: approvalId, Namespace: namespace},
			},
		}

		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(deletedSub).
			Build()

		ctx = client.WithClient(ctx, client.NewJanitorClient(client.NewScopedClient(fakeClient, namespace)))

		hasFailover, err := HasAnySubscriptionWithFailover(ctx, apiExp)
		Expect(err).NotTo(HaveOccurred())
		Expect(hasFailover).To(BeFalse())
	})

	It("should ignore subscriptions for different base paths", func() {
		differentBasePathSub := &apiv1.ApiSubscription{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "different-basepath-sub",
				Namespace: namespace,
				Labels: map[string]string{
					apiv1.BasePathLabelKey: "/different/api",
				},
			},
			Spec: apiv1.ApiSubscriptionSpec{
				ApiBasePath: "/different/api",
				Zone:        zoneRef,
				Requestor: apiv1.Requestor{
					Application: appRef,
				},
				Traffic: apiv1.SubscriberTraffic{
					Failover: &apiv1.Failover{
						Zones: []types.ObjectRef{
							{Name: "zone1", Namespace: "test-env"},
						},
					},
				},
			},
			Status: apiv1.ApiSubscriptionStatus{
				Conditions: []metav1.Condition{
					{
						Type:   approvalbuilder.ConditionTypeApprovalGranted,
						Status: metav1.ConditionTrue,
					},
				},
				Approval: &types.ObjectRef{Name: approvalId, Namespace: namespace},
			},
		}

		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(differentBasePathSub).
			Build()

		ctx = client.WithClient(ctx, client.NewJanitorClient(client.NewScopedClient(fakeClient, namespace)))

		hasFailover, err := HasAnySubscriptionWithFailover(ctx, apiExp)
		Expect(err).NotTo(HaveOccurred())
		Expect(hasFailover).To(BeFalse())
	})

	It("should return false when no subscriptions exist", func() {
		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			Build()

		ctx = client.WithClient(ctx, client.NewJanitorClient(client.NewScopedClient(fakeClient, namespace)))

		hasFailover, err := HasAnySubscriptionWithFailover(ctx, apiExp)
		Expect(err).NotTo(HaveOccurred())
		Expect(hasFailover).To(BeFalse())
	})

	It("should return true when at least one approved subscription has failover (mixed scenario)", func() {
		subWithFailover := &apiv1.ApiSubscription{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "sub-with-failover",
				Namespace: namespace,
				Labels: map[string]string{
					apiv1.BasePathLabelKey:     labelutil.NormalizeLabelValue(basePath),
					config.EnvironmentLabelKey: namespace,
				},
			},
			Spec: apiv1.ApiSubscriptionSpec{
				ApiBasePath: basePath,
				Zone:        zoneRef,
				Requestor: apiv1.Requestor{
					Application: appRef,
				},
				Traffic: apiv1.SubscriberTraffic{
					Failover: &apiv1.Failover{
						Zones: []types.ObjectRef{
							{Name: "zone1", Namespace: "test-env"},
						},
					},
				},
			},
			Status: apiv1.ApiSubscriptionStatus{
				Conditions: []metav1.Condition{
					{
						Type:   approvalbuilder.ConditionTypeApprovalGranted,
						Status: metav1.ConditionTrue,
					},
				},
				Approval: &types.ObjectRef{Name: approvalId, Namespace: namespace},
			},
		}

		subWithoutFailover := &apiv1.ApiSubscription{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "sub-without-failover",
				Namespace: namespace,
				Labels: map[string]string{
					apiv1.BasePathLabelKey:     labelutil.NormalizeLabelValue(basePath),
					config.EnvironmentLabelKey: namespace,
				},
			},
			Spec: apiv1.ApiSubscriptionSpec{
				ApiBasePath: basePath,
				Zone:        zoneRef,
				Requestor: apiv1.Requestor{
					Application: appRef,
				},
				Traffic: apiv1.SubscriberTraffic{
					Failover: nil,
				},
			},
			Status: apiv1.ApiSubscriptionStatus{
				Conditions: []metav1.Condition{
					{
						Type:   approvalbuilder.ConditionTypeApprovalGranted,
						Status: metav1.ConditionTrue,
					},
				},
				Approval: &types.ObjectRef{Name: approvalId + "-2", Namespace: namespace},
			},
		}

		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(subWithFailover, subWithoutFailover).
			Build()

		ctx = client.WithClient(ctx, client.NewJanitorClient(client.NewScopedClient(fakeClient, namespace)))

		hasFailover, err := HasAnySubscriptionWithFailover(ctx, apiExp)
		Expect(err).NotTo(HaveOccurred())
		Expect(hasFailover).To(BeTrue())
	})
})
