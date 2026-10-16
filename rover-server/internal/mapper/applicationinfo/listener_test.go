// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package applicationinfo

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/mock"

	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/types"
	roverv1 "github.com/telekom/controlplane/rover/api/v1"
	spectrev1 "github.com/telekom/controlplane/spectre/api/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/telekom/controlplane/rover-server/internal/api"
	"github.com/telekom/controlplane/rover-server/internal/mapper/status"
	"github.com/telekom/controlplane/rover-server/pkg/store"
	"github.com/telekom/controlplane/rover-server/test/mocks"
)

var _ = Describe("FillListenerInfo", func() {
	const namespace = "poc--eni--observer-team"

	readyConditions := []metav1.Condition{
		{Type: condition.ConditionTypeReady, Status: metav1.ConditionTrue, Reason: condition.ReasonProvisioned},
	}

	applicationRef := func(namespace, name string) types.TypedObjectRef {
		return types.TypedObjectRef{
			TypeMeta:  metav1.TypeMeta{Kind: "Application", APIVersion: "application.cp.ei.telekom.de/v1"},
			ObjectRef: types.ObjectRef{Name: name, Namespace: namespace},
		}
	}

	newSpectreApp := func(deliveryType, callback, sseUrl string) *spectrev1.SpectreApplication {
		return &spectrev1.SpectreApplication{
			TypeMeta:   metav1.TypeMeta{APIVersion: "spectre.cp.ei.telekom.de/v1", Kind: "SpectreApplication"},
			ObjectMeta: metav1.ObjectMeta{Name: "observer--spectre-app", Namespace: namespace},
			Spec:       spectrev1.SpectreApplicationSpec{DeliveryType: deliveryType, Callback: callback},
			Status:     spectrev1.SpectreApplicationStatus{Conditions: readyConditions, SseUrl: sseUrl},
		}
	}

	apiListener := &spectrev1.Listener{
		TypeMeta:   metav1.TypeMeta{APIVersion: "spectre.cp.ei.telekom.de/v1", Kind: "Listener"},
		ObjectMeta: metav1.ObjectMeta{Name: "observer--consumer--provider---orders-v1", Namespace: namespace},
		Spec: spectrev1.ListenerSpec{
			Consumer: applicationRef("poc--eni--consumer-team", "consumer"),
			Provider: applicationRef("poc--hub--provider-team", "provider"),
			ApiListener: &spectrev1.ApiListener{
				ApiBasePath:    "/orders/v1",
				RequestFilter:  &spectrev1.ListenerFilter{Trigger: map[string]string{"method": "POST"}, Payload: []string{"orderId"}},
				ResponseFilter: &spectrev1.ListenerFilter{Payload: []string{"status"}},
			},
		},
		Status: spectrev1.ListenerStatus{Conditions: readyConditions},
	}

	newRover := func() *roverv1.Rover {
		return &roverv1.Rover{Status: roverv1.RoverStatus{
			SpectreApplications: []types.ObjectRef{{Name: "observer--spectre-app", Namespace: namespace}},
			SpectreListeners:    []types.ObjectRef{{Name: apiListener.Name, Namespace: namespace}},
		}}
	}

	newStores := func(app *spectrev1.SpectreApplication, listener *spectrev1.Listener) *store.Stores {
		appMock := mocks.NewMockObjectStore[*spectrev1.SpectreApplication](GinkgoT())
		appMock.EXPECT().Get(mock.Anything, namespace, "observer--spectre-app").Return(app, nil).Once()
		listenerMock := mocks.NewMockObjectStore[*spectrev1.Listener](GinkgoT())
		listenerMock.EXPECT().Get(mock.Anything, namespace, apiListener.Name).Return(listener, nil).Once()
		return &store.Stores{SpectreApplicationStore: appMock, SpectreListenerStore: listenerMock}
	}

	It("maps the listeners and the SSE delivery of the SpectreApplication", func() {
		appInfo := &api.ApplicationInfo{}

		err := FillListenerInfo(ctx, newRover(), appInfo, newStores(newSpectreApp("server_sent_event", "", "https://events.example.com/sse/abc"), apiListener), &status.ProblemsResult{})

		Expect(err).NotTo(HaveOccurred())
		Expect(appInfo.Listeners).To(Equal([]api.RoverListener{{
			Consumer:       "eni--consumer-team--consumer",
			Provider:       "hub--provider-team--provider",
			ApiBasePath:    "/orders/v1",
			RequestFilter:  api.ListenerFilter{Trigger: map[string]string{"method": "POST"}, Payload: []string{"orderId"}},
			ResponseFilter: api.ListenerFilter{Payload: []string{"status"}},
		}}))
		Expect(appInfo.ListenerSubscription).To(Equal(api.ListenerSubscriptionInfo{
			DeliveryType: api.ListenerSubscriptionInfoDeliveryTypeServerSentEvent,
			SseUrl:       "https://events.example.com/sse/abc",
		}))
		Expect(appInfo.Errors).To(BeEmpty())
	})

	It("maps callback delivery without an SSE URL", func() {
		appInfo := &api.ApplicationInfo{}

		err := FillListenerInfo(ctx, newRover(), appInfo, newStores(newSpectreApp("callback", "https://observer.example.com/listener", ""), apiListener), &status.ProblemsResult{})

		Expect(err).NotTo(HaveOccurred())
		Expect(appInfo.ListenerSubscription).To(Equal(api.ListenerSubscriptionInfo{
			DeliveryType: api.ListenerSubscriptionInfoDeliveryTypeCallback,
			Callback:     "https://observer.example.com/listener",
		}))
	})

	It("records a listener that is not ready", func() {
		pending := apiListener.DeepCopy()
		pending.Status.Conditions = []metav1.Condition{
			{Type: condition.ConditionTypeReady, Status: metav1.ConditionFalse, Reason: condition.ReasonApprovalPending, Message: "Waiting for approval decision (consumer gate)"},
		}
		appInfo := &api.ApplicationInfo{}
		children := &status.ProblemsResult{}

		err := FillListenerInfo(ctx, newRover(), appInfo, newStores(newSpectreApp("server_sent_event", "", ""), pending), children)

		Expect(err).NotTo(HaveOccurred())
		Expect(appInfo.Listeners).To(HaveLen(1))
		Expect(appInfo.Errors).To(HaveLen(1))
		Expect(appInfo.Errors[0].Resource.Kind).To(Equal("Listener"))
		Expect(appInfo.Errors[0].Cause).To(Equal("Waiting for approval decision (consumer gate)"))
		Expect(children.WorstOverallStatus).To(Equal(api.OverallStatusBlocked))
	})

	It("records an error when a listener cannot be read and continues", func() {
		appMock := mocks.NewMockObjectStore[*spectrev1.SpectreApplication](GinkgoT())
		appMock.EXPECT().Get(mock.Anything, namespace, "observer--spectre-app").Return(nil, fmt.Errorf("app store error")).Once()
		appMock.EXPECT().Info().Return(schema.GroupVersionResource{}, schema.GroupVersionKind{Group: "spectre.cp.ei.telekom.de", Version: "v1", Kind: "SpectreApplication"}).Once()
		listenerMock := mocks.NewMockObjectStore[*spectrev1.Listener](GinkgoT())
		listenerMock.EXPECT().Get(mock.Anything, namespace, apiListener.Name).Return(nil, fmt.Errorf("listener store error")).Once()
		listenerMock.EXPECT().Info().Return(schema.GroupVersionResource{}, schema.GroupVersionKind{Group: "spectre.cp.ei.telekom.de", Version: "v1", Kind: "Listener"}).Once()
		appInfo := &api.ApplicationInfo{}

		err := FillListenerInfo(ctx, newRover(), appInfo, &store.Stores{SpectreApplicationStore: appMock, SpectreListenerStore: listenerMock}, &status.ProblemsResult{})

		Expect(err).NotTo(HaveOccurred())
		Expect(appInfo.Listeners).To(BeEmpty())
		Expect(appInfo.ListenerSubscription).To(Equal(api.ListenerSubscriptionInfo{}))
		Expect(appInfo.Errors).To(HaveLen(2))
		Expect(appInfo.Errors[0].Message).To(Equal("app store error"))
		Expect(appInfo.Errors[1].Message).To(Equal("listener store error"))
	})

	It("skips the Spectre stores when the Rover has no listeners", func() {
		appInfo := &api.ApplicationInfo{}

		// Nil stores would panic if queried.
		err := FillListenerInfo(ctx, &roverv1.Rover{}, appInfo, &store.Stores{}, &status.ProblemsResult{})

		Expect(err).NotTo(HaveOccurred())
		Expect(appInfo.Listeners).To(BeEmpty())
	})

	It("must return an error if the input rover is nil", func() {
		err := FillListenerInfo(ctx, nil, &api.ApplicationInfo{}, &store.Stores{}, &status.ProblemsResult{})

		Expect(err).To(MatchError(ContainSubstring("input rover is nil")))
	})

	It("must return an error if the input application info is nil", func() {
		err := FillListenerInfo(ctx, newRover(), nil, &store.Stores{}, &status.ProblemsResult{})

		Expect(err).To(MatchError(ContainSubstring("input applicationInfo is nil")))
	})
})
