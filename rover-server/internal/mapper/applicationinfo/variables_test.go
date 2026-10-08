// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package applicationinfo

import (
	"encoding/json"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/mock"
	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	agenticv1 "github.com/telekom/controlplane/agentic/api/v1"
	apiv1 "github.com/telekom/controlplane/api/api/v1"
	applicationv1 "github.com/telekom/controlplane/application/api/v1"
	"github.com/telekom/controlplane/common/pkg/types"
	eventv1 "github.com/telekom/controlplane/event/api/v1"
	"github.com/telekom/controlplane/rover-server/internal/api"
	"github.com/telekom/controlplane/rover-server/pkg/store"
	"github.com/telekom/controlplane/rover-server/test/mocks"
	roverv1 "github.com/telekom/controlplane/rover/api/v1"
)

var _ = Describe("ApplicationInfo variables", func() {
	It("maps application values with legacy names and UTC secret expiration", func() {
		info := &api.ApplicationInfo{
			IrisClientId:         "test-client",
			IrisIssuerUrl:        "https://issuer.example.com",
			IrisTokenEndpointUrl: "https://issuer.example.com/custom-token",
			StargateUrl:          "https://failover.example.com",
			SecretInfo: api.SecretInfo{
				ClientSecret:     "test-secret",
				CurrentExpiresAt: time.Date(2027, 9, 17, 10, 1, 23, 0, time.FixedZone("offset", 2*60*60)),
			},
		}
		fillApplicationVariables(info)
		Expect(info.Variables).To(Equal([]api.Data{
			{Name: "tardis.iris.client.id", Value: "test-client"},
			{Name: "tardis.iris.client.secret", Value: "test-secret"},
			{Name: "tardis.iris.client.secret.expiration", Value: "2027-09-17T08:01:23Z"},
			{Name: "tardis.iris.url.issuer", Value: "https://issuer.example.com"},
			{Name: "tardis.iris.url.token", Value: "https://issuer.example.com/custom-token"},
			{Name: "tardis.stargate.url", Value: "https://failover.example.com"},
		}))
	})

	It("omits absent values without a zero expiration", func() {
		info := &api.ApplicationInfo{Variables: []api.Data{}}
		fillApplicationVariables(info)
		Expect(info.Variables).To(BeEmpty())
		raw, err := json.Marshal(info)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(raw)).To(ContainSubstring(`"variables":[]`))
	})

	It("returns an empty variables array for empty application and zone status values", func() {
		appStore := mocks.NewMockObjectStore[*applicationv1.Application](GinkgoT())
		appStore.EXPECT().Get(mock.Anything, mock.Anything, mock.Anything).Return(&applicationv1.Application{}, nil).Once()
		zoneStore := mocks.NewMockObjectStore[*adminv1.Zone](GinkgoT())
		zoneStore.EXPECT().Get(mock.Anything, mock.Anything, mock.Anything).Return(&adminv1.Zone{
			Spec: adminv1.ZoneSpec{Presets: []adminv1.Preset{{Name: "default", Type: adminv1.GatewayTypeAPI, Default: true}}},
			Status: adminv1.ZoneStatus{Presets: []adminv1.PresetStatus{{
				Name: "default",
			}}},
		}, nil).Once()
		resource := &roverv1.Rover{Status: roverv1.RoverStatus{Application: rover.Status.Application}}
		info, err := MapApplicationInfo(ctx, resource, &store.Stores{
			ApplicationSecretStore: appStore, ZoneStore: zoneStore,
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(info.Variables).NotTo(BeNil())
		Expect(info.Variables).To(BeEmpty())
		Expect(info.IrisTokenEndpointUrl).To(BeEmpty())
		raw, err := json.Marshal(info)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(raw)).To(ContainSubstring(`"variables":[]`))
	})

	DescribeTable("uses resolved subscription endpoints and omits unavailable values",
		func(endpoint, subscriptionID, sseURL string, delivery eventv1.DeliveryType, expected []api.Data) {
			apiStore := mocks.NewMockObjectStore[*apiv1.ApiSubscription](GinkgoT())
			apiStore.EXPECT().Get(mock.Anything, "test", "api").Return(&apiv1.ApiSubscription{
				Spec:   apiv1.ApiSubscriptionSpec{ApiBasePath: "/fs/agreementManagement/v3"},
				Status: apiv1.ApiSubscriptionStatus{GatewayUrl: endpoint},
			}, nil).Once()
			aiStore := mocks.NewMockObjectStore[*agenticv1.AgenticSubscription](GinkgoT())
			aiStore.EXPECT().Get(mock.Anything, "test", "ai").Return(&agenticv1.AgenticSubscription{
				Spec:   agenticv1.AgenticSubscriptionSpec{BasePath: "/mcp/Weather/v1"},
				Status: agenticv1.AgenticSubscriptionStatus{GatewayUrl: endpoint},
			}, nil).Once()
			eventStore := mocks.NewMockObjectStore[*eventv1.EventSubscription](GinkgoT())
			eventStore.EXPECT().Get(mock.Anything, "test", "event").Return(&eventv1.EventSubscription{
				Spec: eventv1.EventSubscriptionSpec{
					EventType: "test.events.v1",
					Delivery:  eventv1.Delivery{Type: delivery},
				},
				Status: eventv1.EventSubscriptionStatus{SubscriptionId: subscriptionID, URL: sseURL},
			}, nil).Once()
			localStores := &store.Stores{
				APISubscriptionStore: apiStore, AgenticSubscriptionStore: aiStore, EventSubscriptionStore: eventStore,
			}
			resource := &roverv1.Rover{Status: roverv1.RoverStatus{
				ApiSubscriptions:     []types.ObjectRef{{Namespace: "test", Name: "api"}},
				AgenticSubscriptions: []types.ObjectRef{{Namespace: "test", Name: "ai"}},
				EventSubscriptions:   []types.ObjectRef{{Namespace: "test", Name: "event"}},
			}}
			info := &api.ApplicationInfo{Variables: []api.Data{}}
			Expect(FillSubscriptionInfo(ctx, resource, info, localStores)).To(Succeed())
			Expect(info.Variables).To(Equal(expected))
		},
		Entry("resolved gateway includes the complete path and preserves case",
			"https://failover.example.com/resolved/path", "subscriber-id", "https://sse.example.com/events", eventv1.DeliveryTypeServerSentEvent,
			[]api.Data{
				{Name: "tardis.stargate.url.api.fs.agreementManagement.v3", Value: "https://failover.example.com/resolved/path"},
				{Name: "tardis.horizon.subscription.id.test.events.v1", Value: "subscriber-id"},
				{Name: "tardis.horizon.subscription.url.test.events.v1", Value: "https://sse.example.com/events"},
				{Name: "tardis.stargate.url.api.mcp.Weather.v1", Value: "https://failover.example.com/resolved/path"},
			}),
		Entry("pending resources have no variables", "", "", "", eventv1.DeliveryTypeServerSentEvent, []api.Data{}),
		Entry("callback delivery does not advertise a stale SSE URL", "", "subscriber-id", "https://sse.example.com/stale", eventv1.DeliveryTypeCallback,
			[]api.Data{{Name: "tardis.horizon.subscription.id.test.events.v1", Value: "subscriber-id"}}),
	)

	It("includes application, subscription, and event publish variables in the final response", func() {
		eventStore := mocks.NewMockObjectStore[*eventv1.EventExposure](GinkgoT())
		eventStore.EXPECT().Get(mock.Anything, "test", "event").Return(&eventv1.EventExposure{
			Status: eventv1.EventExposureStatus{PublishURL: "https://events.example.com/publish"},
		}, nil).Once()
		localStores := *stores
		localStores.EventExposureStore = eventStore
		resource := rover.DeepCopy()
		resource.Status.EventExposures = []types.ObjectRef{{Namespace: "test", Name: "event"}}
		info, err := MapApplicationInfo(ctx, resource, &localStores)
		Expect(err).NotTo(HaveOccurred())
		Expect(info.Variables).To(ContainElements(
			api.Data{Name: "tardis.iris.client.id", Value: info.IrisClientId},
			api.Data{Name: "tardis.iris.client.secret", Value: info.SecretInfo.ClientSecret},
			api.Data{Name: "tardis.stargate.url", Value: info.StargateUrl},
			api.Data{Name: "tardis.horizon.event.url", Value: "https://events.example.com/publish"},
			api.Data{Name: "tardis.horizon.subscription.id.de.telekom.eni.quickstart.v1", Value: "horizon-sub-456"},
		))
		for _, variable := range info.Variables {
			Expect(variable.Value).NotTo(BeEmpty())
		}
	})
})
