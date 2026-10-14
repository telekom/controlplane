// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package applicationinfo

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/mock"

	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	apiv1 "github.com/telekom/controlplane/api/api/v1"
	applicationv1 "github.com/telekom/controlplane/application/api/v1"
	storeLib "github.com/telekom/controlplane/common-server/pkg/store"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/types"
	eventv1 "github.com/telekom/controlplane/event/api/v1"
	roverv1 "github.com/telekom/controlplane/rover/api/v1"
	spectrev1 "github.com/telekom/controlplane/spectre/api/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/telekom/controlplane/rover-server/internal/api"
	"github.com/telekom/controlplane/rover-server/internal/mapper/status"
	"github.com/telekom/controlplane/rover-server/pkg/store"
	"github.com/telekom/controlplane/rover-server/test/mocks"
)

// childStore serves the given objects by name and by owner, so get-info and the
// status endpoint read the same sub-resources.
func childStore[T storeLib.Object](objs ...T) *mocks.MockObjectStore[T] {
	m := mocks.NewMockObjectStore[T](GinkgoT())
	for _, obj := range objs {
		m.EXPECT().Get(mock.Anything, obj.GetNamespace(), obj.GetName()).Return(obj, nil).Maybe()
	}
	m.EXPECT().List(mock.Anything, mock.Anything).Return(&storeLib.ListResponse[T]{Items: objs}, nil).Maybe()
	return m
}

var _ = Describe("MapApplicationInfo status", func() {
	const namespace = "poc--eni--team"

	conditions := func(ready metav1.ConditionStatus, reason string, generation int64) []metav1.Condition {
		processing := metav1.Condition{Type: condition.ConditionTypeProcessing, Status: metav1.ConditionFalse, Reason: condition.ReasonDone, ObservedGeneration: generation}
		switch reason {
		case condition.ReasonSubResourceNotReady:
			processing.Status, processing.Reason = metav1.ConditionTrue, condition.ReasonSubResourceNotReady
		case condition.ReasonApprovalPending, condition.ReasonBlocked:
			processing.Reason = condition.ReasonBlocked
		}
		return []metav1.Condition{
			processing,
			{Type: condition.ConditionTypeReady, Status: ready, Reason: reason, Message: reason, ObservedGeneration: generation},
		}
	}
	ready := conditions(metav1.ConditionTrue, condition.ReasonProvisioned, 1)
	waiting := conditions(metav1.ConditionFalse, condition.ReasonSubResourceNotReady, 1)
	pending := conditions(metav1.ConditionFalse, condition.ReasonApprovalPending, 1)
	blocked := conditions(metav1.ConditionFalse, condition.ReasonBlocked, 1)

	meta := func(kind, name string, generation int64) (metav1.TypeMeta, metav1.ObjectMeta) {
		return metav1.TypeMeta{APIVersion: "test.cp.ei.telekom.de/v1", Kind: kind},
			metav1.ObjectMeta{Name: name, Namespace: namespace, Generation: generation}
	}

	type fixture struct {
		rover       []metav1.Condition
		roverGen    int64
		application []metav1.Condition
		apiSub      []metav1.Condition
		apiSubGen   int64
		eventSub    []metav1.Condition
		listener    []metav1.Condition
		// spectreApp defaults to ready when the fixture has a listener.
		spectreApp []metav1.Condition
	}

	// build returns the Rover and stores of a fixture; children without conditions are omitted.
	build := func(f fixture) (*roverv1.Rover, *store.Stores) {
		r := &roverv1.Rover{
			ObjectMeta: metav1.ObjectMeta{Name: "observer", Namespace: namespace, UID: "uid-observer", Generation: f.roverGen},
			Status: roverv1.RoverStatus{
				Application: &types.ObjectRef{Name: "observer", Namespace: namespace},
				Conditions:  f.rover,
			},
		}

		app := &applicationv1.Application{Status: applicationv1.ApplicationStatus{Conditions: f.application}}
		app.TypeMeta, app.ObjectMeta = meta("Application", "observer", 1)
		appStore := childStore(app)

		zone := &adminv1.Zone{
			Spec:   adminv1.ZoneSpec{Presets: []adminv1.Preset{{Name: "default", Type: adminv1.GatewayTypeAPI, Default: true}}},
			Status: adminv1.ZoneStatus{Presets: []adminv1.PresetStatus{{Name: "default"}}},
		}
		zoneStore := mocks.NewMockObjectStore[*adminv1.Zone](GinkgoT())
		zoneStore.EXPECT().Get(mock.Anything, mock.Anything, mock.Anything).Return(zone, nil).Maybe()

		s := &store.Stores{ApplicationStore: appStore, ApplicationSecretStore: appStore, ZoneStore: zoneStore}

		if f.apiSub != nil {
			apiSub := &apiv1.ApiSubscription{
				Spec:   apiv1.ApiSubscriptionSpec{ApiBasePath: "/orders/v1"},
				Status: apiv1.ApiSubscriptionStatus{Conditions: f.apiSub},
			}
			apiSub.TypeMeta, apiSub.ObjectMeta = meta("ApiSubscription", "orders-v1", f.apiSubGen)
			r.Status.ApiSubscriptions = []types.ObjectRef{{Name: apiSub.Name, Namespace: namespace}}
			s.APISubscriptionStore = childStore(apiSub)
		}

		if f.eventSub != nil {
			eventSub := &eventv1.EventSubscription{
				Spec:   eventv1.EventSubscriptionSpec{EventType: "de.telekom.test.v1"},
				Status: eventv1.EventSubscriptionStatus{Conditions: f.eventSub},
			}
			eventSub.TypeMeta, eventSub.ObjectMeta = meta("EventSubscription", "test-v1", 1)
			r.Status.EventSubscriptions = []types.ObjectRef{{Name: eventSub.Name, Namespace: namespace}}
			s.EventSubscriptionStore = childStore(eventSub)
		}

		if f.listener != nil {
			spectreAppConditions := f.spectreApp
			if spectreAppConditions == nil {
				spectreAppConditions = ready
			}
			spectreApp := &spectrev1.SpectreApplication{Status: spectrev1.SpectreApplicationStatus{Conditions: spectreAppConditions}}
			spectreApp.TypeMeta, spectreApp.ObjectMeta = meta("SpectreApplication", "observer--spectre-app", 1)
			listener := &spectrev1.Listener{Status: spectrev1.ListenerStatus{Conditions: f.listener}}
			listener.TypeMeta, listener.ObjectMeta = meta("Listener", "observer--consumer--provider---orders-v1", 1)
			r.Status.SpectreApplications = []types.ObjectRef{{Name: spectreApp.Name, Namespace: namespace}}
			r.Status.SpectreListeners = []types.ObjectRef{{Name: listener.Name, Namespace: namespace}}
			s.SpectreApplicationStore = childStore(spectreApp)
			s.SpectreListenerStore = childStore(listener)
		}

		return r, s
	}

	DescribeTable("matches the status endpoint",
		func(f fixture, expected api.OverallStatus) {
			r, s := build(f)

			appInfo, err := MapApplicationInfo(ctx, r, s)
			Expect(err).NotTo(HaveOccurred())
			statusResponse, err := status.MapRoverResponse(ctx, r, s)
			Expect(err).NotTo(HaveOccurred())

			Expect(appInfo.Status).To(Equal(statusResponse.OverallStatus))
			Expect(appInfo.Status).To(Equal(expected))
		},
		Entry("complete Rover with ready sub-resources",
			fixture{rover: ready, roverGen: 1, application: ready, apiSub: ready, apiSubGen: 1, listener: ready},
			api.OverallStatusComplete),
		Entry("listener approval pending",
			fixture{rover: waiting, roverGen: 1, application: ready, listener: pending},
			api.OverallStatusBlocked),
		Entry("SpectreApplication not ready",
			fixture{rover: waiting, roverGen: 1, application: ready, listener: ready, spectreApp: blocked},
			api.OverallStatusBlocked),
		Entry("Application blocked",
			fixture{rover: waiting, roverGen: 1, application: blocked},
			api.OverallStatusBlocked),
		Entry("API subscription approval pending",
			fixture{rover: waiting, roverGen: 1, application: ready, apiSub: pending, apiSubGen: 1},
			api.OverallStatusBlocked),
		Entry("event subscription approval pending",
			fixture{rover: waiting, roverGen: 1, application: ready, eventSub: pending},
			api.OverallStatusBlocked),
		// A sub-resource still processing keeps the Rover's state processing, but
		// the overall status takes the worst sub-resource.
		Entry("blocked sub-resource next to one still processing",
			fixture{rover: waiting, roverGen: 1, application: ready, apiSub: pending, apiSubGen: 1, eventSub: waiting},
			api.OverallStatusBlocked),
		Entry("sub-resource still processing",
			fixture{rover: waiting, roverGen: 1, application: ready, apiSub: waiting, apiSubGen: 1},
			api.OverallStatusProcessing),
		// The two entries below are complete by the Rover's own conditions alone.
		Entry("complete Rover with a sub-resource whose spec changed since it was last processed",
			fixture{rover: ready, roverGen: 1, application: ready, apiSub: ready, apiSubGen: 2},
			api.OverallStatusProcessing),
		Entry("complete Rover whose spec changed since it was last processed",
			fixture{rover: ready, roverGen: 2, application: ready},
			api.OverallStatusPending),
	)

	It("records a failed sub-resource lookup and continues with the rest", func() {
		r, s := build(fixture{rover: waiting, roverGen: 1, application: ready})
		r.Status.ApiSubscriptions = []types.ObjectRef{{Name: "missing-v1", Namespace: namespace}, {Name: "orders-v1", Namespace: namespace}}
		apiSub := &apiv1.ApiSubscription{
			Spec:   apiv1.ApiSubscriptionSpec{ApiBasePath: "/orders/v1"},
			Status: apiv1.ApiSubscriptionStatus{Conditions: ready},
		}
		apiSub.TypeMeta, apiSub.ObjectMeta = meta("ApiSubscription", "orders-v1", 1)
		apiSubStore := mocks.NewMockObjectStore[*apiv1.ApiSubscription](GinkgoT())
		apiSubStore.EXPECT().Get(mock.Anything, namespace, "missing-v1").Return(nil, fmt.Errorf("api subscription store error")).Once()
		apiSubStore.EXPECT().Get(mock.Anything, namespace, "orders-v1").Return(apiSub, nil).Once()
		apiSubStore.EXPECT().Info().Return(schema.GroupVersionResource{}, schema.GroupVersionKind{Group: "api.cp.ei.telekom.de", Version: "v1", Kind: "ApiSubscription"}).Once()
		// get-info must not list sub-resources; a failing List would fail the whole request.
		apiSubStore.EXPECT().List(mock.Anything, mock.Anything).Return(nil, fmt.Errorf("api subscription list error")).Maybe()
		s.APISubscriptionStore = apiSubStore

		appInfo, err := MapApplicationInfo(ctx, r, s)

		Expect(err).NotTo(HaveOccurred())
		Expect(appInfo.Subscriptions).To(HaveLen(1))
		Expect(appInfo.Errors).To(ContainElement(HaveField("Message", "api subscription store error")))
	})
})
