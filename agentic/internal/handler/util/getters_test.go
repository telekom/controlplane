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

// ---------- MakeAgenticRouteName ----------

var _ = Describe("MakeAgenticRouteName", func() {
	DescribeTable("should produce correct route names",
		func(basePath, expected string) {
			Expect(util.MakeAgenticRouteName(basePath)).To(Equal(expected))
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

// ---------- FindActiveAgenticServer ----------

var _ = Describe("FindActiveAgenticServer", func() {
	var (
		ctx        context.Context
		fakeClient *fakeclient.MockJanitorClient
	)

	BeforeEach(func() {
		ctx = context.Background()
		fakeClient = fakeclient.NewMockJanitorClient(GinkgoT())
		ctx = cclient.WithClient(ctx, fakeClient)
	})

	mockList := func(items []agenticv1.AgenticServer) {
		fakeClient.EXPECT().
			List(ctx, &agenticv1.AgenticServerList{}, mock.Anything).
			Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
				*list.(*agenticv1.AgenticServerList) = agenticv1.AgenticServerList{Items: items}
			}).
			Return(nil).Once()
	}

	It("should return false when no servers exist", func() {
		mockList([]agenticv1.AgenticServer{})

		found, server, err := util.FindActiveAgenticServer(ctx, "/mcp/weather/v1")
		Expect(err).ToNot(HaveOccurred())
		Expect(found).To(BeFalse())
		Expect(server).To(BeNil())
	})

	It("should return error when List fails", func() {
		fakeClient.EXPECT().
			List(ctx, &agenticv1.AgenticServerList{}, mock.Anything).
			Return(fmt.Errorf("api unavailable")).Once()

		found, server, err := util.FindActiveAgenticServer(ctx, "/mcp/weather/v1")
		Expect(err).To(HaveOccurred())
		Expect(found).To(BeFalse())
		Expect(server).To(BeNil())
		Expect(err.Error()).To(ContainSubstring("failed to list AgenticServers"))
	})

	It("should return false when no server is active", func() {
		inactive := agenticv1.AgenticServer{
			ObjectMeta: metav1.ObjectMeta{Name: "s1", Namespace: "default"},
			Spec:       agenticv1.AgenticServerSpec{BasePath: "/mcp/weather/v1"},
			Status:     agenticv1.AgenticServerStatus{Active: false},
		}
		mockList([]agenticv1.AgenticServer{inactive})

		found, server, err := util.FindActiveAgenticServer(ctx, "/mcp/weather/v1")
		Expect(err).ToNot(HaveOccurred())
		Expect(found).To(BeFalse())
		Expect(server).To(BeNil())
	})

	It("should return the active server when found", func() {
		active := makeReadyAgenticServer("/mcp/weather/v1")
		mockList([]agenticv1.AgenticServer{active})

		found, server, err := util.FindActiveAgenticServer(ctx, "/mcp/weather/v1")
		Expect(err).ToNot(HaveOccurred())
		Expect(found).To(BeTrue())
		Expect(server).ToNot(BeNil())
		Expect(server.Spec.BasePath).To(Equal("/mcp/weather/v1"))
	})

	It("should ignore servers with a different basePath", func() {
		other := makeReadyAgenticServer("/mcp/other/v1")
		mockList([]agenticv1.AgenticServer{other})

		found, _, err := util.FindActiveAgenticServer(ctx, "/mcp/weather/v1")
		Expect(err).ToNot(HaveOccurred())
		Expect(found).To(BeFalse())
	})

	It("should return found=false and the conflicting server when a case-only mismatch exists", func() {
		// Server is registered as /Mcp/Weather/V1 but caller asks for /mcp/weather/v1
		conflict := makeReadyAgenticServer("/Mcp/Weather/V1")
		mockList([]agenticv1.AgenticServer{conflict})

		found, server, err := util.FindActiveAgenticServer(ctx, "/mcp/weather/v1")
		Expect(err).ToNot(HaveOccurred())
		Expect(found).To(BeFalse())
		Expect(server).ToNot(BeNil())
		Expect(server.Spec.BasePath).To(Equal("/Mcp/Weather/V1"))
	})

	It("should not treat case-only mismatches as a conflict when the server is inactive", func() {
		inactive := agenticv1.AgenticServer{
			ObjectMeta: metav1.ObjectMeta{Name: "s1", Namespace: "default"},
			Spec:       agenticv1.AgenticServerSpec{BasePath: "/Mcp/Weather/V1"},
			Status:     agenticv1.AgenticServerStatus{Active: false},
		}
		mockList([]agenticv1.AgenticServer{inactive})

		found, server, err := util.FindActiveAgenticServer(ctx, "/mcp/weather/v1")
		Expect(err).ToNot(HaveOccurred())
		Expect(found).To(BeFalse())
		// inactive servers are ignored — no conflict returned
		Expect(server).To(BeNil())
	})

	It("should return BlockedError when active server is not ready", func() {
		notReady := agenticv1.AgenticServer{
			ObjectMeta: metav1.ObjectMeta{Name: "s1", Namespace: "default"},
			Spec:       agenticv1.AgenticServerSpec{BasePath: "/mcp/weather/v1"},
			Status:     agenticv1.AgenticServerStatus{Active: true},
			// no Ready condition set
		}
		mockList([]agenticv1.AgenticServer{notReady})

		found, server, err := util.FindActiveAgenticServer(ctx, "/mcp/weather/v1")
		Expect(found).To(BeFalse())
		Expect(server).ToNot(BeNil())
		Expect(unwrapAll(err)).To(Satisfy(isBlockedError))
	})

	It("should return the oldest active server when multiple exist", func() {
		now := time.Now()
		older := makeReadyAgenticServer("/mcp/weather/v1")
		older.Name = "s-oldest"
		older.CreationTimestamp = metav1.NewTime(now.Add(-time.Hour))

		newer := makeReadyAgenticServer("/mcp/weather/v1")
		newer.Name = "s-newer"
		newer.CreationTimestamp = metav1.NewTime(now)

		mockList([]agenticv1.AgenticServer{newer, older})

		found, server, err := util.FindActiveAgenticServer(ctx, "/mcp/weather/v1")
		Expect(err).ToNot(HaveOccurred())
		Expect(found).To(BeTrue())
		Expect(server.Name).To(Equal("s-oldest"))
	})
})

// ---------- FindActiveAgenticExposure ----------

var _ = Describe("FindActiveAgenticExposure", func() {
	It("should return false when list is empty", func() {
		found, exp, err := util.FindActiveAgenticExposure([]agenticv1.AgenticExposure{})
		Expect(err).ToNot(HaveOccurred())
		Expect(found).To(BeFalse())
		Expect(exp).To(BeNil())
	})

	It("should return false when no exposure is active", func() {
		inactive := makeActiveAgenticExposure("/mcp/weather/v1", "zone-a", "uid-1")
		inactive.Status.Active = false

		found, exp, err := util.FindActiveAgenticExposure([]agenticv1.AgenticExposure{inactive})
		Expect(err).ToNot(HaveOccurred())
		Expect(found).To(BeFalse())
		Expect(exp).To(BeNil())
	})

	It("should return the single active exposure", func() {
		active := makeActiveAgenticExposure("/mcp/weather/v1", "zone-a", "uid-1")

		found, exp, err := util.FindActiveAgenticExposure([]agenticv1.AgenticExposure{active})
		Expect(err).ToNot(HaveOccurred())
		Expect(found).To(BeTrue())
		Expect(exp.UID).To(Equal(k8stypes.UID("uid-1")))
	})

	It("should return the oldest active exposure when multiple exist", func() {
		now := time.Now()
		older := makeActiveAgenticExposure("/mcp/weather/v1", "zone-a", "uid-old")
		older.CreationTimestamp = metav1.NewTime(now.Add(-time.Hour))

		newer := makeActiveAgenticExposure("/mcp/weather/v1", "zone-b", "uid-new")
		newer.CreationTimestamp = metav1.NewTime(now)

		found, exp, err := util.FindActiveAgenticExposure([]agenticv1.AgenticExposure{newer, older})
		Expect(err).ToNot(HaveOccurred())
		Expect(found).To(BeTrue())
		Expect(exp.UID).To(Equal(k8stypes.UID("uid-old")))
	})
})

// ---------- AnyOtherAgenticExposureExists ----------

var _ = Describe("AnyOtherAgenticExposureExists", func() {
	var (
		ctx        context.Context
		fakeClient *fakeclient.MockJanitorClient
	)

	BeforeEach(func() {
		ctx = context.Background()
		fakeClient = fakeclient.NewMockJanitorClient(GinkgoT())
		ctx = cclient.WithClient(ctx, fakeClient)
	})

	mockList := func(items []agenticv1.AgenticExposure) {
		fakeClient.EXPECT().
			List(ctx, &agenticv1.AgenticExposureList{}, mock.Anything).
			Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
				*list.(*agenticv1.AgenticExposureList) = agenticv1.AgenticExposureList{Items: items}
			}).
			Return(nil).Once()
	}

	It("should return false when only the excluded exposure exists", func() {
		self := makeActiveAgenticExposure("/mcp/weather/v1", "zone-a", "uid-self")
		mockList([]agenticv1.AgenticExposure{self})

		exists, err := util.AnyOtherAgenticExposureExists(ctx, "/mcp/weather/v1", "uid-self")
		Expect(err).ToNot(HaveOccurred())
		Expect(exists).To(BeFalse())
	})

	It("should return true when another exposure exists", func() {
		self := makeActiveAgenticExposure("/mcp/weather/v1", "zone-a", "uid-self")
		other := makeActiveAgenticExposure("/mcp/weather/v1", "zone-b", "uid-other")
		mockList([]agenticv1.AgenticExposure{self, other})

		exists, err := util.AnyOtherAgenticExposureExists(ctx, "/mcp/weather/v1", "uid-self")
		Expect(err).ToNot(HaveOccurred())
		Expect(exists).To(BeTrue())
	})

	It("should return false when no exposures exist", func() {
		mockList([]agenticv1.AgenticExposure{})

		exists, err := util.AnyOtherAgenticExposureExists(ctx, "/mcp/weather/v1", "uid-self")
		Expect(err).ToNot(HaveOccurred())
		Expect(exists).To(BeFalse())
	})
})

// ---------- FindCrossZoneAgenticSubscriptionZones ----------

var _ = Describe("FindCrossZoneAgenticSubscriptionZones", func() {
	var (
		ctx        context.Context
		fakeClient *fakeclient.MockJanitorClient
	)

	BeforeEach(func() {
		ctx = context.Background()
		fakeClient = fakeclient.NewMockJanitorClient(GinkgoT())
		ctx = cclient.WithClient(ctx, fakeClient)
	})

	mockList := func(items []agenticv1.AgenticSubscription) {
		fakeClient.EXPECT().
			List(ctx, &agenticv1.AgenticSubscriptionList{}, mock.Anything).
			Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
				*list.(*agenticv1.AgenticSubscriptionList) = agenticv1.AgenticSubscriptionList{Items: items}
			}).
			Return(nil).Once()
	}

	approvedSub := func(name, basePath, zoneName string) agenticv1.AgenticSubscription {
		s := agenticv1.AgenticSubscription{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
			Spec: agenticv1.AgenticSubscriptionSpec{
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

	unapprovedSub := func(name, basePath, zoneName string) agenticv1.AgenticSubscription {
		return agenticv1.AgenticSubscription{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
			Spec: agenticv1.AgenticSubscriptionSpec{
				BasePath: basePath,
				Zone:     ctypes.ObjectRef{Name: zoneName, Namespace: "default"},
			},
		}
	}

	It("should return error when List fails", func() {
		fakeClient.EXPECT().
			List(ctx, &agenticv1.AgenticSubscriptionList{}, mock.Anything).
			Return(fmt.Errorf("api error")).Once()

		zones, _, err := util.FindCrossZoneAgenticSubscriptionZones(ctx, "/mcp/weather/v1", "zone-provider")
		Expect(err).To(HaveOccurred())
		Expect(zones).To(BeNil())
		Expect(err.Error()).To(ContainSubstring("failed to list AgenticSubscriptions"))
	})

	It("should return empty when no subscriptions exist", func() {
		mockList([]agenticv1.AgenticSubscription{})

		zones, _, err := util.FindCrossZoneAgenticSubscriptionZones(ctx, "/mcp/weather/v1", "zone-provider")
		Expect(err).ToNot(HaveOccurred())
		Expect(zones).To(BeEmpty())
	})

	It("should exclude same-zone subscriptions", func() {
		sameZone := approvedSub("sub-same", "/mcp/weather/v1", "zone-provider")
		mockList([]agenticv1.AgenticSubscription{sameZone})

		zones, _, err := util.FindCrossZoneAgenticSubscriptionZones(ctx, "/mcp/weather/v1", "zone-provider")
		Expect(err).ToNot(HaveOccurred())
		Expect(zones).To(BeEmpty())
	})

	It("should exclude unapproved cross-zone subscriptions", func() {
		sub := unapprovedSub("sub-pending", "/mcp/weather/v1", "zone-subscriber")
		mockList([]agenticv1.AgenticSubscription{sub})

		zones, _, err := util.FindCrossZoneAgenticSubscriptionZones(ctx, "/mcp/weather/v1", "zone-provider")
		Expect(err).ToNot(HaveOccurred())
		Expect(zones).To(BeEmpty())
	})

	It("should return zone for approved cross-zone subscription", func() {
		sub := approvedSub("sub-1", "/mcp/weather/v1", "zone-subscriber")
		mockList([]agenticv1.AgenticSubscription{sub})

		zones, _, err := util.FindCrossZoneAgenticSubscriptionZones(ctx, "/mcp/weather/v1", "zone-provider")
		Expect(err).ToNot(HaveOccurred())
		Expect(zones).To(HaveLen(1))
		Expect(zones[0].Name).To(Equal("zone-subscriber"))
	})

	It("should deduplicate zones when multiple subscriptions come from the same zone", func() {
		sub1 := approvedSub("sub-1", "/mcp/weather/v1", "zone-subscriber")
		sub2 := approvedSub("sub-2", "/mcp/weather/v1", "zone-subscriber")
		mockList([]agenticv1.AgenticSubscription{sub1, sub2})

		zones, _, err := util.FindCrossZoneAgenticSubscriptionZones(ctx, "/mcp/weather/v1", "zone-provider")
		Expect(err).ToNot(HaveOccurred())
		Expect(zones).To(HaveLen(1))
		Expect(zones[0].Name).To(Equal("zone-subscriber"))
	})

	It("should return multiple unique zones for approved subscriptions", func() {
		sub1 := approvedSub("sub-1", "/mcp/weather/v1", "zone-a")
		sub2 := approvedSub("sub-2", "/mcp/weather/v1", "zone-b")
		mockList([]agenticv1.AgenticSubscription{sub1, sub2})

		zones, _, err := util.FindCrossZoneAgenticSubscriptionZones(ctx, "/mcp/weather/v1", "zone-provider")
		Expect(err).ToNot(HaveOccurred())
		Expect(zones).To(HaveLen(2))
		Expect([]string{zones[0].Name, zones[1].Name}).To(ConsistOf("zone-a", "zone-b"))
	})

	It("should exclude subscriptions for a different basePath", func() {
		otherPath := approvedSub("sub-other", "/mcp/other/v1", "zone-subscriber")
		mockList([]agenticv1.AgenticSubscription{otherPath})

		zones, _, err := util.FindCrossZoneAgenticSubscriptionZones(ctx, "/mcp/weather/v1", "zone-provider")
		Expect(err).ToNot(HaveOccurred())
		Expect(zones).To(BeEmpty())
	})

	It("should skip subscriptions being deleted", func() {
		now := metav1.Now()
		deleting := approvedSub("sub-deleting", "/mcp/weather/v1", "zone-subscriber")
		deleting.DeletionTimestamp = &now
		deleting.Finalizers = []string{"some-finalizer"}
		mockList([]agenticv1.AgenticSubscription{deleting})

		zones, _, err := util.FindCrossZoneAgenticSubscriptionZones(ctx, "/mcp/weather/v1", "zone-provider")
		Expect(err).ToNot(HaveOccurred())
		Expect(zones).To(BeEmpty())
	})

	It("should return hasLocalSubs=true when an approved same-zone subscription exists", func() {
		sameZone := approvedSub("sub-local", "/mcp/weather/v1", "zone-provider")
		crossZone := approvedSub("sub-cross", "/mcp/weather/v1", "zone-subscriber")
		mockList([]agenticv1.AgenticSubscription{sameZone, crossZone})

		zones, hasLocalSubs, err := util.FindCrossZoneAgenticSubscriptionZones(ctx, "/mcp/weather/v1", "zone-provider")
		Expect(err).ToNot(HaveOccurred())
		Expect(hasLocalSubs).To(BeTrue())
		// same-zone sub not included in cross-zone zones list
		Expect(zones).To(HaveLen(1))
		Expect(zones[0].Name).To(Equal("zone-subscriber"))
	})

	It("should return hasLocalSubs=false when only cross-zone subscriptions exist", func() {
		crossZone := approvedSub("sub-cross", "/mcp/weather/v1", "zone-subscriber")
		mockList([]agenticv1.AgenticSubscription{crossZone})

		_, hasLocalSubs, err := util.FindCrossZoneAgenticSubscriptionZones(ctx, "/mcp/weather/v1", "zone-provider")
		Expect(err).ToNot(HaveOccurred())
		Expect(hasLocalSubs).To(BeFalse())
	})
})
