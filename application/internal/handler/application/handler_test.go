// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package application

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/mock"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	pkgclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	applicationv1 "github.com/telekom/controlplane/application/api/v1"
	"github.com/telekom/controlplane/application/internal/secret"
	"github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/client/fake"
	"github.com/telekom/controlplane/common/pkg/condition"
	commontypes "github.com/telekom/controlplane/common/pkg/types"
	"github.com/telekom/controlplane/common/pkg/util/contextutil"
	identityv1 "github.com/telekom/controlplane/identity/api/v1"
)

func newTestApp() *applicationv1.Application {
	return &applicationv1.Application{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-app",
			Namespace: "test-ns",
			UID:       "test-uid",
		},
		Spec: applicationv1.ApplicationSpec{
			Team:      "test-team",
			TeamEmail: "team@example.com",
			Secret:    "$<ref:secret>",
			Zone: commontypes.ObjectRef{
				Name:      "test-zone",
				Namespace: "test-ns",
			},
			NeedsClient:   true,
			NeedsConsumer: true,
		},
	}
}

func newZone() *adminv1.Zone {
	return &adminv1.Zone{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-zone",
			Namespace: "test-ns",
		},
		Status: adminv1.ZoneStatus{
			Namespace: "zone-ns",
		},
	}
}

func newScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	_ = applicationv1.AddToScheme(s)
	_ = identityv1.AddToScheme(s)
	_ = adminv1.AddToScheme(s)
	return s
}

// setupHappyPath configures the mock for a full CreateOrUpdate without rotation.
func setupHappyPath(mockClient *fake.MockJanitorClient, app *applicationv1.Application, zone *adminv1.Zone, anyChanged bool) {
	scheme := newScheme()

	// AddKnownTypeToState calls
	mockClient.EXPECT().AddKnownTypeToState(mock.Anything).Maybe()

	// Get zone
	mockClient.EXPECT().
		Get(mock.Anything, types.NamespacedName{Name: zone.Name, Namespace: zone.Namespace}, mock.AnythingOfType("*v1.Zone"), mock.Anything).
		Run(func(_ context.Context, _ types.NamespacedName, obj pkgclient.Object, _ ...pkgclient.GetOption) {
			*obj.(*adminv1.Zone) = *zone
		}).
		Return(nil)

	// Scheme for SetControllerReference
	mockClient.EXPECT().Scheme().Return(scheme).Maybe()

	// CreateOrUpdate for identity client
	mockClient.EXPECT().
		CreateOrUpdate(mock.Anything, mock.AnythingOfType("*v1.Client"), mock.Anything).
		Run(func(_ context.Context, obj pkgclient.Object, fn controllerutil.MutateFn) {
			_ = fn()
		}).
		Return(controllerutil.OperationResultCreated, nil)

	// CreateOrUpdate for gateway consumer
	mockClient.EXPECT().
		CreateOrUpdate(mock.Anything, mock.AnythingOfType("*v1.Consumer"), mock.Anything).
		Run(func(_ context.Context, obj pkgclient.Object, fn controllerutil.MutateFn) {
			_ = fn()
		}).
		Return(controllerutil.OperationResultCreated, nil)

	// CleanupAll
	mockClient.EXPECT().
		CleanupAll(mock.Anything, mock.Anything).
		Return(0, nil)

	// AnyChanged
	mockClient.EXPECT().AnyChanged().Return(anyChanged)
}

var _ = Describe("ApplicationHandler - Secret Rotation", func() {
	var (
		ctx     context.Context
		handler *ApplicationHandler
		app     *applicationv1.Application
		zone    *adminv1.Zone
	)

	BeforeEach(func() {
		ctx = context.Background()
		ctx = contextutil.WithEnv(ctx, "test-env")
		handler = &ApplicationHandler{}
		app = newTestApp()
		zone = newZone()
	})

	Describe("CreateOrUpdate", func() {
		Context("without rotation (spec.rotatedSecret is empty)", func() {
			It("should not set SecretRotation condition when no rotation requested", func() {
				mockClient := fake.NewMockJanitorClient(GinkgoT())
				ctx = client.WithClient(ctx, mockClient)
				setupHappyPath(mockClient, app, zone, false)

				err := handler.CreateOrUpdate(ctx, app)
				Expect(err).ToNot(HaveOccurred())

				cond := meta.FindStatusCondition(app.Status.Conditions, secret.SecretRotationConditionType)
				Expect(cond).To(BeNil(), "SecretRotation condition should not be set when no rotation is requested")
			})

			It("should set Ready condition when sub-resources are up to date", func() {
				mockClient := fake.NewMockJanitorClient(GinkgoT())
				ctx = client.WithClient(ctx, mockClient)
				setupHappyPath(mockClient, app, zone, false)

				err := handler.CreateOrUpdate(ctx, app)
				Expect(err).ToNot(HaveOccurred())

				readyCond := meta.FindStatusCondition(app.Status.Conditions, condition.ConditionTypeReady)
				Expect(readyCond).ToNot(BeNil())
				Expect(readyCond.Status).To(Equal(metav1.ConditionTrue))
			})

			It("should set Status.ClientSecret only after convergence", func() {
				mockClient := fake.NewMockJanitorClient(GinkgoT())
				ctx = client.WithClient(ctx, mockClient)
				setupHappyPath(mockClient, app, zone, false)

				err := handler.CreateOrUpdate(ctx, app)
				Expect(err).ToNot(HaveOccurred())

				Expect(app.Status.ClientSecret).To(Equal(app.Spec.Secret))
			})

			It("should not update Status.ClientSecret before convergence", func() {
				mockClient := fake.NewMockJanitorClient(GinkgoT())
				ctx = client.WithClient(ctx, mockClient)
				setupHappyPath(mockClient, app, zone, true)

				app.Status.ClientSecret = "$<previous-ref>"

				err := handler.CreateOrUpdate(ctx, app)
				Expect(err).ToNot(HaveOccurred())

				Expect(app.Status.ClientSecret).To(Equal("$<previous-ref>"),
					"Status.ClientSecret should retain its previous value until sub-resources converge")
			})

			It("should set Processing condition when sub-resources changed", func() {
				mockClient := fake.NewMockJanitorClient(GinkgoT())
				ctx = client.WithClient(ctx, mockClient)
				setupHappyPath(mockClient, app, zone, true)

				err := handler.CreateOrUpdate(ctx, app)
				Expect(err).ToNot(HaveOccurred())

				processingCond := meta.FindStatusCondition(app.Status.Conditions, condition.ConditionTypeProcessing)
				Expect(processingCond).ToNot(BeNil())
				Expect(processingCond.Status).To(Equal(metav1.ConditionTrue))
			})
		})

		Context("with rotation (spec.rotatedSecret is set)", func() {
			BeforeEach(func() {
				app.Spec.RotatedSecret = "$<ref:rotated-secret>"
			})

			It("should set SecretRotation condition to InProgress on first reconcile", func() {
				mockClient := fake.NewMockJanitorClient(GinkgoT())
				ctx = client.WithClient(ctx, mockClient)
				setupHappyPath(mockClient, app, zone, true) // changed=true on first reconcile

				err := handler.CreateOrUpdate(ctx, app)
				Expect(err).ToNot(HaveOccurred())

				cond := meta.FindStatusCondition(app.Status.Conditions, secret.SecretRotationConditionType)
				Expect(cond).ToNot(BeNil())
				Expect(cond.Reason).To(Equal(secret.SecretRotationReasonInProgress))
				Expect(cond.Status).To(Equal(metav1.ConditionFalse))
			})

			It("should not copy spec.rotatedSecret to status during InProgress (before convergence)", func() {
				mockClient := fake.NewMockJanitorClient(GinkgoT())
				ctx = client.WithClient(ctx, mockClient)
				setupHappyPath(mockClient, app, zone, true)

				err := handler.CreateOrUpdate(ctx, app)
				Expect(err).ToNot(HaveOccurred())

				Expect(app.Status.RotatedClientSecret).To(BeEmpty(),
					"status.rotatedClientSecret should not be set until sub-resources converge")
			})

			It("should transition to Success when sub-resources settle (AnyChanged=false)", func() {
				mockClient := fake.NewMockJanitorClient(GinkgoT())
				ctx = client.WithClient(ctx, mockClient)
				setupHappyPath(mockClient, app, zone, false)

				// Simulate: condition already InProgress from previous reconcile
				app.SetCondition(metav1.Condition{
					Type:    secret.SecretRotationConditionType,
					Status:  metav1.ConditionFalse,
					Reason:  secret.SecretRotationReasonInProgress,
					Message: "Secret rotation initiated",
				})

				// Mock Get for identity client status propagation
				mockClient.EXPECT().
					Get(mock.Anything, mock.Anything, mock.AnythingOfType("*v1.Client"), mock.Anything).
					Run(func(_ context.Context, _ types.NamespacedName, obj pkgclient.Object, _ ...pkgclient.GetOption) {
						idpClient := obj.(*identityv1.Client)
						now := metav1.NewTime(time.Now().Add(24 * time.Hour))
						expires := metav1.NewTime(time.Now().Add(48 * time.Hour))
						idpClient.Status.RotatedSecretExpiresAt = &now
						idpClient.Status.SecretExpiresAt = &expires
					}).
					Return(nil).Maybe()

				err := handler.CreateOrUpdate(ctx, app)
				Expect(err).ToNot(HaveOccurred())

				cond := meta.FindStatusCondition(app.Status.Conditions, secret.SecretRotationConditionType)
				Expect(cond).ToNot(BeNil())
				Expect(cond.Reason).To(Equal(secret.SecretRotationReasonSuccess))
				Expect(cond.Status).To(Equal(metav1.ConditionTrue))

				// Status fields should only be set after convergence
				Expect(app.Status.RotatedClientSecret).To(Equal("$<ref:rotated-secret>"))
			})

			It("should not re-set InProgress if already InProgress", func() {
				mockClient := fake.NewMockJanitorClient(GinkgoT())
				ctx = client.WithClient(ctx, mockClient)
				setupHappyPath(mockClient, app, zone, true) // still changing

				// Already InProgress
				app.SetCondition(metav1.Condition{
					Type:    secret.SecretRotationConditionType,
					Status:  metav1.ConditionFalse,
					Reason:  secret.SecretRotationReasonInProgress,
					Message: "Secret rotation initiated",
				})

				err := handler.CreateOrUpdate(ctx, app)
				Expect(err).ToNot(HaveOccurred())

				cond := meta.FindStatusCondition(app.Status.Conditions, secret.SecretRotationConditionType)
				Expect(cond).ToNot(BeNil())
				Expect(cond.Reason).To(Equal(secret.SecretRotationReasonInProgress))
				// Should still be InProgress since AnyChanged=true
			})

			It("should propagate expiry timestamps from identity client on Success", func() {
				mockClient := fake.NewMockJanitorClient(GinkgoT())
				ctx = client.WithClient(ctx, mockClient)
				setupHappyPath(mockClient, app, zone, false)

				// Simulate InProgress
				app.SetCondition(metav1.Condition{
					Type:    secret.SecretRotationConditionType,
					Status:  metav1.ConditionFalse,
					Reason:  secret.SecretRotationReasonInProgress,
					Message: "Secret rotation initiated",
				})

				// The identity client ref was set during CreateIdentityClient
				// propagateRotationStatus will Get the client refs from status
				rotatedExpires := metav1.NewTime(time.Now().Add(24 * time.Hour))
				currentExpires := metav1.NewTime(time.Now().Add(48 * time.Hour))

				mockClient.EXPECT().
					Get(mock.Anything, mock.Anything, mock.AnythingOfType("*v1.Client"), mock.Anything).
					Run(func(_ context.Context, _ types.NamespacedName, obj pkgclient.Object, _ ...pkgclient.GetOption) {
						idpClient := obj.(*identityv1.Client)
						idpClient.Status.RotatedSecretExpiresAt = &rotatedExpires
						idpClient.Status.SecretExpiresAt = &currentExpires
					}).
					Return(nil).Maybe()

				err := handler.CreateOrUpdate(ctx, app)
				Expect(err).ToNot(HaveOccurred())

				Expect(app.Status.RotatedExpiresAt).ToNot(BeNil())
				Expect(app.Status.RotatedExpiresAt.Time).To(BeTemporally("~", rotatedExpires.Time, time.Second))
				Expect(app.Status.CurrentExpiresAt).ToNot(BeNil())
				Expect(app.Status.CurrentExpiresAt.Time).To(BeTemporally("~", currentExpires.Time, time.Second))
			})

			It("should not re-initiate rotation after Success when spec.rotatedSecret matches status", func() {
				mockClient := fake.NewMockJanitorClient(GinkgoT())
				ctx = client.WithClient(ctx, mockClient)
				setupHappyPath(mockClient, app, zone, false)

				// Simulate completed rotation: condition=Success, spec and status match
				app.Status.RotatedClientSecret = app.Spec.RotatedSecret
				app.SetCondition(metav1.Condition{
					Type:    secret.SecretRotationConditionType,
					Status:  metav1.ConditionTrue,
					Reason:  secret.SecretRotationReasonSuccess,
					Message: "Secret rotation completed successfully",
				})

				err := handler.CreateOrUpdate(ctx, app)
				Expect(err).ToNot(HaveOccurred())

				cond := meta.FindStatusCondition(app.Status.Conditions, secret.SecretRotationConditionType)
				Expect(cond).ToNot(BeNil())
				// Should remain Success, NOT go back to InProgress
				Expect(cond.Reason).To(Equal(secret.SecretRotationReasonSuccess))
				Expect(cond.Status).To(Equal(metav1.ConditionTrue))
			})
		})
	})
})
