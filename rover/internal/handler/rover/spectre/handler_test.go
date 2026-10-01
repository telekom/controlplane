// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package spectre_test

import (
	"context"
	"fmt"

	"github.com/stretchr/testify/mock"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	applicationv1 "github.com/telekom/controlplane/application/api/v1"
	cclient "github.com/telekom/controlplane/common/pkg/client"
	fakeclient "github.com/telekom/controlplane/common/pkg/client/fake"
	"github.com/telekom/controlplane/common/pkg/types"
	"github.com/telekom/controlplane/common/pkg/util/contextutil"
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
	_ = applicationv1.AddToScheme(s)
	_ = roverv1.AddToScheme(s)
	_ = spectrev1.AddToScheme(s)
	return s
}

// mockResolveApplication stubs a List call that resolveApplication makes to
// look up an Application by name. The returned Application has the given name,
// teamNamespace, and a deterministic UID.
func mockResolveApplication(fakeClient *fakeclient.MockJanitorClient, ctx context.Context, name string) {
	fakeClient.EXPECT().
		List(ctx, mock.AnythingOfType("*v1.ApplicationList"), mock.Anything).
		Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
			*list.(*applicationv1.ApplicationList) = applicationv1.ApplicationList{
				Items: []applicationv1.Application{{
					ObjectMeta: metav1.ObjectMeta{
						Name:      name,
						Namespace: teamNamespace,
						UID:       k8stypes.UID("app-uid-" + name),
					},
				}},
			}
		}).
		Return(nil).Once()
}

// mockUnresolvedApplication stubs a List call that finds no Application, so
// resolveApplication returns a BlockedError.
func mockUnresolvedApplication(fakeClient *fakeclient.MockJanitorClient, ctx context.Context) {
	fakeClient.EXPECT().
		List(ctx, mock.AnythingOfType("*v1.ApplicationList"), mock.Anything).
		Return(nil).Once()
}

// mockExistingListener stubs the Get for the Listener of a blocked entry. A nil
// existing Listener makes the Get return NotFound.
func mockExistingListener(fakeClient *fakeclient.MockJanitorClient, ctx context.Context, name string, existing *spectrev1.Listener) {
	call := fakeClient.EXPECT().
		Get(ctx, client.ObjectKey{Name: name, Namespace: teamNamespace}, mock.AnythingOfType("*v1.Listener"))
	if existing == nil {
		call.Return(apierrors.NewNotFound(spectrev1.GroupVersion.WithResource("listeners").GroupResource(), name)).Once()
		return
	}
	call.Run(func(_ context.Context, _ k8stypes.NamespacedName, obj client.Object, _ ...client.GetOption) {
		*obj.(*spectrev1.Listener) = *existing
	}).Return(nil).Once()
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
		ctx = contextutil.WithEnv(ctx, testEnvironment)
		testScheme = newTestScheme()
		owner = createTestOwner()
	})

	It("should clear status and return when listeners is empty", func() {
		owner.Spec.Listeners = nil

		err := spectre.HandleListeners(ctx, fakeClient, owner)

		Expect(err).ToNot(HaveOccurred())
		Expect(owner.Status.SpectreApplications).To(BeEmpty())
		Expect(owner.Status.SpectreListeners).To(BeEmpty())
	})

	It("should create one SpectreApplication and one Listener for a single listener entry", func() {
		owner.Spec.Listeners = []roverv1.RoverListener{
			{
				Consumer:    "consumer",
				Provider:    "provider",
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
		mockResolveApplication(fakeClient, ctx, "consumer")
		mockResolveApplication(fakeClient, ctx, "provider")
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
		Expect(capturedListener.Name).To(Equal("my-app--consumer---echo-v1"))
		Expect(capturedListener.Namespace).To(Equal(teamNamespace))
		Expect(capturedListener.Spec.Consumer.Name).To(Equal("consumer"))
		Expect(capturedListener.Spec.Consumer.Kind).To(Equal("Application"))
		Expect(capturedListener.Spec.Provider.Name).To(Equal("provider"))
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
		Expect(owner.Status.SpectreListeners[0].Name).To(Equal("my-app--consumer---echo-v1"))
	})

	It("should create two Listeners for two listener entries", func() {
		owner.Spec.Listeners = []roverv1.RoverListener{
			{
				Consumer:    "consumer1",
				Provider:    "provider1",
				ApiBasePath: "/api/v1",
			},
			{
				Consumer:  "consumer2",
				Provider:  "provider2",
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
		mockResolveApplication(fakeClient, ctx, "consumer1")
		mockResolveApplication(fakeClient, ctx, "provider1")
		mockResolveApplication(fakeClient, ctx, "consumer2")
		mockResolveApplication(fakeClient, ctx, "provider2")
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
		Expect(capturedListeners[0].Name).To(Equal("my-app--consumer1---api-v1"))
		Expect(capturedListeners[0].Spec.ApiListener).ToNot(BeNil())
		Expect(capturedListeners[0].Spec.ApiListener.ApiBasePath).To(Equal("/api/v1"))
		Expect(capturedListeners[0].Spec.EventListener).To(BeNil())

		Expect(capturedListeners[1].Name).To(Equal("my-app--consumer2--de.telekom.eni.test.v1"))
		Expect(capturedListeners[1].Spec.EventListener).ToNot(BeNil())
		Expect(capturedListeners[1].Spec.EventListener.EventType).To(Equal("de.telekom.eni.test.v1"))
		Expect(capturedListeners[1].Spec.EventListener.Filter).ToNot(BeNil())
		Expect(capturedListeners[1].Spec.EventListener.Filter.Payload).To(Equal([]string{"status"}))
		Expect(capturedListeners[1].Spec.ApiListener).To(BeNil())

		Expect(owner.Status.SpectreListeners).To(HaveLen(2))
	})

	It("should block when two listeners differ only by provider and keep processing the rest", func() {
		// The Listener name is derived from consumer + discriminator, so two
		// entries that differ only by provider would collapse into a single CR
		// and the second would silently overwrite the first. Surface it instead.
		owner.Spec.Listeners = []roverv1.RoverListener{
			{
				Consumer:    "consumer1",
				Provider:    "provider1",
				ApiBasePath: "/api/v1",
			},
			{
				Consumer:    "consumer1",
				Provider:    "provider2",
				ApiBasePath: "/api/v1",
			},
			{
				Consumer:    "consumer3",
				Provider:    "provider3",
				ApiBasePath: "/api/v3",
			},
		}

		fakeClient.EXPECT().Scheme().Return(testScheme).Maybe()
		fakeClient.EXPECT().
			CreateOrUpdate(ctx, mock.AnythingOfType("*v1.SpectreApplication"), mock.AnythingOfType("controllerutil.MutateFn")).
			Run(func(_ context.Context, _ client.Object, mutate controllerutil.MutateFn) {
				_ = mutate()
			}).
			Return(controllerutil.OperationResultCreated, nil).Once()
		mockResolveApplication(fakeClient, ctx, "consumer1")
		mockResolveApplication(fakeClient, ctx, "provider1")
		mockResolveApplication(fakeClient, ctx, "consumer3")
		mockResolveApplication(fakeClient, ctx, "provider3")
		fakeClient.EXPECT().
			CreateOrUpdate(ctx, mock.AnythingOfType("*v1.Listener"), mock.AnythingOfType("controllerutil.MutateFn")).
			Run(func(_ context.Context, _ client.Object, mutate controllerutil.MutateFn) {
				_ = mutate()
			}).
			Return(controllerutil.OperationResultCreated, nil).Times(2)

		err := spectre.HandleListeners(ctx, fakeClient, owner)

		Expect(err).To(HaveOccurred())
		Expect(err).To(Satisfy(isBlockedError))
		Expect(err.Error()).To(ContainSubstring("duplicate listener"))
		Expect(owner.Status.SpectreListeners).To(HaveLen(2))
		Expect(owner.Status.SpectreListeners[0].Name).To(Equal("my-app--consumer1---api-v1"))
		Expect(owner.Status.SpectreListeners[1].Name).To(Equal("my-app--consumer3---api-v3"))
	})

	It("should create the other Listeners and return BlockedError when one entry cannot be resolved", func() {
		owner.Spec.Listeners = []roverv1.RoverListener{
			{
				Consumer:    owner.Name,
				Provider:    "does-not-exist",
				ApiBasePath: "/missing/v1",
			},
			{
				Consumer:    owner.Name,
				Provider:    "provider",
				ApiBasePath: "/echo/v1",
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
		mockUnresolvedApplication(fakeClient, ctx)
		mockExistingListener(fakeClient, ctx, "my-app--my-app---missing-v1", nil)
		mockResolveApplication(fakeClient, ctx, "provider")
		fakeClient.EXPECT().
			CreateOrUpdate(ctx, mock.AnythingOfType("*v1.Listener"), mock.AnythingOfType("controllerutil.MutateFn")).
			Run(func(_ context.Context, obj client.Object, mutate controllerutil.MutateFn) {
				_ = mutate()
				capturedListeners = append(capturedListeners, obj.(*spectrev1.Listener))
			}).
			Return(controllerutil.OperationResultCreated, nil).Once()

		err := spectre.HandleListeners(ctx, fakeClient, owner)

		Expect(err).To(HaveOccurred())
		Expect(err).To(Satisfy(isBlockedError))
		Expect(err.Error()).To(ContainSubstring(`application "does-not-exist" not found`))
		Expect(capturedListeners).To(HaveLen(1))
		Expect(capturedListeners[0].Name).To(Equal("my-app--my-app---echo-v1"))
		Expect(owner.Status.SpectreApplications).To(HaveLen(1))
		Expect(owner.Status.SpectreListeners).To(HaveLen(1))
		Expect(owner.Status.SpectreListeners[0].Name).To(Equal("my-app--my-app---echo-v1"))
	})

	It("should keep the existing Listener of an entry that cannot be resolved", func() {
		owner.Spec.Listeners = []roverv1.RoverListener{
			{
				Consumer:    owner.Name,
				Provider:    "does-not-exist",
				ApiBasePath: "/echo/v1",
			},
		}
		existing := &spectrev1.Listener{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "my-app--my-app---echo-v1",
				Namespace: teamNamespace,
				UID:       "listener-uid-1",
			},
			Spec: spectrev1.ListenerSpec{
				Provider: types.TypedObjectRef{ObjectRef: types.ObjectRef{Name: "old-provider", Namespace: teamNamespace}},
			},
		}

		var keptListener *spectrev1.Listener

		fakeClient.EXPECT().Scheme().Return(testScheme).Maybe()
		fakeClient.EXPECT().
			CreateOrUpdate(ctx, mock.AnythingOfType("*v1.SpectreApplication"), mock.AnythingOfType("controllerutil.MutateFn")).
			Run(func(_ context.Context, _ client.Object, mutate controllerutil.MutateFn) {
				_ = mutate()
			}).
			Return(controllerutil.OperationResultCreated, nil).Once()
		mockUnresolvedApplication(fakeClient, ctx)
		mockExistingListener(fakeClient, ctx, existing.Name, existing)
		fakeClient.EXPECT().
			CreateOrUpdate(ctx, mock.AnythingOfType("*v1.Listener"), mock.AnythingOfType("controllerutil.MutateFn")).
			Run(func(_ context.Context, obj client.Object, mutate controllerutil.MutateFn) {
				Expect(mutate()).To(Succeed())
				keptListener = obj.(*spectrev1.Listener)
			}).
			Return(controllerutil.OperationResultNone, nil).Once()

		err := spectre.HandleListeners(ctx, fakeClient, owner)

		Expect(err).To(HaveOccurred())
		Expect(err).To(Satisfy(isBlockedError))
		Expect(keptListener).ToNot(BeNil())
		Expect(keptListener.UID).To(Equal(existing.UID))
		Expect(keptListener.Spec).To(Equal(existing.Spec))
		Expect(owner.Status.SpectreListeners).To(Equal([]types.ObjectRef{{Name: existing.Name, Namespace: teamNamespace}}))
	})

	It("should return a non-blocked error when the existing Listener of a blocked entry cannot be read", func() {
		owner.Spec.Listeners = []roverv1.RoverListener{
			{
				Consumer:    owner.Name,
				Provider:    "does-not-exist",
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
		mockUnresolvedApplication(fakeClient, ctx)
		fakeClient.EXPECT().
			Get(ctx, client.ObjectKey{Name: "my-app--my-app---echo-v1", Namespace: teamNamespace}, mock.AnythingOfType("*v1.Listener")).
			Return(fmt.Errorf("api server error")).Once()

		err := spectre.HandleListeners(ctx, fakeClient, owner)

		Expect(err).To(HaveOccurred())
		Expect(err).ToNot(Satisfy(isBlockedError))
		Expect(err.Error()).To(ContainSubstring("failed to get Listener"))
		Expect(owner.Status.SpectreListeners).To(BeEmpty())
	})

	It("should return a non-blocked error immediately", func() {
		owner.Spec.Listeners = []roverv1.RoverListener{
			{
				Consumer:    owner.Name,
				Provider:    "provider1",
				ApiBasePath: "/api/v1",
			},
			{
				Consumer:    owner.Name,
				Provider:    "provider2",
				ApiBasePath: "/api/v2",
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
			List(ctx, mock.AnythingOfType("*v1.ApplicationList"), mock.Anything).
			Return(fmt.Errorf("api server error")).Once()

		err := spectre.HandleListeners(ctx, fakeClient, owner)

		Expect(err).To(HaveOccurred())
		Expect(err).ToNot(Satisfy(isBlockedError))
		Expect(err.Error()).To(ContainSubstring("api server error"))
	})

	It("should set callback delivery type when listenerSubscription specifies callback", func() {
		owner.Spec.Listeners = []roverv1.RoverListener{
			{
				Consumer:    "consumer",
				Provider:    "provider",
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
		mockResolveApplication(fakeClient, ctx, "consumer")
		mockResolveApplication(fakeClient, ctx, "provider")
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
				Consumer:    "consumer",
				Provider:    "provider",
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
		mockResolveApplication(fakeClient, ctx, "consumer")
		mockResolveApplication(fakeClient, ctx, "provider")
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
				Consumer:    "consumer",
				Provider:    "provider",
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
				Consumer:    "consumer",
				Provider:    "provider",
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
		mockResolveApplication(fakeClient, ctx, "consumer")
		mockResolveApplication(fakeClient, ctx, "provider")
		fakeClient.EXPECT().
			CreateOrUpdate(ctx, mock.AnythingOfType("*v1.Listener"), mock.AnythingOfType("controllerutil.MutateFn")).
			Return(controllerutil.OperationResultNone, fmt.Errorf("api server error")).Once()

		err := spectre.HandleListeners(ctx, fakeClient, owner)

		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("failed to create or update Listener"))
	})

	It("should return a non-blocked error when status.application is nil", func() {
		owner.Spec.Listeners = []roverv1.RoverListener{
			{
				Consumer:    "consumer",
				Provider:    "provider",
				ApiBasePath: "/echo/v1",
			},
		}
		owner.Status.Application = nil

		err := spectre.HandleListeners(ctx, fakeClient, owner)

		Expect(err).To(HaveOccurred())
		Expect(err).ToNot(Satisfy(isBlockedError))
		Expect(err.Error()).To(ContainSubstring("rover status.application is not yet set"))
	})
})
