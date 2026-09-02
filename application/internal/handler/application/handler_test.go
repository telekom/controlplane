// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package application

import (
	"context"
	"errors"
	"strings"
	"time"

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
	"github.com/telekom/controlplane/common/pkg/errors/ctrlerrors"
	commontypes "github.com/telekom/controlplane/common/pkg/types"
	"github.com/telekom/controlplane/common/pkg/util/contextutil"
	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
	identityv1 "github.com/telekom/controlplane/identity/api/v1"
	notificationv1 "github.com/telekom/controlplane/notification/api/v1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
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
		Spec: adminv1.ZoneSpec{
			Gateways: []adminv1.GatewayConfig{{Name: "standard", Types: []adminv1.GatewayType{adminv1.GatewayTypeAPI}}},
			Presets:  []adminv1.Preset{{Name: "default", Default: true, GatewayRef: "standard"}},
		},
		Status: adminv1.ZoneStatus{
			Namespace: "zone-ns",
			Gateways: []adminv1.GatewayStatus{{
				Name:    "standard",
				Gateway: &commontypes.ObjectRef{Name: "test-gateway", Namespace: "zone-ns"},
			}},
			Presets: []adminv1.PresetStatus{{Name: "default", Links: adminv1.Links{TokenUrl: "https://identity.example.com/token"}, GatewayRef: &commontypes.ObjectRef{
				Name:      "test-gateway",
				Namespace: "zone-ns",
			}}},
			IdentityRealm: &commontypes.ObjectRef{
				Name:      "test-env",
				Namespace: "zone-ns",
			},
		},
	}
}

func newScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	_ = applicationv1.AddToScheme(s)
	_ = identityv1.AddToScheme(s)
	_ = adminv1.AddToScheme(s)
	_ = notificationv1.AddToScheme(s)
	return s
}

// identityClientMutator is an optional function that populates identity client
// status fields during CreateOrUpdate. Used to simulate a converged identity client
// that has been reconciled by the identity controller.
type identityClientMutator func(idpClient *identityv1.Client)

// setupHappyPath configures the mock for a full CreateOrUpdate.
// The optional idpMutator is called during the identity client CreateOrUpdate to
// populate status fields (e.g., expiry timestamps) on the idpClient object.
// When idpMutators are provided, the identity client CreateOrUpdate returns
// OperationResultNone (simulating a converged, unchanged client whose status is
// up to date). Otherwise it returns OperationResultCreated.
func setupHappyPath(mockClient *fake.MockJanitorClient, zone *adminv1.Zone, anyChanged bool, idpMutators ...identityClientMutator) {
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
	idpResult := controllerutil.OperationResultCreated
	if len(idpMutators) > 0 {
		idpResult = controllerutil.OperationResultNone
	}
	mockClient.EXPECT().
		CreateOrUpdate(mock.Anything, mock.AnythingOfType("*v1.Client"), mock.Anything).
		Run(func(_ context.Context, obj pkgclient.Object, fn controllerutil.MutateFn) {
			_ = fn()
			if idpClient, ok := obj.(*identityv1.Client); ok {
				// When converged (anyChanged=false), simulate a ready identity client
				// so the readiness gate in the handler is satisfied.
				if !anyChanged {
					idpClient.SetCondition(metav1.Condition{
						Type:   condition.ConditionTypeReady,
						Status: metav1.ConditionTrue,
						Reason: "Ready",
					})
				}
				// Apply optional identity client status mutators (simulates converged identity client)
				for _, m := range idpMutators {
					m(idpClient)
				}
			}
		}).
		Return(idpResult, nil)

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

	// When converged (anyChanged=false), notifications may be sent
	if !anyChanged {
		// List notification channels (called by builder.WithDefaultChannels)
		mockClient.EXPECT().
			List(mock.Anything, mock.AnythingOfType("*v1.NotificationChannelList"), mock.Anything).
			Return(nil).Maybe()

		// CreateOrUpdate for notification (called by builder.Send)
		mockClient.EXPECT().
			CreateOrUpdate(mock.Anything, mock.AnythingOfType("*v1.Notification"), mock.Anything).
			Return(controllerutil.OperationResultCreated, nil).Maybe()
	}
}

var _ = Describe("ApplicationHandler - Token URL", func() {
	var (
		ctx     context.Context
		handler *ApplicationHandler
		app     *applicationv1.Application
		zone    *adminv1.Zone
	)

	BeforeEach(func() {
		ctx = contextutil.WithEnv(context.Background(), "test-env")
		handler = &ApplicationHandler{}
		app = newTestApp()
		zone = newZone()
	})

	It("publishes the default preset token URL", func() {
		mockClient := fake.NewMockJanitorClient(GinkgoT())
		ctx = client.WithClient(ctx, mockClient)
		setupHappyPath(mockClient, zone, false)

		Expect(handler.CreateOrUpdate(ctx, app)).To(Succeed())
		Expect(app.Status.TokenUrl).To(Equal("https://identity.example.com/token"))
	})

	It("publishes the shared failover token URL when several presets match", func() {
		app.Spec.Failover.Enabled = true
		zone.Spec.Presets = append(zone.Spec.Presets,
			adminv1.Preset{Name: "api-failover", GatewayRef: "standard", IdentityProviderRef: "primary", Features: []adminv1.Feature{{Name: adminv1.FeatureConsumerFailover, Enabled: true}}},
			adminv1.Preset{Name: "ai-failover", GatewayRef: "standard", IdentityProviderRef: "primary", Features: []adminv1.Feature{{Name: adminv1.FeatureConsumerFailover, Enabled: true}}},
		)
		zone.Status.Presets = append(zone.Status.Presets,
			adminv1.PresetStatus{Name: "api-failover", Links: adminv1.Links{TokenUrl: "https://failover-identity.example.com/token"}},
			adminv1.PresetStatus{Name: "ai-failover", Links: adminv1.Links{TokenUrl: "https://failover-identity.example.com/token"}},
		)

		mockClient := fake.NewMockJanitorClient(GinkgoT())
		ctx = client.WithClient(ctx, mockClient)
		setupHappyPath(mockClient, zone, false)
		mockClient.EXPECT().List(mock.Anything, mock.AnythingOfType("*v1.ZoneList")).Return(nil)

		Expect(handler.CreateOrUpdate(ctx, app)).To(Succeed())
		Expect(app.Status.TokenUrl).To(Equal("https://failover-identity.example.com/token"))
	})

	Describe("consumerFailoverTokenURL", func() {
		BeforeEach(func() {
			zone.Spec.Presets = append(zone.Spec.Presets,
				adminv1.Preset{Name: "api-failover", GatewayRef: "standard", Features: []adminv1.Feature{{Name: adminv1.FeatureConsumerFailover, Enabled: true}}},
				adminv1.Preset{Name: "ai-failover", GatewayRef: "standard", Features: []adminv1.Feature{{Name: adminv1.FeatureConsumerFailover, Enabled: true}}},
			)
			zone.Status.Presets = append(zone.Status.Presets,
				adminv1.PresetStatus{Name: "api-failover", Links: adminv1.Links{TokenUrl: "https://failover.example.com/token"}},
				adminv1.PresetStatus{Name: "ai-failover", Links: adminv1.Links{TokenUrl: "https://failover.example.com/token"}},
			)
		})

		It("rejects a missing preset status", func() {
			zone.Status.Presets = zone.Status.Presets[:2]

			_, err := consumerFailoverTokenURL(zone)

			Expect(err).To(MatchError(`status for ConsumerFailover preset "ai-failover" is missing`))
		})

		It("rejects an empty token URL", func() {
			zone.Status.Presets[1].Links.TokenUrl = ""

			_, err := consumerFailoverTokenURL(zone)

			Expect(err).To(MatchError(`ConsumerFailover preset "api-failover" does not contain a token URL`))
		})

		It("rejects conflicting reconciled token URLs", func() {
			zone.Status.Presets[2].Links.TokenUrl = "https://other.example.com/token"

			_, err := consumerFailoverTokenURL(zone)

			Expect(err).To(MatchError(`ConsumerFailover presets resolve to conflicting token URLs`))
		})
	})

	It("blocks when the selected preset status is missing", func() {
		zone.Status.Presets = nil
		mockClient := fake.NewMockJanitorClient(GinkgoT())
		ctx = client.WithClient(ctx, mockClient)
		mockClient.EXPECT().AddKnownTypeToState(mock.Anything).Maybe()
		mockClient.EXPECT().Get(mock.Anything, types.NamespacedName{Name: zone.Name, Namespace: zone.Namespace}, mock.AnythingOfType("*v1.Zone"), mock.Anything).
			Run(func(_ context.Context, _ types.NamespacedName, obj pkgclient.Object, _ ...pkgclient.GetOption) {
				*obj.(*adminv1.Zone) = *zone
			}).Return(nil)

		err := handler.CreateOrUpdate(ctx, app)
		var blockedErr ctrlerrors.BlockedError
		Expect(errors.As(err, &blockedErr)).To(BeTrue())
		Expect(err).To(MatchError(`zone "test-zone" does not contain status for preset "default"`))
	})

	It("blocks when the selected preset token URL is empty", func() {
		zone.Status.Presets[0].Links.TokenUrl = ""
		mockClient := fake.NewMockJanitorClient(GinkgoT())
		ctx = client.WithClient(ctx, mockClient)
		mockClient.EXPECT().AddKnownTypeToState(mock.Anything).Maybe()
		mockClient.EXPECT().Get(mock.Anything, types.NamespacedName{Name: zone.Name, Namespace: zone.Namespace}, mock.AnythingOfType("*v1.Zone"), mock.Anything).
			Run(func(_ context.Context, _ types.NamespacedName, obj pkgclient.Object, _ ...pkgclient.GetOption) {
				*obj.(*adminv1.Zone) = *zone
			}).Return(nil)

		err := handler.CreateOrUpdate(ctx, app)
		var blockedErr ctrlerrors.BlockedError
		Expect(errors.As(err, &blockedErr)).To(BeTrue())
		Expect(err).To(MatchError(`zone "test-zone" preset "default" does not contain a token URL`))
	})
})

var _ = Describe("ApplicationHandler - Zone resolution", func() {
	var (
		ctx        context.Context
		mockClient *fake.MockJanitorClient
		app        *applicationv1.Application
		primary    *adminv1.Zone
	)

	failoverZone := func(namespace, name string, gatewayType adminv1.GatewayType, enabled bool) adminv1.Zone {
		return adminv1.Zone{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
			Spec: adminv1.ZoneSpec{
				Gateways: []adminv1.GatewayConfig{{Name: "gateway", Types: []adminv1.GatewayType{gatewayType}}},
				Presets:  []adminv1.Preset{{Name: "failover", GatewayRef: "gateway", Features: []adminv1.Feature{{Name: adminv1.FeatureConsumerFailover, Enabled: enabled}}}},
			},
		}
	}

	BeforeEach(func() {
		ctx = contextutil.WithEnv(context.Background(), "test-env")
		mockClient = fake.NewMockJanitorClient(GinkgoT())
		ctx = client.WithClient(ctx, mockClient)
		app = newTestApp()
		app.Spec.Failover.Enabled = true
		primary = newZone()

		mockClient.EXPECT().Get(mock.Anything, types.NamespacedName{Name: primary.Name, Namespace: primary.Namespace}, mock.AnythingOfType("*v1.Zone"), mock.Anything).
			Run(func(_ context.Context, _ types.NamespacedName, obj pkgclient.Object, _ ...pkgclient.GetOption) {
				*obj.(*adminv1.Zone) = *primary
			}).Return(nil)
	})

	It("returns API-only and AI-only failover zones in namespace and name order", func() {
		apiZone := failoverZone("z-ns", "api", adminv1.GatewayTypeAPI, true)
		aiZone := failoverZone("a-ns", "ai", adminv1.GatewayTypeAI, true)
		disabledZone := failoverZone("a-ns", "disabled", adminv1.GatewayTypeAPI, false)

		mockClient.EXPECT().List(mock.Anything, mock.AnythingOfType("*v1.ZoneList")).
			Run(func(_ context.Context, list pkgclient.ObjectList, _ ...pkgclient.ListOption) {
				list.(*adminv1.ZoneList).Items = []adminv1.Zone{apiZone, *primary, disabledZone, aiZone}
			}).Return(nil)

		zone, failoverZones, err := (&ApplicationHandler{}).resolveZones(ctx, mockClient, app)

		Expect(err).NotTo(HaveOccurred())
		Expect(zone).To(Equal(primary))
		Expect(failoverZones).To(Equal([]*adminv1.Zone{&aiZone, &apiZone}))
	})

	It("rejects a failover zone with a broken gateway reference", func() {
		brokenZone := failoverZone("broken-ns", "broken", adminv1.GatewayTypeAPI, true)
		brokenZone.Spec.Presets[0].GatewayRef = "missing"
		mockClient.EXPECT().List(mock.Anything, mock.AnythingOfType("*v1.ZoneList")).
			Run(func(_ context.Context, list pkgclient.ObjectList, _ ...pkgclient.ListOption) {
				list.(*adminv1.ZoneList).Items = []adminv1.Zone{brokenZone}
			}).Return(nil)

		_, _, err := (&ApplicationHandler{}).resolveZones(ctx, mockClient, app)

		var blockedErr ctrlerrors.BlockedError
		Expect(errors.As(err, &blockedErr)).To(BeTrue())
		Expect(err).To(MatchError(ContainSubstring(`failover zone "broken-ns/broken" is invalid`)))
		Expect(err).To(MatchError(ContainSubstring(`gateway "missing" not found`)))
	})

	It("excludes failover zones being deleted", func() {
		activeZone := failoverZone("active-ns", "active", adminv1.GatewayTypeAPI, true)
		deletingZone := failoverZone("deleting-ns", "deleting", adminv1.GatewayTypeAI, true)
		deletionTime := metav1.Now()
		deletingZone.DeletionTimestamp = &deletionTime
		mockClient.EXPECT().List(mock.Anything, mock.AnythingOfType("*v1.ZoneList")).
			Run(func(_ context.Context, list pkgclient.ObjectList, _ ...pkgclient.ListOption) {
				list.(*adminv1.ZoneList).Items = []adminv1.Zone{deletingZone, activeZone}
			}).Return(nil)

		_, failoverZones, err := (&ApplicationHandler{}).resolveZones(ctx, mockClient, app)

		Expect(err).NotTo(HaveOccurred())
		Expect(failoverZones).To(Equal([]*adminv1.Zone{&activeZone}))
	})
})

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
				setupHappyPath(mockClient, zone, false)

				err := handler.CreateOrUpdate(ctx, app)
				Expect(err).ToNot(HaveOccurred())

				cond := meta.FindStatusCondition(app.Status.Conditions, secret.SecretRotationConditionType)
				Expect(cond).To(BeNil(), "SecretRotation condition should not be set when no rotation is requested")
			})

			It("should set Ready condition when sub-resources are up to date", func() {
				mockClient := fake.NewMockJanitorClient(GinkgoT())
				ctx = client.WithClient(ctx, mockClient)
				setupHappyPath(mockClient, zone, false)

				err := handler.CreateOrUpdate(ctx, app)
				Expect(err).ToNot(HaveOccurred())

				readyCond := meta.FindStatusCondition(app.Status.Conditions, condition.ConditionTypeReady)
				Expect(readyCond).ToNot(BeNil())
				Expect(readyCond.Status).To(Equal(metav1.ConditionTrue))
			})

			It("should set Status.ClientSecret only after convergence", func() {
				mockClient := fake.NewMockJanitorClient(GinkgoT())
				ctx = client.WithClient(ctx, mockClient)
				setupHappyPath(mockClient, zone, false)

				err := handler.CreateOrUpdate(ctx, app)
				Expect(err).ToNot(HaveOccurred())

				Expect(app.Status.ClientSecret).To(Equal(app.Spec.Secret))
			})

			It("should not update Status.ClientSecret before convergence", func() {
				mockClient := fake.NewMockJanitorClient(GinkgoT())
				ctx = client.WithClient(ctx, mockClient)
				setupHappyPath(mockClient, zone, true)

				app.Status.ClientSecret = "$<previous-ref>"

				err := handler.CreateOrUpdate(ctx, app)
				Expect(err).ToNot(HaveOccurred())

				Expect(app.Status.ClientSecret).To(Equal("$<previous-ref>"),
					"Status.ClientSecret should retain its previous value until sub-resources converge")
			})

			It("should set NotReady condition when sub-resources changed", func() {
				mockClient := fake.NewMockJanitorClient(GinkgoT())
				ctx = client.WithClient(ctx, mockClient)
				setupHappyPath(mockClient, zone, true)

				err := handler.CreateOrUpdate(ctx, app)
				Expect(err).ToNot(HaveOccurred())

				readyCond := meta.FindStatusCondition(app.Status.Conditions, condition.ConditionTypeReady)
				Expect(readyCond).ToNot(BeNil())
				Expect(readyCond.Status).To(Equal(metav1.ConditionFalse))
			})
		})

		Context("with rotation (spec.rotatedSecret is set)", func() {
			BeforeEach(func() {
				app.Spec.RotatedSecret = "$<ref:rotated-secret>"
			})

			It("should set SecretRotation condition to InProgress on first reconcile", func() {
				mockClient := fake.NewMockJanitorClient(GinkgoT())
				ctx = client.WithClient(ctx, mockClient)
				setupHappyPath(mockClient, zone, true) // changed=true on first reconcile

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
				setupHappyPath(mockClient, zone, true)

				err := handler.CreateOrUpdate(ctx, app)
				Expect(err).ToNot(HaveOccurred())

				Expect(app.Status.RotatedClientSecret).To(BeEmpty(),
					"status.rotatedClientSecret should not be set until sub-resources converge")
			})

			It("should transition to Success when sub-resources settle (AnyChanged=false)", func() {
				mockClient := fake.NewMockJanitorClient(GinkgoT())
				ctx = client.WithClient(ctx, mockClient)

				rotatedExpires := metav1.NewTime(time.Now().Add(24 * time.Hour))
				currentExpires := metav1.NewTime(time.Now().Add(48 * time.Hour))

				// Identity client CreateOrUpdate populates expiry timestamps
				setupHappyPath(mockClient, zone, false, func(idpClient *identityv1.Client) {
					idpClient.Status.RotatedSecretExpiresAt = &rotatedExpires
					idpClient.Status.SecretExpiresAt = &currentExpires
				})

				// Simulate: condition already InProgress from previous reconcile
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
				Expect(cond.Reason).To(Equal(secret.SecretRotationReasonSuccess))
				Expect(cond.Status).To(Equal(metav1.ConditionTrue))

				// Status fields should only be set after convergence
				Expect(app.Status.RotatedClientSecret).To(Equal("$<ref:rotated-secret>"))
			})

			It("should not re-set InProgress if already InProgress", func() {
				mockClient := fake.NewMockJanitorClient(GinkgoT())
				ctx = client.WithClient(ctx, mockClient)
				setupHappyPath(mockClient, zone, true) // still changing

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

				rotatedExpires := metav1.NewTime(time.Now().Add(24 * time.Hour))
				currentExpires := metav1.NewTime(time.Now().Add(48 * time.Hour))

				// Identity client CreateOrUpdate populates expiry timestamps
				setupHappyPath(mockClient, zone, false, func(idpClient *identityv1.Client) {
					idpClient.Status.RotatedSecretExpiresAt = &rotatedExpires
					idpClient.Status.SecretExpiresAt = &currentExpires
				})

				// Simulate InProgress
				app.SetCondition(metav1.Condition{
					Type:    secret.SecretRotationConditionType,
					Status:  metav1.ConditionFalse,
					Reason:  secret.SecretRotationReasonInProgress,
					Message: "Secret rotation initiated",
				})

				err := handler.CreateOrUpdate(ctx, app)
				Expect(err).ToNot(HaveOccurred())

				Expect(app.Status.RotatedExpiresAt).ToNot(BeNil())
				Expect(app.Status.RotatedExpiresAt.Time).To(BeTemporally("~", rotatedExpires.Time, time.Second))
				Expect(app.Status.CurrentExpiresAt).ToNot(BeNil())
				Expect(app.Status.CurrentExpiresAt.Time).To(BeTemporally("~", currentExpires.Time, time.Second))
			})

			It("should complete rotation immediately when identity client has disable-secret-rotation annotation", func() {
				mockClient := fake.NewMockJanitorClient(GinkgoT())
				ctx = client.WithClient(ctx, mockClient)

				// Identity client has rotation disabled — no expiry timestamps will ever arrive.
				setupHappyPath(mockClient, zone, false, func(idpClient *identityv1.Client) {
					idpClient.Annotations = map[string]string{
						identityv1.DisableSecretRotationAnnotation: "true",
					}
				})

				// Simulate InProgress from previous reconcile
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
				Expect(cond.Reason).To(Equal(secret.SecretRotationReasonSuccess))
				Expect(cond.Status).To(Equal(metav1.ConditionTrue))

				Expect(app.Status.RotatedClientSecret).To(Equal("$<ref:rotated-secret>"))
				Expect(app.Status.RotatedExpiresAt).To(BeNil(),
					"RotatedExpiresAt should be nil when graceful rotation is disabled")
			})

			It("should not re-initiate rotation after Success when spec.rotatedSecret matches status", func() {
				mockClient := fake.NewMockJanitorClient(GinkgoT())
				ctx = client.WithClient(ctx, mockClient)
				setupHappyPath(mockClient, zone, false)

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

var _ = Describe("ApplicationHandler - Gateway consumers", func() {
	var (
		ctx        context.Context
		mockClient *fake.MockJanitorClient
		app        *applicationv1.Application
		zone       *adminv1.Zone
	)

	BeforeEach(func() {
		ctx = context.Background()
		mockClient = fake.NewMockJanitorClient(GinkgoT())
		ctx = client.WithClient(ctx, mockClient)
		app = newTestApp()
		app.Status.ClientId = "test-team--test-app"
		zone = newZone()
		zone.Status.Gateways = []adminv1.GatewayStatus{
			{Name: "standard", Gateway: &commontypes.ObjectRef{Name: "standard-gw", Namespace: "zone-ns"}},
			{Name: "ai", Gateway: &commontypes.ObjectRef{Name: "ai-gw", Namespace: "zone-ns"}},
		}
		mockClient.EXPECT().Scheme().Return(newScheme()).Maybe()
	})

	DescribeTable("creates Consumers on the requested distinct gateways",
		func(gateways []*adminv1.GatewayConfig, expected ...string) {
			createdNames := []string{}
			mockClient.EXPECT().
				CreateOrUpdate(mock.Anything, mock.AnythingOfType("*v1.Consumer"), mock.Anything).
				Run(func(_ context.Context, obj pkgclient.Object, fn controllerutil.MutateFn) {
					Expect(fn()).To(Succeed())
					consumer := obj.(*gatewayv1.Consumer)
					createdNames = append(createdNames, consumer.Name)
					Expect(consumer.Spec.Name).To(Equal("test-team--test-app"))
				}).
				Return(controllerutil.OperationResultCreated, nil).Times(len(expected))

			Expect(CreateGatewayConsumers(ctx, zone, app, gateways, WithFailover())).To(Succeed())

			Expect(createdNames).To(ConsistOf(expected))
			Expect(app.Status.Consumers).To(HaveLen(len(expected)))
		},
		Entry("API only", []*adminv1.GatewayConfig{{Name: "standard"}},
			"test-team--test-app--test-zone--standard"),
		Entry("AI only", []*adminv1.GatewayConfig{{Name: "ai"}},
			"test-team--test-app--test-zone--ai"),
		Entry("API and AI", []*adminv1.GatewayConfig{{Name: "standard"}, {Name: "ai"}},
			"test-team--test-app--test-zone--standard", "test-team--test-app--test-zone--ai"),
	)

	It("creates one Consumer when two failover presets reference the same combined gateway", func() {
		zone.Spec.Gateways = []adminv1.GatewayConfig{{Name: "combined", Types: []adminv1.GatewayType{adminv1.GatewayTypeAPI, adminv1.GatewayTypeAI}}}
		zone.Spec.Presets = []adminv1.Preset{
			{Name: "api-failover", GatewayRef: "combined", Features: []adminv1.Feature{{Name: adminv1.FeatureConsumerFailover, Enabled: true}}},
			{Name: "ai-failover", GatewayRef: "combined", Features: []adminv1.Feature{{Name: adminv1.FeatureConsumerFailover, Enabled: true}}},
		}
		zone.Status.Gateways = []adminv1.GatewayStatus{{Name: "combined", Gateway: &commontypes.ObjectRef{Name: "combined-gw", Namespace: "zone-ns"}}}
		gateways, err := zone.Spec.MatchingGateways(adminv1.FeatureConsumerFailover)
		Expect(err).NotTo(HaveOccurred())
		mockClient.EXPECT().
			CreateOrUpdate(mock.Anything, mock.AnythingOfType("*v1.Consumer"), mock.Anything).
			Run(func(_ context.Context, obj pkgclient.Object, fn controllerutil.MutateFn) {
				Expect(fn()).To(Succeed())
				consumer := obj.(*gatewayv1.Consumer)
				Expect(consumer.Name).To(Equal("test-team--test-app--test-zone--combined"))
			}).
			Return(controllerutil.OperationResultCreated, nil).Once()

		Expect(CreateGatewayConsumers(ctx, zone, app, gateways, WithFailover())).To(Succeed())
		Expect(app.Status.Consumers).To(HaveLen(1))
	})

	It("orders Consumer status references by namespace and name", func() {
		mockClient.EXPECT().
			CreateOrUpdate(mock.Anything, mock.AnythingOfType("*v1.Consumer"), mock.Anything).
			Run(func(_ context.Context, _ pkgclient.Object, fn controllerutil.MutateFn) { Expect(fn()).To(Succeed()) }).
			Return(controllerutil.OperationResultCreated, nil).Times(2)

		Expect(CreateGatewayConsumers(ctx, zone, app, []*adminv1.GatewayConfig{{Name: "standard"}, {Name: "ai"}})).To(Succeed())

		Expect(app.Status.Consumers).To(Equal([]commontypes.ObjectRef{
			{Name: "test-team--test-app--test-zone--ai", Namespace: "test-ns"},
			{Name: "test-team--test-app--test-zone--standard", Namespace: "test-ns"},
		}))
	})

	It("blocks when a requested Gateway has no status", func() {
		err := CreateGatewayConsumers(ctx, zone, app, []*adminv1.GatewayConfig{{Name: "missing"}})

		var blockedErr ctrlerrors.BlockedError
		Expect(errors.As(err, &blockedErr)).To(BeTrue())
		Expect(err).To(MatchError(ContainSubstring(`does not contain status for gateway "missing"`)))
	})

	It("blocks when a requested Gateway status has no Gateway reference", func() {
		zone.Status.Gateways = []adminv1.GatewayStatus{{Name: "standard"}}

		err := CreateGatewayConsumers(ctx, zone, app, []*adminv1.GatewayConfig{{Name: "standard"}})

		var blockedErr ctrlerrors.BlockedError
		Expect(errors.As(err, &blockedErr)).To(BeTrue())
		Expect(err).To(MatchError(ContainSubstring(`gateway "standard" has no Gateway reference`)))
	})

	It("returns a partial failure after recording only successful Consumers", func() {
		mockClient.EXPECT().
			CreateOrUpdate(mock.Anything, mock.MatchedBy(func(obj pkgclient.Object) bool {
				return obj.GetName() == "test-team--test-app--test-zone--standard"
			}), mock.Anything).
			Run(func(_ context.Context, _ pkgclient.Object, fn controllerutil.MutateFn) { Expect(fn()).To(Succeed()) }).
			Return(controllerutil.OperationResultCreated, nil).Once()
		mockClient.EXPECT().
			CreateOrUpdate(mock.Anything, mock.MatchedBy(func(obj pkgclient.Object) bool {
				return obj.GetName() == "test-team--test-app--test-zone--ai"
			}), mock.Anything).
			Return(controllerutil.OperationResultNone, errors.New("creating AI Consumer failed")).Once()

		err := CreateGatewayConsumers(ctx, zone, app, []*adminv1.GatewayConfig{{Name: "standard"}, {Name: "ai"}})

		Expect(err).To(MatchError(ContainSubstring("creating AI Consumer failed")))
		Expect(app.Status.Consumers).To(Equal([]commontypes.ObjectRef{{
			Name: "test-team--test-app--test-zone--standard", Namespace: "test-ns",
		}}))
	})

	It("excludes Event-only primary gateways", func() {
		zone.Spec.Gateways = append(zone.Spec.Gateways,
			adminv1.GatewayConfig{Name: "events", Types: []adminv1.GatewayType{adminv1.GatewayTypeEvent}},
			adminv1.GatewayConfig{Name: "ai", Types: []adminv1.GatewayType{adminv1.GatewayTypeAI}},
		)
		createdNames := []string{}
		mockClient.EXPECT().
			CreateOrUpdate(mock.Anything, mock.AnythingOfType("*v1.Consumer"), mock.Anything).
			Run(func(_ context.Context, obj pkgclient.Object, fn controllerutil.MutateFn) {
				Expect(fn()).To(Succeed())
				createdNames = append(createdNames, obj.GetName())
			}).
			Return(controllerutil.OperationResultCreated, nil).Times(2)

		Expect((&ApplicationHandler{}).ensureGatewayConsumers(ctx, zone, nil, app)).To(Succeed())

		Expect(createdNames).To(ConsistOf(
			"test-team--test-app--test-zone--standard",
			"test-team--test-app--test-zone--ai",
		))
	})

	It("blocks before creating a Consumer whose resource name exceeds the Kubernetes limit", func() {
		app.Spec.Team = strings.Repeat("t", 64)
		app.Name = strings.Repeat("a", 128)
		zone.Name = strings.Repeat("z", 32)
		zone.Status.Gateways = []adminv1.GatewayStatus{{
			Name:    strings.Repeat("g", 24),
			Gateway: &commontypes.ObjectRef{Name: "gateway", Namespace: "zone-ns"},
		}}

		err := CreateGatewayConsumers(ctx, zone, app, []*adminv1.GatewayConfig{{Name: zone.Status.Gateways[0].Name}})

		var blockedErr ctrlerrors.BlockedError
		Expect(errors.As(err, &blockedErr)).To(BeTrue())
		Expect(err).To(MatchError(ContainSubstring("exceeds the Kubernetes maximum of 253 characters")))
		Expect(app.Status.Consumers).To(BeEmpty())
	})
})
