// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package applicationinfo

import (
	"fmt"

	"github.com/gkampitakis/go-snaps/snaps"
	"github.com/stretchr/testify/mock"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	apiv1 "github.com/telekom/controlplane/api/api/v1"
	applicationv1 "github.com/telekom/controlplane/application/api/v1"
	"github.com/telekom/controlplane/common-server/pkg/server/middleware/security"
	"github.com/telekom/controlplane/common/pkg/types"
	eventv1 "github.com/telekom/controlplane/event/api/v1"
	roverv1 "github.com/telekom/controlplane/rover/api/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/telekom/controlplane/rover-server/internal/api"
	"github.com/telekom/controlplane/rover-server/pkg/store"
	"github.com/telekom/controlplane/rover-server/test/mocks"
)

var _ = Describe("ApplicationInfo Mapper", func() {
	Context("FillExposureInfo", func() {
		It("must fill exposure info correctly", func() {
			var applicationInfo = &api.ApplicationInfo{}
			err := FillExposureInfo(ctx, rover, applicationInfo, stores)

			Expect(err).To(BeNil())

			snaps.MatchJSON(GinkgoT(), applicationInfo)
		})

		It("must return an error if the input rover is nil", func() {
			var applicationInfo = &api.ApplicationInfo{}
			err := FillExposureInfo(ctx, nil, applicationInfo, stores)

			Expect(err).ToNot(BeNil())
			Expect(err.Error()).To(ContainSubstring("input rover is nil"))
		})

		It("must return an error if the input application info is nil", func() {
			err := FillExposureInfo(ctx, rover, nil, stores)

			Expect(err).ToNot(BeNil())
			Expect(err.Error()).To(ContainSubstring("input applicationInfo is nil"))
		})

		It("must record error when APIExposureStore.Get fails", func() {
			errStores := &store.Stores{}
			apiExpMock := mocks.NewMockObjectStore[*apiv1.ApiExposure](GinkgoT())
			failedExp := &apiv1.ApiExposure{
				TypeMeta:   metav1.TypeMeta{APIVersion: "api/v1", Kind: "ApiExposure"},
				ObjectMeta: metav1.ObjectMeta{Name: "failed-exp", Namespace: "test-ns"},
				Status: apiv1.ApiExposureStatus{
					Conditions: []metav1.Condition{
						{Type: "Ready", Status: metav1.ConditionFalse, Reason: "Error", Message: "store error cause"},
					},
				},
			}
			apiExpMock.EXPECT().Get(mock.Anything, mock.Anything, mock.Anything).Return(failedExp, fmt.Errorf("store error")).Maybe()
			errStores.APIExposureStore = apiExpMock

			eventExpMock := mocks.NewMockObjectStore[*eventv1.EventExposure](GinkgoT())
			eventExpMock.EXPECT().Get(mock.Anything, mock.Anything, mock.Anything).Return(nil, nil).Maybe()
			errStores.EventExposureStore = eventExpMock

			roverWithExp := rover.DeepCopy()
			roverWithExp.Status.ApiExposures = []types.ObjectRef{
				{Name: "some-exp", Namespace: "test-ns"},
			}
			roverWithExp.Status.EventExposures = nil

			appInfo := &api.ApplicationInfo{}
			err := FillExposureInfo(ctx, roverWithExp, appInfo, errStores)

			Expect(err).To(BeNil())
			Expect(appInfo.Errors).To(HaveLen(1))
			Expect(appInfo.Errors[0].Message).To(Equal("store error"))
		})

		It("must handle event exposures with PublishEventUrl", func() {
			secCtx := security.ToContext(ctx, &security.BusinessContext{Environment: "poc"})

			localStores := &store.Stores{}

			// API exposure mock (empty - no API exposures in this test)
			apiExpMock := mocks.NewMockObjectStore[*apiv1.ApiExposure](GinkgoT())
			apiExpMock.EXPECT().Get(mock.Anything, mock.Anything, mock.Anything).Return(nil, nil).Maybe()
			localStores.APIExposureStore = apiExpMock

			// Event exposure mock returning a ready event exposure
			readyEventExp := &eventv1.EventExposure{
				TypeMeta:   metav1.TypeMeta{APIVersion: "event.2.2/v1", Kind: "EventExposure"},
				ObjectMeta: metav1.ObjectMeta{Name: "event-exp-1", Namespace: "test-ns"},
				Spec: eventv1.EventExposureSpec{
					EventType:  "de.telekom.test.event.v1",
					Visibility: eventv1.VisibilityWorld,
					Approval: eventv1.Approval{
						Strategy: eventv1.ApprovalStrategyAuto,
					},
				},
				Status: eventv1.EventExposureStatus{
					Conditions: []metav1.Condition{
						{Type: "Ready", Status: metav1.ConditionTrue, Reason: "Ready"},
					},
				},
			}
			eventExpMock := mocks.NewMockObjectStore[*eventv1.EventExposure](GinkgoT())
			eventExpMock.EXPECT().Get(mock.Anything, mock.Anything, mock.Anything).Return(readyEventExp, nil).Maybe()
			localStores.EventExposureStore = eventExpMock

			// Zone mock
			zone := &adminv1.Zone{
				Status: adminv1.ZoneStatus{
					Namespace: "zone-ns",
				},
			}
			zoneMock := mocks.NewMockObjectStore[*adminv1.Zone](GinkgoT())
			zoneMock.EXPECT().Get(mock.Anything, mock.Anything, mock.Anything).Return(zone, nil).Maybe()
			localStores.ZoneStore = zoneMock

			// EventConfig mock
			eventCfg := &eventv1.EventConfig{
				Status: eventv1.EventConfigStatus{
					PublishURL: "https://horizon.example.com/events/v1",
				},
			}
			eventCfgMock := mocks.NewMockObjectStore[*eventv1.EventConfig](GinkgoT())
			eventCfgMock.EXPECT().Get(mock.Anything, mock.Anything, mock.Anything).Return(eventCfg, nil).Maybe()
			localStores.EventConfigStore = eventCfgMock

			roverWithEventExp := rover.DeepCopy()
			roverWithEventExp.Status.ApiExposures = nil
			roverWithEventExp.Status.EventExposures = []types.ObjectRef{
				{Name: "event-exp-1", Namespace: "test-ns"},
			}

			appInfo := &api.ApplicationInfo{}
			err := FillExposureInfo(secCtx, roverWithEventExp, appInfo, localStores)

			Expect(err).To(BeNil())
			Expect(appInfo.Exposures).To(HaveLen(1))
			Expect(appInfo.StargatePublishEventUrl).To(Equal("https://horizon.example.com/events/v1"))
		})

		It("must return error when zone fetch fails for event exposures", func() {
			secCtx := security.ToContext(ctx, &security.BusinessContext{Environment: "poc"})

			localStores := &store.Stores{}

			apiExpMock := mocks.NewMockObjectStore[*apiv1.ApiExposure](GinkgoT())
			localStores.APIExposureStore = apiExpMock

			readyEventExp := &eventv1.EventExposure{
				TypeMeta:   metav1.TypeMeta{APIVersion: "event.2.2/v1", Kind: "EventExposure"},
				ObjectMeta: metav1.ObjectMeta{Name: "event-exp-1", Namespace: "test-ns"},
				Spec: eventv1.EventExposureSpec{
					EventType:  "de.telekom.test.event.v1",
					Visibility: eventv1.VisibilityEnterprise,
					Approval:   eventv1.Approval{Strategy: eventv1.ApprovalStrategySimple},
				},
				Status: eventv1.EventExposureStatus{
					Conditions: []metav1.Condition{
						{Type: "Ready", Status: metav1.ConditionTrue, Reason: "Ready"},
					},
				},
			}
			eventExpMock := mocks.NewMockObjectStore[*eventv1.EventExposure](GinkgoT())
			eventExpMock.EXPECT().Get(mock.Anything, mock.Anything, mock.Anything).Return(readyEventExp, nil).Maybe()
			localStores.EventExposureStore = eventExpMock

			zoneMock := mocks.NewMockObjectStore[*adminv1.Zone](GinkgoT())
			zoneMock.EXPECT().Get(mock.Anything, mock.Anything, mock.Anything).Return(nil, fmt.Errorf("zone not found")).Maybe()
			localStores.ZoneStore = zoneMock

			roverWithEventExp := rover.DeepCopy()
			roverWithEventExp.Status.ApiExposures = nil
			roverWithEventExp.Status.EventExposures = []types.ObjectRef{
				{Name: "event-exp-1", Namespace: "test-ns"},
			}

			appInfo := &api.ApplicationInfo{}
			err := FillExposureInfo(secCtx, roverWithEventExp, appInfo, localStores)

			Expect(err).ToNot(BeNil())
			Expect(err.Error()).To(ContainSubstring("failed to get zone"))
		})

		It("must record error when EventExposureStore.Get fails", func() {
			localStores := &store.Stores{}

			apiExpMock := mocks.NewMockObjectStore[*apiv1.ApiExposure](GinkgoT())
			localStores.APIExposureStore = apiExpMock

			failedExp := &eventv1.EventExposure{
				TypeMeta:   metav1.TypeMeta{APIVersion: "event.2.2/v1", Kind: "EventExposure"},
				ObjectMeta: metav1.ObjectMeta{Name: "event-exp-1", Namespace: "test-ns"},
				Status: eventv1.EventExposureStatus{
					Conditions: []metav1.Condition{
						{Type: "Ready", Status: metav1.ConditionFalse, Reason: "Error", Message: "event store error cause"},
					},
				},
			}
			eventExpMock := mocks.NewMockObjectStore[*eventv1.EventExposure](GinkgoT())
			eventExpMock.EXPECT().Get(mock.Anything, mock.Anything, mock.Anything).Return(failedExp, fmt.Errorf("event store error")).Maybe()
			localStores.EventExposureStore = eventExpMock

			roverWithEventExp := rover.DeepCopy()
			roverWithEventExp.Status.ApiExposures = nil
			roverWithEventExp.Status.EventExposures = []types.ObjectRef{
				{Name: "event-exp-1", Namespace: "test-ns"},
			}

			appInfo := &api.ApplicationInfo{}
			err := FillExposureInfo(ctx, roverWithEventExp, appInfo, localStores)

			Expect(err).To(BeNil())
			Expect(appInfo.Errors).To(HaveLen(1))
			Expect(appInfo.Errors[0].Message).To(Equal("event store error"))
		})
	})

	Context("FillSubscriptionInfo", func() {
		It("must fill subscription info correctly", func() {
			var applicationInfo = &api.ApplicationInfo{}
			err := FillSubscriptionInfo(ctx, rover, applicationInfo, stores)

			Expect(err).To(BeNil())

			snaps.MatchJSON(GinkgoT(), applicationInfo)
		})

		It("must populate HorizonSubscriptionId and HorizonSubscriptionUrl for event subscriptions", func() {
			var applicationInfo = &api.ApplicationInfo{}
			err := FillSubscriptionInfo(ctx, rover, applicationInfo, stores)

			Expect(err).To(BeNil())

			// rover fixture has 1 API subscription + 1 event subscription
			Expect(applicationInfo.Subscriptions).To(HaveLen(2))

			// The event subscription is the second one (after the API subscription)
			eventSubInfo, err := applicationInfo.Subscriptions[1].AsEventSubscriptionInfo()
			Expect(err).To(BeNil())
			Expect(eventSubInfo.HorizonSubscriptionId).To(Equal("horizon-sub-456"))
			Expect(eventSubInfo.HorizonSubscriptionUrl).To(Equal("https://sse.example.com/events/horizon-sub-456"))
		})

		It("must return an error if the input rover is nil", func() {
			var applicationInfo = &api.ApplicationInfo{}
			err := FillSubscriptionInfo(ctx, nil, applicationInfo, stores)

			Expect(err).ToNot(BeNil())
			Expect(err.Error()).To(ContainSubstring("input rover is nil"))
		})

		It("must return an error if the input application info is nil", func() {
			err := FillSubscriptionInfo(ctx, rover, nil, stores)

			Expect(err).ToNot(BeNil())
			Expect(err.Error()).To(ContainSubstring("input applicationInfo is nil"))
		})

		It("must record error when APISubscriptionStore.Get fails", func() {
			localStores := &store.Stores{}

			failedSub := &apiv1.ApiSubscription{
				TypeMeta:   metav1.TypeMeta{APIVersion: "rover/v1", Kind: "ApiSubscription"},
				ObjectMeta: metav1.ObjectMeta{Name: "failed-sub", Namespace: "test-ns"},
				Status: apiv1.ApiSubscriptionStatus{
					Conditions: []metav1.Condition{
						{Type: "Ready", Status: metav1.ConditionFalse, Reason: "Error", Message: "api sub error cause"},
					},
				},
			}
			apiSubMock := mocks.NewMockObjectStore[*apiv1.ApiSubscription](GinkgoT())
			apiSubMock.EXPECT().Get(mock.Anything, mock.Anything, mock.Anything).Return(failedSub, fmt.Errorf("api sub error")).Maybe()
			localStores.APISubscriptionStore = apiSubMock

			eventSubMock := mocks.NewMockObjectStore[*eventv1.EventSubscription](GinkgoT())
			eventSubMock.EXPECT().Get(mock.Anything, mock.Anything, mock.Anything).Return(nil, nil).Maybe()
			localStores.EventSubscriptionStore = eventSubMock

			roverWithSub := rover.DeepCopy()
			roverWithSub.Status.ApiSubscriptions = []types.ObjectRef{
				{Name: "some-sub", Namespace: "test-ns"},
			}
			roverWithSub.Status.EventSubscriptions = nil

			appInfo := &api.ApplicationInfo{}
			err := FillSubscriptionInfo(ctx, roverWithSub, appInfo, localStores)

			Expect(err).To(BeNil())
			Expect(appInfo.Errors).To(HaveLen(1))
			Expect(appInfo.Errors[0].Message).To(Equal("api sub error"))
		})

		It("must record error when EventSubscriptionStore.Get fails", func() {
			localStores := &store.Stores{}

			apiSubMock := mocks.NewMockObjectStore[*apiv1.ApiSubscription](GinkgoT())
			localStores.APISubscriptionStore = apiSubMock

			failedSub := &eventv1.EventSubscription{
				TypeMeta:   metav1.TypeMeta{APIVersion: "event.2.2/v1", Kind: "EventSubscription"},
				ObjectMeta: metav1.ObjectMeta{Name: "failed-event-sub", Namespace: "test-ns"},
				Status: eventv1.EventSubscriptionStatus{
					Conditions: []metav1.Condition{
						{Type: "Ready", Status: metav1.ConditionFalse, Reason: "Error", Message: "event sub error cause"},
					},
				},
			}
			eventSubMock := mocks.NewMockObjectStore[*eventv1.EventSubscription](GinkgoT())
			eventSubMock.EXPECT().Get(mock.Anything, mock.Anything, mock.Anything).Return(failedSub, fmt.Errorf("event sub error")).Maybe()
			localStores.EventSubscriptionStore = eventSubMock

			roverWithSub := rover.DeepCopy()
			roverWithSub.Status.ApiSubscriptions = nil
			roverWithSub.Status.EventSubscriptions = []types.ObjectRef{
				{Name: "some-event-sub", Namespace: "test-ns"},
			}

			appInfo := &api.ApplicationInfo{}
			err := FillSubscriptionInfo(ctx, roverWithSub, appInfo, localStores)

			Expect(err).To(BeNil())
			Expect(appInfo.Errors).To(HaveLen(1))
			Expect(appInfo.Errors[0].Message).To(Equal("event sub error"))
		})
	})

	Context("FillApplicationInfo", func() {
		It("must fill application info correctly", func() {
			var applicationInfo = &api.ApplicationInfo{}
			err := FillApplicationInfo(ctx, rover, applicationInfo, stores)

			Expect(err).To(BeNil())

			snaps.MatchJSON(GinkgoT(), applicationInfo)
		})

		It("must return an error if the input rover is nil", func() {
			var applicationInfo = &api.ApplicationInfo{}
			err := FillApplicationInfo(ctx, nil, applicationInfo, stores)

			Expect(err).ToNot(BeNil())
			Expect(err.Error()).To(ContainSubstring("rover resource is not processed and does not contain an application"))
		})

		It("must return an error if the input application info is nil", func() {
			err := FillApplicationInfo(ctx, rover, nil, stores)

			Expect(err).ToNot(BeNil())
			Expect(err.Error()).To(ContainSubstring("input applicationInfo is nil"))
		})

		It("must return error when ApplicationSecretStore.Get fails", func() {
			localStores := &store.Stores{}

			failedApp := &applicationv1.Application{
				TypeMeta:   metav1.TypeMeta{APIVersion: "application/v1", Kind: "Application"},
				ObjectMeta: metav1.ObjectMeta{Name: "failed-app", Namespace: "test-ns"},
				Status: applicationv1.ApplicationStatus{
					Conditions: []metav1.Condition{
						{Type: "Ready", Status: metav1.ConditionFalse, Reason: "Error", Message: "app store error cause"},
					},
				},
			}
			appMock := mocks.NewMockObjectStore[*applicationv1.Application](GinkgoT())
			appMock.EXPECT().Get(mock.Anything, mock.Anything, mock.Anything).Return(failedApp, fmt.Errorf("app store error")).Maybe()
			localStores.ApplicationSecretStore = appMock

			appInfo := &api.ApplicationInfo{}
			err := FillApplicationInfo(ctx, rover, appInfo, localStores)

			Expect(err).ToNot(BeNil())
			Expect(err.Error()).To(ContainSubstring("failed to get application"))
		})

		It("must return error when ZoneStore.Get fails", func() {
			localStores := &store.Stores{}

			app := &applicationv1.Application{
				TypeMeta:   metav1.TypeMeta{APIVersion: "application/v1", Kind: "Application"},
				ObjectMeta: metav1.ObjectMeta{Name: "test-app", Namespace: "test-ns"},
			}
			appMock := mocks.NewMockObjectStore[*applicationv1.Application](GinkgoT())
			appMock.EXPECT().Get(mock.Anything, mock.Anything, mock.Anything).Return(app, nil).Maybe()
			localStores.ApplicationSecretStore = appMock

			failedZone := &adminv1.Zone{
				TypeMeta:   metav1.TypeMeta{APIVersion: "admin/v1", Kind: "Zone"},
				ObjectMeta: metav1.ObjectMeta{Name: "failed-zone", Namespace: "test-ns"},
				Status: adminv1.ZoneStatus{
					Conditions: []metav1.Condition{
						{Type: "Ready", Status: metav1.ConditionFalse, Reason: "Error", Message: "zone error cause"},
					},
				},
			}
			zoneMock := mocks.NewMockObjectStore[*adminv1.Zone](GinkgoT())
			zoneMock.EXPECT().Get(mock.Anything, mock.Anything, mock.Anything).Return(failedZone, fmt.Errorf("zone error")).Maybe()
			localStores.ZoneStore = zoneMock

			appInfo := &api.ApplicationInfo{}
			err := FillApplicationInfo(ctx, rover, appInfo, localStores)

			Expect(err).ToNot(BeNil())
			Expect(err.Error()).To(ContainSubstring("failed to get zone"))
		})
	})

	Context("MapApplicationInfo", func() {
		It("must map application info correctly", func() {
			output, err := MapApplicationInfo(ctx, rover, stores)

			Expect(err).To(BeNil())

			snaps.MatchJSON(GinkgoT(), output)
		})

		It("must return an error if the input rover is nil", func() {
			output, err := MapApplicationInfo(ctx, nil, stores)

			Expect(output).To(BeNil())

			Expect(err).ToNot(BeNil())
			Expect(err.Error()).To(ContainSubstring("input rover is nil"))
		})

		It("must propagate FillApplicationInfo error", func() {
			localStores := &store.Stores{}

			failedApp := &applicationv1.Application{
				TypeMeta:   metav1.TypeMeta{APIVersion: "application/v1", Kind: "Application"},
				ObjectMeta: metav1.ObjectMeta{Name: "failed-app", Namespace: "test-ns"},
				Status: applicationv1.ApplicationStatus{
					Conditions: []metav1.Condition{
						{Type: "Ready", Status: metav1.ConditionFalse, Reason: "Error", Message: "app error cause"},
					},
				},
			}
			appMock := mocks.NewMockObjectStore[*applicationv1.Application](GinkgoT())
			appMock.EXPECT().Get(mock.Anything, mock.Anything, mock.Anything).Return(failedApp, fmt.Errorf("app error")).Maybe()
			localStores.ApplicationSecretStore = appMock

			output, err := MapApplicationInfo(ctx, rover, localStores)

			Expect(output).To(BeNil())
			Expect(err).ToNot(BeNil())
			Expect(err.Error()).To(ContainSubstring("failed to fill application info"))
		})
	})

	Context("FillChevronInfo", func() {
		It("must skip when rover has no authorization", func() {
			appInfo := &api.ApplicationInfo{}
			err := FillChevronInfo(ctx, rover, appInfo, stores)

			Expect(err).To(BeNil())
			Expect(appInfo.ChevronUrl).To(BeEmpty())
			Expect(appInfo.ChevronApplication).To(BeEmpty())
			Expect(appInfo.Authorization).To(BeNil())
		})

		It("must populate chevron info when authorization is configured", func() {
			secCtx := security.ToContext(ctx, &security.BusinessContext{Environment: "poc"})

			localStores := &store.Stores{}

			zone := &adminv1.Zone{
				Status: adminv1.ZoneStatus{
					Links: adminv1.Links{
						PermissionsUrl: "https://stargate.example.com/eni/chevron/v2/permission",
					},
				},
			}
			zoneMock := mocks.NewMockObjectStore[*adminv1.Zone](GinkgoT())
			zoneMock.EXPECT().Get(mock.Anything, mock.Anything, mock.Anything).Return(zone, nil).Maybe()
			localStores.ZoneStore = zoneMock

			roverWithAuth := rover.DeepCopy()
			roverWithAuth.Spec.Authorization = []roverv1.Authorization{
				{
					Resource: "stargate:myapi:v1",
					Permissions: []roverv1.AuthorizationPermission{
						{Role: "admin", Actions: []string{"read", "write"}},
					},
				},
			}

			appInfo := &api.ApplicationInfo{IrisClientId: "test-client-id"}
			err := FillChevronInfo(secCtx, roverWithAuth, appInfo, localStores)

			Expect(err).To(BeNil())
			Expect(appInfo.ChevronUrl).To(Equal("https://stargate.example.com/eni/chevron/v2/permission?application=test-client-id"))
			Expect(appInfo.ChevronApplication).To(Equal("test-client-id"))
			Expect(appInfo.Authorization).To(HaveLen(1))
			Expect(appInfo.Authorization[0].Resource).To(Equal("stargate:myapi:v1"))
			Expect(appInfo.Authorization[0].Permissions).To(HaveLen(1))
			Expect(appInfo.Authorization[0].Permissions[0].Role).To(Equal("admin"))
			Expect(appInfo.Authorization[0].Permissions[0].Actions).To(ConsistOf("read", "write"))

			// Verify variables are populated
			Expect(appInfo.Variables).To(ContainElement(api.Data{
				Name:  "tardis.chevron.url",
				Value: "https://stargate.example.com/eni/chevron/v2/permission?application=test-client-id",
			}))
			Expect(appInfo.Variables).To(ContainElement(api.Data{
				Name:  "tardis.chevron.application",
				Value: "test-client-id",
			}))
		})

		It("must skip chevron fields when zone has no chevron URL", func() {
			secCtx := security.ToContext(ctx, &security.BusinessContext{Environment: "poc"})

			localStores := &store.Stores{}

			zone := &adminv1.Zone{
				Status: adminv1.ZoneStatus{
					Links: adminv1.Links{}, // No PermissionsUrl
				},
			}
			zoneMock := mocks.NewMockObjectStore[*adminv1.Zone](GinkgoT())
			zoneMock.EXPECT().Get(mock.Anything, mock.Anything, mock.Anything).Return(zone, nil).Maybe()
			localStores.ZoneStore = zoneMock

			roverWithAuth := rover.DeepCopy()
			roverWithAuth.Spec.Authorization = []roverv1.Authorization{
				{Role: "admin", Resource: "res", Actions: []string{"read"}},
			}

			appInfo := &api.ApplicationInfo{IrisClientId: "test-client-id"}
			err := FillChevronInfo(secCtx, roverWithAuth, appInfo, localStores)

			Expect(err).To(BeNil())
			Expect(appInfo.ChevronUrl).To(BeEmpty())
			Expect(appInfo.ChevronApplication).To(BeEmpty())
			Expect(appInfo.Authorization).To(BeNil())
		})

		It("must return error when zone fetch fails", func() {
			secCtx := security.ToContext(ctx, &security.BusinessContext{Environment: "poc"})

			localStores := &store.Stores{}

			zoneMock := mocks.NewMockObjectStore[*adminv1.Zone](GinkgoT())
			zoneMock.EXPECT().Get(mock.Anything, mock.Anything, mock.Anything).Return(nil, fmt.Errorf("zone not found")).Maybe()
			localStores.ZoneStore = zoneMock

			roverWithAuth := rover.DeepCopy()
			roverWithAuth.Spec.Authorization = []roverv1.Authorization{
				{Role: "admin", Resource: "res", Actions: []string{"read"}},
			}

			appInfo := &api.ApplicationInfo{}
			err := FillChevronInfo(secCtx, roverWithAuth, appInfo, localStores)

			Expect(err).ToNot(BeNil())
			Expect(err.Error()).To(ContainSubstring("failed to get zone for chevron info"))
		})

		It("must properly URL-encode client ID with special characters", func() {
			secCtx := security.ToContext(ctx, &security.BusinessContext{Environment: "poc"})

			localStores := &store.Stores{}

			zone := &adminv1.Zone{
				Status: adminv1.ZoneStatus{
					Links: adminv1.Links{
						PermissionsUrl: "https://stargate.example.com/eni/chevron/v2/permission",
					},
				},
			}
			zoneMock := mocks.NewMockObjectStore[*adminv1.Zone](GinkgoT())
			zoneMock.EXPECT().Get(mock.Anything, mock.Anything, mock.Anything).Return(zone, nil).Maybe()
			localStores.ZoneStore = zoneMock

			roverWithAuth := rover.DeepCopy()
			roverWithAuth.Spec.Authorization = []roverv1.Authorization{
				{Role: "admin", Resource: "res", Actions: []string{"read"}},
			}

			// Client ID with special characters that need URL encoding
			appInfo := &api.ApplicationInfo{IrisClientId: "client&id=test?value"}
			err := FillChevronInfo(secCtx, roverWithAuth, appInfo, localStores)

			Expect(err).To(BeNil())
			// The special characters should be URL-encoded
			Expect(appInfo.ChevronUrl).To(Equal("https://stargate.example.com/eni/chevron/v2/permission?application=client%26id%3Dtest%3Fvalue"))
			Expect(appInfo.ChevronApplication).To(Equal("client&id=test?value"))
		})

		It("must handle base URL with existing query params", func() {
			secCtx := security.ToContext(ctx, &security.BusinessContext{Environment: "poc"})

			localStores := &store.Stores{}

			zone := &adminv1.Zone{
				Status: adminv1.ZoneStatus{
					Links: adminv1.Links{
						// Base URL already has query params
						PermissionsUrl: "https://stargate.example.com/eni/chevron/v2/permission?env=prod&tenant=acme",
					},
				},
			}
			zoneMock := mocks.NewMockObjectStore[*adminv1.Zone](GinkgoT())
			zoneMock.EXPECT().Get(mock.Anything, mock.Anything, mock.Anything).Return(zone, nil).Maybe()
			localStores.ZoneStore = zoneMock

			roverWithAuth := rover.DeepCopy()
			roverWithAuth.Spec.Authorization = []roverv1.Authorization{
				{Role: "admin", Resource: "res", Actions: []string{"read"}},
			}

			appInfo := &api.ApplicationInfo{IrisClientId: "test-client"}
			err := FillChevronInfo(secCtx, roverWithAuth, appInfo, localStores)

			Expect(err).To(BeNil())
			// Should append application param correctly to existing query params
			Expect(appInfo.ChevronUrl).To(ContainSubstring("application=test-client"))
			Expect(appInfo.ChevronUrl).To(ContainSubstring("env=prod"))
			Expect(appInfo.ChevronUrl).To(ContainSubstring("tenant=acme"))
			// Should have proper query param separators
			Expect(appInfo.ChevronUrl).To(MatchRegexp(`\?.*&.*&`))
		})

		It("must return error when base URL is malformed", func() {
			secCtx := security.ToContext(ctx, &security.BusinessContext{Environment: "poc"})

			localStores := &store.Stores{}

			zone := &adminv1.Zone{
				Status: adminv1.ZoneStatus{
					Links: adminv1.Links{
						// Invalid URL scheme
						PermissionsUrl: "ht!tp://invalid url with spaces",
					},
				},
			}
			zoneMock := mocks.NewMockObjectStore[*adminv1.Zone](GinkgoT())
			zoneMock.EXPECT().Get(mock.Anything, mock.Anything, mock.Anything).Return(zone, nil).Maybe()
			localStores.ZoneStore = zoneMock

			roverWithAuth := rover.DeepCopy()
			roverWithAuth.Spec.Authorization = []roverv1.Authorization{
				{Role: "admin", Resource: "res", Actions: []string{"read"}},
			}

			appInfo := &api.ApplicationInfo{IrisClientId: "test-client"}
			err := FillChevronInfo(secCtx, roverWithAuth, appInfo, localStores)

			Expect(err).ToNot(BeNil())
			Expect(err.Error()).To(ContainSubstring("failed to parse chevron URL"))
		})

		It("must map all authorization formats correctly", func() {
			secCtx := security.ToContext(ctx, &security.BusinessContext{Environment: "poc"})

			localStores := &store.Stores{}

			zone := &adminv1.Zone{
				Status: adminv1.ZoneStatus{
					Links: adminv1.Links{
						PermissionsUrl: "https://stargate.example.com/eni/chevron/v2/permission",
					},
				},
			}
			zoneMock := mocks.NewMockObjectStore[*adminv1.Zone](GinkgoT())
			zoneMock.EXPECT().Get(mock.Anything, mock.Anything, mock.Anything).Return(zone, nil).Maybe()
			localStores.ZoneStore = zoneMock

			roverWithAuth := rover.DeepCopy()
			roverWithAuth.Spec.Authorization = []roverv1.Authorization{
				// Resource-oriented
				{
					Resource: "api:v1",
					Permissions: []roverv1.AuthorizationPermission{
						{Role: "admin", Actions: []string{"read"}},
					},
				},
				// Role-oriented
				{
					Role: "viewer",
					Permissions: []roverv1.AuthorizationPermission{
						{Resource: "dashboard", Actions: []string{"view"}},
					},
				},
				// Flat
				{
					Role:     "operator",
					Resource: "cluster",
					Actions:  []string{"restart"},
				},
			}

			appInfo := &api.ApplicationInfo{IrisClientId: "my-app"}
			err := FillChevronInfo(secCtx, roverWithAuth, appInfo, localStores)

			Expect(err).To(BeNil())
			Expect(appInfo.Authorization).To(HaveLen(3))

			// Resource-oriented format preserved as-is
			Expect(appInfo.Authorization[0].Resource).To(Equal("api:v1"))
			Expect(appInfo.Authorization[0].Permissions).To(HaveLen(1))

			// Role-oriented format preserved as-is
			Expect(appInfo.Authorization[1].Role).To(Equal("viewer"))
			Expect(appInfo.Authorization[1].Permissions).To(HaveLen(1))

			// Flat format preserved as-is
			Expect(appInfo.Authorization[2].Role).To(Equal("operator"))
			Expect(appInfo.Authorization[2].Resource).To(Equal("cluster"))
			Expect(appInfo.Authorization[2].Actions).To(ConsistOf("restart"))
		})
	})

	Context("toApiVisibilityFromEvent", func() {
		DescribeTable("converts event visibility to API visibility",
			func(input eventv1.Visibility, expected api.Visibility) {
				Expect(toApiVisibilityFromEvent(input)).To(Equal(expected))
			},
			Entry("World", eventv1.VisibilityWorld, api.WORLD),
			Entry("Zone", eventv1.VisibilityZone, api.ZONE),
			Entry("Enterprise", eventv1.VisibilityEnterprise, api.ENTERPRISE),
			Entry("unknown defaults to uppercased string", eventv1.Visibility("custom"), api.Visibility("CUSTOM")),
		)
	})

	Context("toApiApprovalStrategyFromEvent", func() {
		DescribeTable("converts event approval strategy to API approval strategy",
			func(input eventv1.ApprovalStrategy, expected api.ApprovalStrategy) {
				Expect(toApiApprovalStrategyFromEvent(input)).To(Equal(expected))
			},
			Entry("Auto", eventv1.ApprovalStrategyAuto, api.AUTO),
			Entry("Simple", eventv1.ApprovalStrategySimple, api.SIMPLE),
			Entry("FourEyes", eventv1.ApprovalStrategyFourEyes, api.FOUREYES),
			Entry("unknown defaults to uppercased string", eventv1.ApprovalStrategy("custom"), api.ApprovalStrategy("CUSTOM")),
		)
	})
})
