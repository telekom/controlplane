// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package util_test

import (
	"context"
	"fmt"
	"time"

	mock "github.com/stretchr/testify/mock"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	agenticv1 "github.com/telekom/controlplane/agentic/api/v1"
	"github.com/telekom/controlplane/agentic/internal/handler/util"
	applicationapi "github.com/telekom/controlplane/application/api/v1"
	cclient "github.com/telekom/controlplane/common/pkg/client"
	fakeclient "github.com/telekom/controlplane/common/pkg/client/fake"
	ctypes "github.com/telekom/controlplane/common/pkg/types"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// ---------- MakeMcpRouteName ----------

var _ = Describe("MakeMcpRouteName", func() {
	DescribeTable("should produce correct route names",
		func(basePath, expected string) {
			Expect(util.MakeMcpRouteName(basePath)).To(Equal(expected))
		},
		Entry("simple path", "/mcp/weather/v1", "ai-gateway--mcp-weather-v1"),
		Entry("single segment", "/mcp", "ai-gateway--mcp"),
		Entry("deep path", "/mcp/tools/search/v2", "ai-gateway--mcp-tools-search-v2"),
		Entry("trailing slashes normalised", "/mcp/v1", "ai-gateway--mcp-v1"),
	)
})

// ---------- GetZone ----------

var _ = Describe("GetZone", func() {
	var (
		ctx        context.Context
		fakeClient *fakeclient.MockJanitorClient
		ref        client.ObjectKey
	)

	BeforeEach(func() {
		ctx = context.Background()
		fakeClient = fakeclient.NewMockJanitorClient(GinkgoT())
		ctx = cclient.WithClient(ctx, fakeClient)
		ref = client.ObjectKey{Name: "zone-a", Namespace: "default"}
	})

	It("should return zone when found and ready", func() {
		zone := makeReadyZone("zone-a")

		fakeClient.EXPECT().
			Get(ctx, k8stypes.NamespacedName{Name: "zone-a", Namespace: "default"}, &adminv1.Zone{}).
			Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
				*out.(*adminv1.Zone) = *zone
			}).
			Return(nil)

		result, err := util.GetZone(ctx, ref)
		Expect(err).ToNot(HaveOccurred())
		Expect(result.Name).To(Equal("zone-a"))
	})

	It("should return BlockedError when zone is not found", func() {
		fakeClient.EXPECT().
			Get(ctx, k8stypes.NamespacedName{Name: "zone-a", Namespace: "default"}, &adminv1.Zone{}).
			Return(apierrors.NewNotFound(schema.GroupResource{Group: "admin.cp.ei.telekom.de", Resource: "zones"}, "zone-a"))

		result, err := util.GetZone(ctx, ref)
		Expect(err).To(HaveOccurred())
		Expect(result).To(BeNil())
		Expect(unwrapAll(err)).To(Satisfy(isBlockedError))
		Expect(err.Error()).To(ContainSubstring("not found"))
	})

	It("should return wrapped error on unexpected Get failure", func() {
		fakeClient.EXPECT().
			Get(ctx, k8stypes.NamespacedName{Name: "zone-a", Namespace: "default"}, &adminv1.Zone{}).
			Return(fmt.Errorf("connection refused"))

		result, err := util.GetZone(ctx, ref)
		Expect(err).To(HaveOccurred())
		Expect(result).To(BeNil())
		Expect(err.Error()).To(ContainSubstring("failed to get zone"))
		Expect(err.Error()).To(ContainSubstring("connection refused"))
	})

	It("should return BlockedError when zone is not ready", func() {
		notReady := &adminv1.Zone{
			ObjectMeta: metav1.ObjectMeta{Name: "zone-a", Namespace: "default"},
		}

		fakeClient.EXPECT().
			Get(ctx, k8stypes.NamespacedName{Name: "zone-a", Namespace: "default"}, &adminv1.Zone{}).
			Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
				*out.(*adminv1.Zone) = *notReady
			}).
			Return(nil)

		result, err := util.GetZone(ctx, ref)
		Expect(err).To(HaveOccurred())
		Expect(result).To(BeNil())
		Expect(unwrapAll(err)).To(Satisfy(isBlockedError))
		Expect(err.Error()).To(ContainSubstring("not ready"))
	})
})

// ---------- GetApplication ----------

var _ = Describe("GetApplication", func() {
	var (
		ctx        context.Context
		fakeClient *fakeclient.MockJanitorClient
		ref        ctypes.ObjectRef
	)

	BeforeEach(func() {
		ctx = context.Background()
		fakeClient = fakeclient.NewMockJanitorClient(GinkgoT())
		ctx = cclient.WithClient(ctx, fakeClient)
		ref = ctypes.ObjectRef{Name: "my-app", Namespace: "default"}
	})

	It("should return application when found and ready", func() {
		app := makeReadyApplication("my-app", "client-id-123")

		fakeClient.EXPECT().
			Get(ctx, k8stypes.NamespacedName{Name: "my-app", Namespace: "default"}, &applicationapi.Application{}).
			Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
				*out.(*applicationapi.Application) = *app
			}).
			Return(nil)

		result, err := util.GetApplication(ctx, ref)
		Expect(err).ToNot(HaveOccurred())
		Expect(result.Status.ClientId).To(Equal("client-id-123"))
	})

	It("should return BlockedError when application is not found", func() {
		fakeClient.EXPECT().
			Get(ctx, k8stypes.NamespacedName{Name: "my-app", Namespace: "default"}, &applicationapi.Application{}).
			Return(apierrors.NewNotFound(schema.GroupResource{Group: "application.cp.ei.telekom.de", Resource: "applications"}, "my-app"))

		result, err := util.GetApplication(ctx, ref)
		Expect(result).To(BeNil())
		Expect(unwrapAll(err)).To(Satisfy(isBlockedError))
		Expect(err.Error()).To(ContainSubstring("not found"))
	})

	It("should return BlockedError when application is not ready", func() {
		notReady := &applicationapi.Application{
			ObjectMeta: metav1.ObjectMeta{Name: "my-app", Namespace: "default"},
		}

		fakeClient.EXPECT().
			Get(ctx, k8stypes.NamespacedName{Name: "my-app", Namespace: "default"}, &applicationapi.Application{}).
			Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
				*out.(*applicationapi.Application) = *notReady
			}).
			Return(nil)

		result, err := util.GetApplication(ctx, ref)
		Expect(result).To(BeNil())
		Expect(unwrapAll(err)).To(Satisfy(isBlockedError))
		Expect(err.Error()).To(ContainSubstring("not ready"))
	})
})

// ---------- FindActiveMcpServer ----------

var _ = Describe("FindActiveMcpServer", func() {
	var (
		ctx        context.Context
		fakeClient *fakeclient.MockJanitorClient
	)

	BeforeEach(func() {
		ctx = context.Background()
		fakeClient = fakeclient.NewMockJanitorClient(GinkgoT())
		ctx = cclient.WithClient(ctx, fakeClient)
	})

	mockList := func(items []agenticv1.McpServer) {
		fakeClient.EXPECT().
			List(ctx, &agenticv1.McpServerList{}, mock.Anything).
			Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
				*list.(*agenticv1.McpServerList) = agenticv1.McpServerList{Items: items}
			}).
			Return(nil).Once()
	}

	It("should return false when no servers exist", func() {
		mockList([]agenticv1.McpServer{})

		found, server, err := util.FindActiveMcpServer(ctx, "/mcp/weather/v1")
		Expect(err).ToNot(HaveOccurred())
		Expect(found).To(BeFalse())
		Expect(server).To(BeNil())
	})

	It("should return error when List fails", func() {
		fakeClient.EXPECT().
			List(ctx, &agenticv1.McpServerList{}, mock.Anything).
			Return(fmt.Errorf("api unavailable")).Once()

		found, server, err := util.FindActiveMcpServer(ctx, "/mcp/weather/v1")
		Expect(err).To(HaveOccurred())
		Expect(found).To(BeFalse())
		Expect(server).To(BeNil())
		Expect(err.Error()).To(ContainSubstring("failed to list McpServers"))
	})

	It("should return false when no server is active", func() {
		inactive := agenticv1.McpServer{
			ObjectMeta: metav1.ObjectMeta{Name: "s1", Namespace: "default"},
			Spec:       agenticv1.McpServerSpec{BasePath: "/mcp/weather/v1"},
			Status:     agenticv1.McpServerStatus{Active: false},
		}
		mockList([]agenticv1.McpServer{inactive})

		found, server, err := util.FindActiveMcpServer(ctx, "/mcp/weather/v1")
		Expect(err).ToNot(HaveOccurred())
		Expect(found).To(BeFalse())
		Expect(server).To(BeNil())
	})

	It("should return the active server when found", func() {
		active := makeReadyMcpServer("/mcp/weather/v1")
		mockList([]agenticv1.McpServer{active})

		found, server, err := util.FindActiveMcpServer(ctx, "/mcp/weather/v1")
		Expect(err).ToNot(HaveOccurred())
		Expect(found).To(BeTrue())
		Expect(server).ToNot(BeNil())
		Expect(server.Spec.BasePath).To(Equal("/mcp/weather/v1"))
	})

	It("should ignore servers with a different basePath", func() {
		other := makeReadyMcpServer("/mcp/other/v1")
		mockList([]agenticv1.McpServer{other})

		found, _, err := util.FindActiveMcpServer(ctx, "/mcp/weather/v1")
		Expect(err).ToNot(HaveOccurred())
		Expect(found).To(BeFalse())
	})

	It("should return found=false and the conflicting server when a case-only mismatch exists", func() {
		// Server is registered as /Mcp/Weather/V1 but caller asks for /mcp/weather/v1
		conflict := makeReadyMcpServer("/Mcp/Weather/V1")
		mockList([]agenticv1.McpServer{conflict})

		found, server, err := util.FindActiveMcpServer(ctx, "/mcp/weather/v1")
		Expect(err).ToNot(HaveOccurred())
		Expect(found).To(BeFalse())
		Expect(server).ToNot(BeNil())
		Expect(server.Spec.BasePath).To(Equal("/Mcp/Weather/V1"))
	})

	It("should not treat case-only mismatches as a conflict when the server is inactive", func() {
		inactive := agenticv1.McpServer{
			ObjectMeta: metav1.ObjectMeta{Name: "s1", Namespace: "default"},
			Spec:       agenticv1.McpServerSpec{BasePath: "/Mcp/Weather/V1"},
			Status:     agenticv1.McpServerStatus{Active: false},
		}
		mockList([]agenticv1.McpServer{inactive})

		found, server, err := util.FindActiveMcpServer(ctx, "/mcp/weather/v1")
		Expect(err).ToNot(HaveOccurred())
		Expect(found).To(BeFalse())
		// inactive servers are ignored — no conflict returned
		Expect(server).To(BeNil())
	})

	It("should return BlockedError when active server is not ready", func() {
		notReady := agenticv1.McpServer{
			ObjectMeta: metav1.ObjectMeta{Name: "s1", Namespace: "default"},
			Spec:       agenticv1.McpServerSpec{BasePath: "/mcp/weather/v1"},
			Status:     agenticv1.McpServerStatus{Active: true},
			// no Ready condition set
		}
		mockList([]agenticv1.McpServer{notReady})

		found, server, err := util.FindActiveMcpServer(ctx, "/mcp/weather/v1")
		Expect(found).To(BeFalse())
		Expect(server).ToNot(BeNil())
		Expect(unwrapAll(err)).To(Satisfy(isBlockedError))
	})

	It("should return the oldest active server when multiple exist", func() {
		now := time.Now()
		older := makeReadyMcpServer("/mcp/weather/v1")
		older.Name = "s-oldest"
		older.CreationTimestamp = metav1.NewTime(now.Add(-time.Hour))

		newer := makeReadyMcpServer("/mcp/weather/v1")
		newer.Name = "s-newer"
		newer.CreationTimestamp = metav1.NewTime(now)

		mockList([]agenticv1.McpServer{newer, older})

		found, server, err := util.FindActiveMcpServer(ctx, "/mcp/weather/v1")
		Expect(err).ToNot(HaveOccurred())
		Expect(found).To(BeTrue())
		Expect(server.Name).To(Equal("s-oldest"))
	})
})

// ---------- FindActiveMcpExposure ----------

var _ = Describe("FindActiveMcpExposure", func() {
	It("should return false when list is empty", func() {
		found, exp, err := util.FindActiveMcpExposure([]agenticv1.McpExposure{})
		Expect(err).ToNot(HaveOccurred())
		Expect(found).To(BeFalse())
		Expect(exp).To(BeNil())
	})

	It("should return false when no exposure is active", func() {
		inactive := makeActiveMcpExposure("/mcp/weather/v1", "zone-a", "uid-1")
		inactive.Status.Active = false

		found, exp, err := util.FindActiveMcpExposure([]agenticv1.McpExposure{inactive})
		Expect(err).ToNot(HaveOccurred())
		Expect(found).To(BeFalse())
		Expect(exp).To(BeNil())
	})

	It("should return the single active exposure", func() {
		active := makeActiveMcpExposure("/mcp/weather/v1", "zone-a", "uid-1")

		found, exp, err := util.FindActiveMcpExposure([]agenticv1.McpExposure{active})
		Expect(err).ToNot(HaveOccurred())
		Expect(found).To(BeTrue())
		Expect(exp.UID).To(Equal(k8stypes.UID("uid-1")))
	})

	It("should return the oldest active exposure when multiple exist", func() {
		now := time.Now()
		older := makeActiveMcpExposure("/mcp/weather/v1", "zone-a", "uid-old")
		older.CreationTimestamp = metav1.NewTime(now.Add(-time.Hour))

		newer := makeActiveMcpExposure("/mcp/weather/v1", "zone-b", "uid-new")
		newer.CreationTimestamp = metav1.NewTime(now)

		found, exp, err := util.FindActiveMcpExposure([]agenticv1.McpExposure{newer, older})
		Expect(err).ToNot(HaveOccurred())
		Expect(found).To(BeTrue())
		Expect(exp.UID).To(Equal(k8stypes.UID("uid-old")))
	})
})

// ---------- AnyOtherMcpExposureExists ----------

var _ = Describe("AnyOtherMcpExposureExists", func() {
	var (
		ctx        context.Context
		fakeClient *fakeclient.MockJanitorClient
	)

	BeforeEach(func() {
		ctx = context.Background()
		fakeClient = fakeclient.NewMockJanitorClient(GinkgoT())
		ctx = cclient.WithClient(ctx, fakeClient)
	})

	mockList := func(items []agenticv1.McpExposure) {
		fakeClient.EXPECT().
			List(ctx, &agenticv1.McpExposureList{}, mock.Anything).
			Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
				*list.(*agenticv1.McpExposureList) = agenticv1.McpExposureList{Items: items}
			}).
			Return(nil).Once()
	}

	It("should return false when only the excluded exposure exists", func() {
		self := makeActiveMcpExposure("/mcp/weather/v1", "zone-a", "uid-self")
		mockList([]agenticv1.McpExposure{self})

		exists, err := util.AnyOtherMcpExposureExists(ctx, "/mcp/weather/v1", "uid-self")
		Expect(err).ToNot(HaveOccurred())
		Expect(exists).To(BeFalse())
	})

	It("should return true when another exposure exists", func() {
		self := makeActiveMcpExposure("/mcp/weather/v1", "zone-a", "uid-self")
		other := makeActiveMcpExposure("/mcp/weather/v1", "zone-b", "uid-other")
		mockList([]agenticv1.McpExposure{self, other})

		exists, err := util.AnyOtherMcpExposureExists(ctx, "/mcp/weather/v1", "uid-self")
		Expect(err).ToNot(HaveOccurred())
		Expect(exists).To(BeTrue())
	})

	It("should return false when no exposures exist", func() {
		mockList([]agenticv1.McpExposure{})

		exists, err := util.AnyOtherMcpExposureExists(ctx, "/mcp/weather/v1", "uid-self")
		Expect(err).ToNot(HaveOccurred())
		Expect(exists).To(BeFalse())
	})
})

// ---------- FindCrossZoneMcpSubscriptionZones ----------

var _ = Describe("FindCrossZoneMcpSubscriptionZones", func() {
	var (
		ctx        context.Context
		fakeClient *fakeclient.MockJanitorClient
	)

	BeforeEach(func() {
		ctx = context.Background()
		fakeClient = fakeclient.NewMockJanitorClient(GinkgoT())
		ctx = cclient.WithClient(ctx, fakeClient)
	})

	mockList := func(items []agenticv1.McpSubscription) {
		fakeClient.EXPECT().
			List(ctx, &agenticv1.McpSubscriptionList{}, mock.Anything).
			Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
				*list.(*agenticv1.McpSubscriptionList) = agenticv1.McpSubscriptionList{Items: items}
			}).
			Return(nil).Once()
	}

	approvedSub := func(name, basePath, zoneName string) agenticv1.McpSubscription {
		s := agenticv1.McpSubscription{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
			Spec: agenticv1.McpSubscriptionSpec{
				BasePath: basePath,
				Zone:     ctypes.ObjectRef{Name: zoneName, Namespace: "default"},
			},
		}
		setReady(&s.Status.Conditions)
		s.Status.Conditions = append(s.Status.Conditions, metav1.Condition{
			Type:   "ApprovalGranted",
			Status: metav1.ConditionTrue,
			Reason: "Approved",
		})
		return s
	}

	unapprovedSub := func(name, basePath, zoneName string) agenticv1.McpSubscription {
		return agenticv1.McpSubscription{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
			Spec: agenticv1.McpSubscriptionSpec{
				BasePath: basePath,
				Zone:     ctypes.ObjectRef{Name: zoneName, Namespace: "default"},
			},
		}
	}

	It("should return error when List fails", func() {
		fakeClient.EXPECT().
			List(ctx, &agenticv1.McpSubscriptionList{}, mock.Anything).
			Return(fmt.Errorf("api error")).Once()

		zones, err := util.FindCrossZoneMcpSubscriptionZones(ctx, "/mcp/weather/v1", "zone-provider")
		Expect(err).To(HaveOccurred())
		Expect(zones).To(BeNil())
		Expect(err.Error()).To(ContainSubstring("failed to list McpSubscriptions"))
	})

	It("should return empty when no subscriptions exist", func() {
		mockList([]agenticv1.McpSubscription{})

		zones, err := util.FindCrossZoneMcpSubscriptionZones(ctx, "/mcp/weather/v1", "zone-provider")
		Expect(err).ToNot(HaveOccurred())
		Expect(zones).To(BeEmpty())
	})

	It("should exclude same-zone subscriptions", func() {
		sameZone := approvedSub("sub-same", "/mcp/weather/v1", "zone-provider")
		mockList([]agenticv1.McpSubscription{sameZone})

		zones, err := util.FindCrossZoneMcpSubscriptionZones(ctx, "/mcp/weather/v1", "zone-provider")
		Expect(err).ToNot(HaveOccurred())
		Expect(zones).To(BeEmpty())
	})

	It("should exclude unapproved cross-zone subscriptions", func() {
		sub := unapprovedSub("sub-pending", "/mcp/weather/v1", "zone-subscriber")
		mockList([]agenticv1.McpSubscription{sub})

		zones, err := util.FindCrossZoneMcpSubscriptionZones(ctx, "/mcp/weather/v1", "zone-provider")
		Expect(err).ToNot(HaveOccurred())
		Expect(zones).To(BeEmpty())
	})

	It("should return zone for approved cross-zone subscription", func() {
		sub := approvedSub("sub-1", "/mcp/weather/v1", "zone-subscriber")
		mockList([]agenticv1.McpSubscription{sub})

		zones, err := util.FindCrossZoneMcpSubscriptionZones(ctx, "/mcp/weather/v1", "zone-provider")
		Expect(err).ToNot(HaveOccurred())
		Expect(zones).To(HaveLen(1))
		Expect(zones[0].Name).To(Equal("zone-subscriber"))
	})

	It("should deduplicate zones when multiple subscriptions come from the same zone", func() {
		sub1 := approvedSub("sub-1", "/mcp/weather/v1", "zone-subscriber")
		sub2 := approvedSub("sub-2", "/mcp/weather/v1", "zone-subscriber")
		mockList([]agenticv1.McpSubscription{sub1, sub2})

		zones, err := util.FindCrossZoneMcpSubscriptionZones(ctx, "/mcp/weather/v1", "zone-provider")
		Expect(err).ToNot(HaveOccurred())
		Expect(zones).To(HaveLen(1))
		Expect(zones[0].Name).To(Equal("zone-subscriber"))
	})

	It("should return multiple unique zones for approved subscriptions", func() {
		sub1 := approvedSub("sub-1", "/mcp/weather/v1", "zone-a")
		sub2 := approvedSub("sub-2", "/mcp/weather/v1", "zone-b")
		mockList([]agenticv1.McpSubscription{sub1, sub2})

		zones, err := util.FindCrossZoneMcpSubscriptionZones(ctx, "/mcp/weather/v1", "zone-provider")
		Expect(err).ToNot(HaveOccurred())
		Expect(zones).To(HaveLen(2))
		Expect([]string{zones[0].Name, zones[1].Name}).To(ConsistOf("zone-a", "zone-b"))
	})

	It("should exclude subscriptions for a different basePath", func() {
		otherPath := approvedSub("sub-other", "/mcp/other/v1", "zone-subscriber")
		mockList([]agenticv1.McpSubscription{otherPath})

		zones, err := util.FindCrossZoneMcpSubscriptionZones(ctx, "/mcp/weather/v1", "zone-provider")
		Expect(err).ToNot(HaveOccurred())
		Expect(zones).To(BeEmpty())
	})

	It("should skip subscriptions being deleted", func() {
		now := metav1.Now()
		deleting := approvedSub("sub-deleting", "/mcp/weather/v1", "zone-subscriber")
		deleting.DeletionTimestamp = &now
		deleting.Finalizers = []string{"some-finalizer"}
		mockList([]agenticv1.McpSubscription{deleting})

		zones, err := util.FindCrossZoneMcpSubscriptionZones(ctx, "/mcp/weather/v1", "zone-provider")
		Expect(err).ToNot(HaveOccurred())
		Expect(zones).To(BeEmpty())
	})
})
