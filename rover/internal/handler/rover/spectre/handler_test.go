// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package spectre_test

import (
	"context"
	"fmt"

	"github.com/stretchr/testify/mock"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	cclient "github.com/telekom/controlplane/common/pkg/client"
	fakeclient "github.com/telekom/controlplane/common/pkg/client/fake"
	"github.com/telekom/controlplane/common/pkg/types"
	roverv1 "github.com/telekom/controlplane/rover/api/v1"
	"github.com/telekom/controlplane/rover/internal/handler/rover/spectre"
	spectrev1 "github.com/telekom/controlplane/spectre/api/v1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const (
	testEnvironment = "test-env"
	teamNamespace   = testEnvironment + "--eni--pandora"
)

func createTestOwner() *roverv1.Rover {
	return &roverv1.Rover{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-app",
			Namespace: teamNamespace,
			UID:       "rover-uid-1234",
		},
		Spec: roverv1.RoverSpec{
			Zone: "zone1",
		},
		Status: roverv1.RoverStatus{
			Application: &types.ObjectRef{
				Name:      "my-app",
				Namespace: teamNamespace,
			},
		},
	}
}

func newTestScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	_ = roverv1.AddToScheme(s)
	_ = spectrev1.AddToScheme(s)
	return s
}

var _ = Describe("HandleListeners", func() {
	var (
		ctx        context.Context
		fakeClient *fakeclient.MockJanitorClient
		testScheme *runtime.Scheme
		owner      *roverv1.Rover
	)

	BeforeEach(func() {
		ctx = context.Background()
		fakeClient = fakeclient.NewMockJanitorClient(GinkgoT())
		ctx = cclient.WithClient(ctx, fakeClient)
		testScheme = newTestScheme()
		owner = createTestOwner()
	})

	It("should do nothing when listeners is empty", func() {
		owner.Spec.Listeners = nil

		err := spectre.HandleListeners(ctx, fakeClient, owner)

		Expect(err).ToNot(HaveOccurred())
		Expect(owner.Status.SpectreApplications).To(BeNil())
		Expect(owner.Status.SpectreListeners).To(BeNil())
	})

	It("should create one SpectreApplication and one Listener for a single listener entry", func() {
		owner.Spec.Listeners = []roverv1.RoverListener{
			{
				Consumer:    "eni--team--consumer",
				Provider:    "eni--team--provider",
				ApiBasePath: "/echo/v1",
				RequestFilter: &roverv1.ListenerFilter{
					Trigger: map[string]string{"method": "GET"},
					Payload: []string{"name"},
				},
			},
		}
		owner.Spec.ListenerSubscription = &roverv1.ListenerSubscription{
			DeliveryType: "server_sent_event",
		}

		var capturedApp *spectrev1.SpectreApplication
		var capturedListener *spectrev1.Listener

		fakeClient.EXPECT().Scheme().Return(testScheme).Maybe()
		fakeClient.EXPECT().
			CreateOrUpdate(ctx, mock.AnythingOfType("*v1.SpectreApplication"), mock.AnythingOfType("controllerutil.MutateFn")).
			Run(func(_ context.Context, obj client.Object, mutate controllerutil.MutateFn) {
				_ = mutate()
				capturedApp = obj.(*spectrev1.SpectreApplication)
			}).
			Return(controllerutil.OperationResultCreated, nil).Once()
		fakeClient.EXPECT().
			CreateOrUpdate(ctx, mock.AnythingOfType("*v1.Listener"), mock.AnythingOfType("controllerutil.MutateFn")).
			Run(func(_ context.Context, obj client.Object, mutate controllerutil.MutateFn) {
				_ = mutate()
				capturedListener = obj.(*spectrev1.Listener)
			}).
			Return(controllerutil.OperationResultCreated, nil).Once()

		err := spectre.HandleListeners(ctx, fakeClient, owner)

		Expect(err).ToNot(HaveOccurred())

		// Verify SpectreApplication
		Expect(capturedApp).ToNot(BeNil())
		Expect(capturedApp.Name).To(Equal("my-app--spectre-app"))
		Expect(capturedApp.Namespace).To(Equal(teamNamespace))
		Expect(capturedApp.Spec.Application.Kind).To(Equal("Application"))
		Expect(capturedApp.Spec.Application.APIVersion).To(Equal("application.cp.ei.telekom.de/v1"))
		Expect(capturedApp.Spec.Application.Name).To(Equal("my-app"))
		Expect(capturedApp.Spec.DeliveryType).To(Equal("server_sent_event"))
		Expect(capturedApp.Spec.Callback).To(BeEmpty())

		// Verify Listener
		Expect(capturedListener).ToNot(BeNil())
		Expect(capturedListener.Name).To(Equal("my-app--listener--0"))
		Expect(capturedListener.Namespace).To(Equal(teamNamespace))
		Expect(capturedListener.Spec.Consumer.Name).To(Equal("eni--team--consumer"))
		Expect(capturedListener.Spec.Consumer.Kind).To(Equal("Application"))
		Expect(capturedListener.Spec.Provider.Name).To(Equal("eni--team--provider"))
		Expect(capturedListener.Spec.Provider.Kind).To(Equal("Application"))
		Expect(capturedListener.Spec.Application.Name).To(Equal("my-app--spectre-app"))
		Expect(capturedListener.Spec.ApiListener).ToNot(BeNil())
		Expect(capturedListener.Spec.ApiListener.ApiBasePath).To(Equal("/echo/v1"))
		Expect(capturedListener.Spec.ApiListener.RequestFilter).ToNot(BeNil())
		Expect(capturedListener.Spec.ApiListener.RequestFilter.Trigger).To(HaveKeyWithValue("method", "GET"))
		Expect(capturedListener.Spec.ApiListener.RequestFilter.Payload).To(Equal([]string{"name"}))
		Expect(capturedListener.Spec.ApiListener.ResponseFilter).To(BeNil())
		Expect(capturedListener.Spec.EventListener).To(BeNil())

		// Verify status refs
		Expect(owner.Status.SpectreApplications).To(HaveLen(1))
		Expect(owner.Status.SpectreApplications[0].Name).To(Equal("my-app--spectre-app"))
		Expect(owner.Status.SpectreListeners).To(HaveLen(1))
		Expect(owner.Status.SpectreListeners[0].Name).To(Equal("my-app--listener--0"))
	})

	It("should create two Listeners for two listener entries", func() {
		owner.Spec.Listeners = []roverv1.RoverListener{
			{
				Consumer:    "eni--team--consumer1",
				Provider:    "eni--team--provider1",
				ApiBasePath: "/api/v1",
			},
			{
				Consumer:  "eni--team--consumer2",
				Provider:  "eni--team--provider2",
				EventType: "de.telekom.eni.test.v1",
				EventFilter: &roverv1.ListenerFilter{
					Payload: []string{"status"},
				},
			},
		}

		var capturedListeners []*spectrev1.Listener

		fakeClient.EXPECT().Scheme().Return(testScheme).Maybe()
		fakeClient.EXPECT().
			CreateOrUpdate(ctx, mock.AnythingOfType("*v1.SpectreApplication"), mock.AnythingOfType("controllerutil.MutateFn")).
			Run(func(_ context.Context, _ client.Object, mutate controllerutil.MutateFn) {
				_ = mutate()
			}).
			Return(controllerutil.OperationResultCreated, nil).Once()
		fakeClient.EXPECT().
			CreateOrUpdate(ctx, mock.AnythingOfType("*v1.Listener"), mock.AnythingOfType("controllerutil.MutateFn")).
			Run(func(_ context.Context, obj client.Object, mutate controllerutil.MutateFn) {
				_ = mutate()
				capturedListeners = append(capturedListeners, obj.(*spectrev1.Listener))
			}).
			Return(controllerutil.OperationResultCreated, nil).Times(2)

		err := spectre.HandleListeners(ctx, fakeClient, owner)

		Expect(err).ToNot(HaveOccurred())
		Expect(capturedListeners).To(HaveLen(2))
		Expect(capturedListeners[0].Name).To(Equal("my-app--listener--0"))
		Expect(capturedListeners[0].Spec.ApiListener).ToNot(BeNil())
		Expect(capturedListeners[0].Spec.ApiListener.ApiBasePath).To(Equal("/api/v1"))
		Expect(capturedListeners[0].Spec.EventListener).To(BeNil())

		Expect(capturedListeners[1].Name).To(Equal("my-app--listener--1"))
		Expect(capturedListeners[1].Spec.EventListener).ToNot(BeNil())
		Expect(capturedListeners[1].Spec.EventListener.EventType).To(Equal("de.telekom.eni.test.v1"))
		Expect(capturedListeners[1].Spec.EventListener.Filter).ToNot(BeNil())
		Expect(capturedListeners[1].Spec.EventListener.Filter.Payload).To(Equal([]string{"status"}))
		Expect(capturedListeners[1].Spec.ApiListener).To(BeNil())

		Expect(owner.Status.SpectreListeners).To(HaveLen(2))
	})

	It("should set callback delivery type when listenerSubscription specifies callback", func() {
		owner.Spec.Listeners = []roverv1.RoverListener{
			{
				Consumer:    "eni--team--consumer",
				Provider:    "eni--team--provider",
				ApiBasePath: "/echo/v1",
			},
		}
		owner.Spec.ListenerSubscription = &roverv1.ListenerSubscription{
			DeliveryType: "callback",
			Callback:     "https://my-listener.example.com/events",
		}

		var capturedApp *spectrev1.SpectreApplication

		fakeClient.EXPECT().Scheme().Return(testScheme).Maybe()
		fakeClient.EXPECT().
			CreateOrUpdate(ctx, mock.AnythingOfType("*v1.SpectreApplication"), mock.AnythingOfType("controllerutil.MutateFn")).
			Run(func(_ context.Context, obj client.Object, mutate controllerutil.MutateFn) {
				_ = mutate()
				capturedApp = obj.(*spectrev1.SpectreApplication)
			}).
			Return(controllerutil.OperationResultCreated, nil).Once()
		fakeClient.EXPECT().
			CreateOrUpdate(ctx, mock.AnythingOfType("*v1.Listener"), mock.AnythingOfType("controllerutil.MutateFn")).
			Run(func(_ context.Context, _ client.Object, mutate controllerutil.MutateFn) {
				_ = mutate()
			}).
			Return(controllerutil.OperationResultCreated, nil).Once()

		err := spectre.HandleListeners(ctx, fakeClient, owner)

		Expect(err).ToNot(HaveOccurred())
		Expect(capturedApp.Spec.DeliveryType).To(Equal("callback"))
		Expect(capturedApp.Spec.Callback).To(Equal("https://my-listener.example.com/events"))
	})

	It("should default to server_sent_event when listenerSubscription is nil", func() {
		owner.Spec.Listeners = []roverv1.RoverListener{
			{
				Consumer:    "eni--team--consumer",
				Provider:    "eni--team--provider",
				ApiBasePath: "/echo/v1",
			},
		}
		owner.Spec.ListenerSubscription = nil

		var capturedApp *spectrev1.SpectreApplication

		fakeClient.EXPECT().Scheme().Return(testScheme).Maybe()
		fakeClient.EXPECT().
			CreateOrUpdate(ctx, mock.AnythingOfType("*v1.SpectreApplication"), mock.AnythingOfType("controllerutil.MutateFn")).
			Run(func(_ context.Context, obj client.Object, mutate controllerutil.MutateFn) {
				_ = mutate()
				capturedApp = obj.(*spectrev1.SpectreApplication)
			}).
			Return(controllerutil.OperationResultCreated, nil).Once()
		fakeClient.EXPECT().
			CreateOrUpdate(ctx, mock.AnythingOfType("*v1.Listener"), mock.AnythingOfType("controllerutil.MutateFn")).
			Run(func(_ context.Context, _ client.Object, mutate controllerutil.MutateFn) {
				_ = mutate()
			}).
			Return(controllerutil.OperationResultCreated, nil).Once()

		err := spectre.HandleListeners(ctx, fakeClient, owner)

		Expect(err).ToNot(HaveOccurred())
		Expect(capturedApp.Spec.DeliveryType).To(Equal("server_sent_event"))
		Expect(capturedApp.Spec.Callback).To(BeEmpty())
	})

	It("should return error when SpectreApplication creation fails", func() {
		owner.Spec.Listeners = []roverv1.RoverListener{
			{
				Consumer:    "eni--team--consumer",
				Provider:    "eni--team--provider",
				ApiBasePath: "/echo/v1",
			},
		}

		fakeClient.EXPECT().Scheme().Return(testScheme).Maybe()
		fakeClient.EXPECT().
			CreateOrUpdate(ctx, mock.AnythingOfType("*v1.SpectreApplication"), mock.AnythingOfType("controllerutil.MutateFn")).
			Return(controllerutil.OperationResultNone, fmt.Errorf("api server error")).Once()

		err := spectre.HandleListeners(ctx, fakeClient, owner)

		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("failed to create or update SpectreApplication"))
	})

	It("should return error when Listener creation fails", func() {
		owner.Spec.Listeners = []roverv1.RoverListener{
			{
				Consumer:    "eni--team--consumer",
				Provider:    "eni--team--provider",
				ApiBasePath: "/echo/v1",
			},
		}

		fakeClient.EXPECT().Scheme().Return(testScheme).Maybe()
		fakeClient.EXPECT().
			CreateOrUpdate(ctx, mock.AnythingOfType("*v1.SpectreApplication"), mock.AnythingOfType("controllerutil.MutateFn")).
			Run(func(_ context.Context, _ client.Object, mutate controllerutil.MutateFn) {
				_ = mutate()
			}).
			Return(controllerutil.OperationResultCreated, nil).Once()
		fakeClient.EXPECT().
			CreateOrUpdate(ctx, mock.AnythingOfType("*v1.Listener"), mock.AnythingOfType("controllerutil.MutateFn")).
			Return(controllerutil.OperationResultNone, fmt.Errorf("api server error")).Once()

		err := spectre.HandleListeners(ctx, fakeClient, owner)

		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("failed to create or update Listener"))
	})
})
