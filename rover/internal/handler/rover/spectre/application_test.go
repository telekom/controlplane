// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package spectre_test

import (
	"context"
	"errors"

	"github.com/stretchr/testify/mock"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	applicationv1 "github.com/telekom/controlplane/application/api/v1"
	cclient "github.com/telekom/controlplane/common/pkg/client"
	fakeclient "github.com/telekom/controlplane/common/pkg/client/fake"
	"github.com/telekom/controlplane/common/pkg/errors/ctrlerrors"
	"github.com/telekom/controlplane/common/pkg/util/contextutil"
	roverv1 "github.com/telekom/controlplane/rover/api/v1"
	"github.com/telekom/controlplane/rover/internal/handler/rover/spectre"
	spectrev1 "github.com/telekom/controlplane/spectre/api/v1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// isBlockedError checks if the error implements the BlockedError interface.
func isBlockedError(err error) bool {
	var be ctrlerrors.BlockedError
	ok := errors.As(err, &be)
	return ok && be.IsBlocked()
}

// makeApplicationInNs creates an Application with name, namespace, and UID.
func makeApplicationInNs(name, namespace string) applicationv1.Application {
	return applicationv1.Application{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			UID:       k8stypes.UID("uid-" + name),
		},
	}
}

// makeDeletingApplication creates an Application that has a deletion timestamp set.
func makeDeletingApplication(name, namespace string) applicationv1.Application {
	now := metav1.Now()
	return applicationv1.Application{
		ObjectMeta: metav1.ObjectMeta{
			Name:              name,
			Namespace:         namespace,
			UID:               k8stypes.UID("uid-" + name),
			DeletionTimestamp: &now,
			Finalizers:        []string{"test-finalizer"},
		},
	}
}

// exerciseResolveApplication triggers resolveApplication through HandleListeners
// by setting up a Rover with a single listener where the provider is the target.
// The consumer is resolved via a separate mock; the provider resolution exercises
// resolveApplication with the provided Application list result.
func exerciseResolveApplication(
	ctx context.Context,
	fakeClient *fakeclient.MockJanitorClient,
	providerName string,
	providerListResult []applicationv1.Application,
	providerListErr error,
) error {
	testScheme := newTestScheme()
	owner := createTestOwner()
	owner.Spec.Listeners = []roverv1.RoverListener{
		{
			Consumer:    owner.Name,
			Provider:    providerName,
			ApiBasePath: "/test/v1",
		},
	}

	fakeClient.EXPECT().Scheme().Return(testScheme).Maybe()
	// SpectreApplication creation
	fakeClient.EXPECT().
		CreateOrUpdate(ctx, mock.AnythingOfType("*v1.SpectreApplication"), mock.AnythingOfType("controllerutil.MutateFn")).
		Run(func(_ context.Context, _ client.Object, mutate controllerutil.MutateFn) {
			_ = mutate()
		}).
		Return(controllerutil.OperationResultCreated, nil).Maybe()
	// Listener creation
	fakeClient.EXPECT().
		CreateOrUpdate(ctx, mock.AnythingOfType("*v1.Listener"), mock.AnythingOfType("controllerutil.MutateFn")).
		Run(func(_ context.Context, _ client.Object, mutate controllerutil.MutateFn) {
			_ = mutate()
		}).
		Return(controllerutil.OperationResultCreated, nil).Maybe()

	// Provider resolution — this is the List call under test.
	fakeClient.EXPECT().
		List(ctx, mock.AnythingOfType("*v1.ApplicationList"), mock.Anything).
		Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
			*list.(*applicationv1.ApplicationList) = applicationv1.ApplicationList{Items: providerListResult}
		}).
		Return(providerListErr).Once()

	return spectre.HandleListeners(ctx, fakeClient, owner)
}

// exerciseResolveApplicationByID triggers resolveApplication through HandleListeners
// with the provider given as full application ID "eni--other-team--provider". It
// stubs the Get call for the expected key and returns the captured Listener.
func exerciseResolveApplicationByID(
	ctx context.Context,
	fakeClient *fakeclient.MockJanitorClient,
	expectedKey client.ObjectKey,
	getResult *applicationv1.Application,
	getErr error,
) (*spectrev1.Listener, error) {
	testScheme := newTestScheme()
	owner := createTestOwner()
	owner.Spec.Listeners = []roverv1.RoverListener{
		{
			Consumer:    owner.Name,
			Provider:    "eni--other-team--provider",
			ApiBasePath: "/test/v1",
		},
	}

	var capturedListener *spectrev1.Listener

	fakeClient.EXPECT().Scheme().Return(testScheme).Maybe()
	fakeClient.EXPECT().
		CreateOrUpdate(ctx, mock.AnythingOfType("*v1.SpectreApplication"), mock.AnythingOfType("controllerutil.MutateFn")).
		Run(func(_ context.Context, _ client.Object, mutate controllerutil.MutateFn) {
			_ = mutate()
		}).
		Return(controllerutil.OperationResultCreated, nil).Maybe()
	fakeClient.EXPECT().
		CreateOrUpdate(ctx, mock.AnythingOfType("*v1.Listener"), mock.AnythingOfType("controllerutil.MutateFn")).
		Run(func(_ context.Context, obj client.Object, mutate controllerutil.MutateFn) {
			_ = mutate()
			capturedListener = obj.(*spectrev1.Listener)
		}).
		Return(controllerutil.OperationResultCreated, nil).Maybe()

	fakeClient.EXPECT().
		Get(ctx, expectedKey, mock.AnythingOfType("*v1.Application")).
		Run(func(_ context.Context, _ k8stypes.NamespacedName, obj client.Object, _ ...client.GetOption) {
			if getResult != nil {
				*obj.(*applicationv1.Application) = *getResult
			}
		}).
		Return(getErr).Once()

	err := spectre.HandleListeners(ctx, fakeClient, owner)
	return capturedListener, err
}

var _ = Describe("resolveApplication", func() {
	var (
		ctx        context.Context
		fakeClient *fakeclient.MockJanitorClient
	)

	BeforeEach(func() {
		ctx = context.Background()
		fakeClient = fakeclient.NewMockJanitorClient(GinkgoT())
		ctx = cclient.WithClient(ctx, fakeClient)
		ctx = contextutil.WithEnv(ctx, testEnvironment)
	})

	It("should resolve a provider in another team namespace", func() {
		providerApp := makeApplicationInNs("provider", "test-env--eni--other-team")
		err := exerciseResolveApplication(ctx, fakeClient, "provider",
			[]applicationv1.Application{providerApp}, nil)
		Expect(err).ToNot(HaveOccurred())
	})

	It("should resolve a consumer through the Rover's own Application reference", func() {
		// When consumer == rover.Name, the handler uses the already-resolved
		// status ref instead of calling resolveApplication. Verify this path
		// works by checking that only ONE List call is made (for the provider).
		testScheme := newTestScheme()
		owner := createTestOwner()
		owner.Spec.Listeners = []roverv1.RoverListener{
			{
				Consumer:    owner.Name, // same as rover name
				Provider:    "provider",
				ApiBasePath: "/test/v1",
			},
		}

		var capturedListener *spectrev1.Listener

		fakeClient.EXPECT().Scheme().Return(testScheme).Maybe()
		fakeClient.EXPECT().
			CreateOrUpdate(ctx, mock.AnythingOfType("*v1.SpectreApplication"), mock.AnythingOfType("controllerutil.MutateFn")).
			Run(func(_ context.Context, _ client.Object, mutate controllerutil.MutateFn) {
				_ = mutate()
			}).
			Return(controllerutil.OperationResultCreated, nil).Once()
		// Only the provider triggers a List call
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
		Expect(capturedListener.Spec.Consumer.Name).To(Equal(owner.Name))
		Expect(capturedListener.Spec.Consumer.Namespace).To(Equal(teamNamespace))
		Expect(capturedListener.Spec.Consumer.UID).To(Equal(owner.Status.Application.UID))
	})

	It("should resolve when exactly one Application matches the name", func() {
		app := makeApplicationInNs("unique-app", teamNamespace)
		err := exerciseResolveApplication(ctx, fakeClient, "unique-app",
			[]applicationv1.Application{app}, nil)
		Expect(err).ToNot(HaveOccurred())
	})

	It("should return BlockedError when no Application matches", func() {
		err := exerciseResolveApplication(ctx, fakeClient, "nonexistent",
			[]applicationv1.Application{}, nil)
		Expect(err).To(HaveOccurred())
		Expect(err).To(Satisfy(isBlockedError))
		Expect(err.Error()).To(ContainSubstring("not found"))
	})

	It("should return BlockedError when multiple exact-name matches exist across namespaces", func() {
		app1 := makeApplicationInNs("ambiguous-app", "test-env--eni--team-a")
		app2 := makeApplicationInNs("ambiguous-app", "test-env--eni--team-b")
		err := exerciseResolveApplication(ctx, fakeClient, "ambiguous-app",
			[]applicationv1.Application{app1, app2}, nil)
		Expect(err).To(HaveOccurred())
		Expect(err).To(Satisfy(isBlockedError))
		Expect(err.Error()).To(ContainSubstring("ambiguous"))
	})

	It("should not match when normalized label collides but Application.Name differs", func() {
		// NormalizeValue("foo/bar") == NormalizeValue("foo-bar") == "foo-bar",
		// so the label index returns both, but resolveApplication filters by
		// exact Application.Name match.
		wrongName := makeApplicationInNs("foo-bar", teamNamespace)
		err := exerciseResolveApplication(ctx, fakeClient, "foo/bar",
			[]applicationv1.Application{wrongName}, nil)
		Expect(err).To(HaveOccurred())
		Expect(err).To(Satisfy(isBlockedError))
		Expect(err.Error()).To(ContainSubstring("not found"))
	})

	It("should not select Applications that are being deleted", func() {
		deletingApp := makeDeletingApplication("dying-app", teamNamespace)
		err := exerciseResolveApplication(ctx, fakeClient, "dying-app",
			[]applicationv1.Application{deletingApp}, nil)
		Expect(err).To(HaveOccurred())
		Expect(err).To(Satisfy(isBlockedError))
		Expect(err.Error()).To(ContainSubstring("not found"))
	})

	It("should look up a bare name with two parts by label", func() {
		app := makeApplicationInNs("team--app", teamNamespace)
		err := exerciseResolveApplication(ctx, fakeClient, "team--app",
			[]applicationv1.Application{app}, nil)
		Expect(err).ToNot(HaveOccurred())
	})

	Context("with a full application ID", func() {
		providerKey := client.ObjectKey{Name: "provider", Namespace: testEnvironment + "--eni--other-team"}

		It("should get the Application from the team namespace of the ID", func() {
			providerApp := makeApplicationInNs("provider", providerKey.Namespace)
			listener, err := exerciseResolveApplicationByID(ctx, fakeClient,
				providerKey, &providerApp, nil)
			Expect(err).ToNot(HaveOccurred())
			Expect(listener).ToNot(BeNil())
			Expect(listener.Spec.Provider.Name).To(Equal("provider"))
			Expect(listener.Spec.Provider.Namespace).To(Equal(providerKey.Namespace))
			Expect(listener.Spec.Provider.UID).To(Equal(providerApp.UID))
		})

		It("should return BlockedError when the Application does not exist in the team", func() {
			notFound := apierrors.NewNotFound(applicationv1.GroupVersion.WithResource("applications").GroupResource(), "provider")
			_, err := exerciseResolveApplicationByID(ctx, fakeClient,
				providerKey, nil, notFound)
			Expect(err).To(HaveOccurred())
			Expect(err).To(Satisfy(isBlockedError))
			Expect(err.Error()).To(ContainSubstring("not found in team"))
		})

		It("should return BlockedError when the Application is being deleted", func() {
			deletingApp := makeDeletingApplication("provider", providerKey.Namespace)
			_, err := exerciseResolveApplicationByID(ctx, fakeClient,
				providerKey, &deletingApp, nil)
			Expect(err).To(HaveOccurred())
			Expect(err).To(Satisfy(isBlockedError))
			Expect(err.Error()).To(ContainSubstring("not found in team"))
		})

		It("should return a non-blocked error when the Get fails", func() {
			_, err := exerciseResolveApplicationByID(ctx, fakeClient,
				providerKey, nil, errors.New("api server error"))
			Expect(err).To(HaveOccurred())
			Expect(err).ToNot(Satisfy(isBlockedError))
			Expect(err.Error()).To(ContainSubstring("api server error"))
		})

		It("should use the Rover's own Application reference for its own full ID", func() {
			testScheme := newTestScheme()
			owner := createTestOwner()
			owner.Spec.Listeners = []roverv1.RoverListener{
				{
					Consumer:    "eni--pandora--" + owner.Name,
					Provider:    "provider",
					ApiBasePath: "/test/v1",
				},
			}

			var capturedListener *spectrev1.Listener

			fakeClient.EXPECT().Scheme().Return(testScheme).Maybe()
			fakeClient.EXPECT().
				CreateOrUpdate(ctx, mock.AnythingOfType("*v1.SpectreApplication"), mock.AnythingOfType("controllerutil.MutateFn")).
				Run(func(_ context.Context, _ client.Object, mutate controllerutil.MutateFn) {
					_ = mutate()
				}).
				Return(controllerutil.OperationResultCreated, nil).Once()
			// Only the provider is resolved; the consumer needs neither Get nor List.
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
			Expect(capturedListener.Spec.Consumer.Name).To(Equal(owner.Name))
			Expect(capturedListener.Spec.Consumer.Namespace).To(Equal(teamNamespace))
		})
	})
})
