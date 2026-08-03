// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package zoneserviceconfig

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/stretchr/testify/mock"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	k8smeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	cclient "github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/client/fake"
	"github.com/telekom/controlplane/common/pkg/condition"
	cconfig "github.com/telekom/controlplane/common/pkg/config"
	"github.com/telekom/controlplane/common/pkg/errors/ctrlerrors"
	"github.com/telekom/controlplane/common/pkg/types"
	filev1 "github.com/telekom/controlplane/file/api/v1"
	gatewayapi "github.com/telekom/controlplane/gateway/api/v1"
	identityv1 "github.com/telekom/controlplane/identity/api/v1"
	secretsapi "github.com/telekom/controlplane/secret-manager/api"
	fakesecrets "github.com/telekom/controlplane/secret-manager/api/fake"
	sftpv1 "github.com/telekom/controlplane/sftp/api/v1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const (
	testNamespace   = "test"
	testEnv         = "test-env"
	testZoneName    = "test-zsc"
	testZoneNS      = testEnv
	testAPIURL      = "https://sftp-api.internal/v1"
	testAPIPath     = "/sftp/v1"
	testGatewayHost = "gateway.example.com"
	testIssuerURL   = "https://idp.example.com/auth/realms/test"
)

// buildScheme builds a runtime.Scheme with all types used by the handler.
func buildScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	_ = filev1.AddToScheme(s)
	_ = adminv1.AddToScheme(s)
	_ = identityv1.AddToScheme(s)
	_ = gatewayapi.AddToScheme(s)
	_ = sftpv1.AddToScheme(s)
	return s
}

func newTestContext() (context.Context, *fake.MockJanitorClient) {
	mockClient := fake.NewMockJanitorClient(GinkgoT())
	ctx := cclient.WithClient(context.Background(), mockClient)
	return ctx, mockClient
}

func testZoneServiceConfig() *filev1.ZoneServiceConfig {
	return &filev1.ZoneServiceConfig{
		TypeMeta: metav1.TypeMeta{
			APIVersion: filev1.GroupVersion.String(),
			Kind:       "ZoneServiceConfig",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      testZoneName,
			Namespace: testNamespace,
			Labels:    map[string]string{cconfig.EnvironmentLabelKey: testEnv},
		},
		Spec: filev1.ZoneServiceConfigSpec{
			API: adminv1.ManagedRouteConfig{
				Name: "sftp-api",
				Path: testAPIPath,
				Url:  testAPIURL,
				Type: adminv1.ManagedRouteTypeTeamAPI,
			},
			Zone: &types.ObjectRef{Name: testZoneName, Namespace: testEnv},
		},
	}
}

func testReadyZone() *adminv1.Zone {
	z := &adminv1.Zone{
		TypeMeta: metav1.TypeMeta{
			APIVersion: adminv1.GroupVersion.String(),
			Kind:       "Zone",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      testZoneName,
			Namespace: testEnv,
			Labels:    map[string]string{cconfig.EnvironmentLabelKey: testEnv},
		},
		Spec: adminv1.ZoneSpec{
			Gateway: adminv1.GatewayConfig{
				Presets: []adminv1.GatewayConfigPreset{{
					Name:    "default",
					Default: true,
					Urls: []adminv1.UrlConfig{{
						Hostname: testGatewayHost,
						BasePath: "/",
					}},
				}},
			},
		},
		Status: adminv1.ZoneStatus{
			Gateway: &types.ObjectRef{Name: "gw", Namespace: testEnv},
			InternalIdentityRealm: &types.ObjectRef{
				Name:      "internal-realm",
				Namespace: testEnv,
			},
			Links: adminv1.Links{
				InternalIssuer: testIssuerURL,
			},
		},
	}
	k8smeta.SetStatusCondition(&z.Status.Conditions, metav1.Condition{
		Type:   condition.ConditionTypeReady,
		Status: metav1.ConditionTrue,
		Reason: "Ready",
	})
	return z
}

var _ = Describe("ZoneServiceConfigHandler", func() {
	var handler *ZoneServiceConfigHandler

	BeforeEach(func() {
		handler = &ZoneServiceConfigHandler{}
	})

	Describe("CreateOrUpdate", func() {
		It("blocks when the Zone is not found", func() {
			obj := testZoneServiceConfig()
			ctx, mockClient := newTestContext()

			mockClient.EXPECT().
				Get(mock.Anything, k8stypes.NamespacedName{Name: testZoneName, Namespace: testEnv}, mock.AnythingOfType("*v1.Zone")).
				Return(apierrors.NewNotFound(schema.GroupResource{Group: adminv1.GroupVersion.Group, Resource: "zones"}, testZoneName)).
				Once()

			err := handler.CreateOrUpdate(ctx, obj)

			var blocked ctrlerrors.BlockedError
			Expect(errors.As(err, &blocked)).To(BeTrue())
			Expect(err.Error()).To(ContainSubstring("Zone"))
		})

		It("returns error when Zone Get fails", func() {
			obj := testZoneServiceConfig()
			ctx, mockClient := newTestContext()

			mockClient.EXPECT().
				Get(mock.Anything, k8stypes.NamespacedName{Name: testZoneName, Namespace: testEnv}, mock.AnythingOfType("*v1.Zone")).
				Return(fmt.Errorf("api server unavailable")).
				Once()

			err := handler.CreateOrUpdate(ctx, obj)

			Expect(err).To(MatchError(ContainSubstring("api server unavailable")))
		})

		It("blocks when Zone is not ready", func() {
			obj := testZoneServiceConfig()
			ctx, mockClient := newTestContext()

			notReadyZone := testReadyZone()
			notReadyZone.Status.Conditions = nil

			mockClient.EXPECT().
				Get(mock.Anything, k8stypes.NamespacedName{Name: testZoneName, Namespace: testEnv}, mock.AnythingOfType("*v1.Zone")).
				Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
					*out.(*adminv1.Zone) = *notReadyZone
				}).
				Return(nil).Once()

			err := handler.CreateOrUpdate(ctx, obj)

			Expect(err).NotTo(HaveOccurred())
			Expect(k8smeta.IsStatusConditionFalse(obj.Status.Conditions, condition.ConditionTypeReady)).To(BeTrue())
			ready := k8smeta.FindStatusCondition(obj.Status.Conditions, condition.ConditionTypeReady)
			Expect(ready.Reason).To(Equal("ZoneNotReady"))
		})

		It("blocks when Zone has no InternalIdentityRealm", func() {
			obj := testZoneServiceConfig()
			ctx, mockClient := newTestContext()

			zoneNoRealm := testReadyZone()
			zoneNoRealm.Status.InternalIdentityRealm = nil

			mockClient.EXPECT().
				Get(mock.Anything, k8stypes.NamespacedName{Name: testZoneName, Namespace: testEnv}, mock.AnythingOfType("*v1.Zone")).
				Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
					*out.(*adminv1.Zone) = *zoneNoRealm
				}).
				Return(nil).Once()

			err := handler.CreateOrUpdate(ctx, obj)

			var blocked2 ctrlerrors.BlockedError
			Expect(errors.As(err, &blocked2)).To(BeTrue())
			Expect(err.Error()).To(ContainSubstring("internal identity realm"))
		})

		It("provisions all child resources and sets Ready condition", func() {
			obj := testZoneServiceConfig()
			ctx, mockClient := newTestContext()
			testScheme := buildScheme()

			zone := testReadyZone()

			// replace secretsapi.API with a mock so UpsertEnvironment works without a real server
			origAPI := secretsapi.API
			DeferCleanup(func() { secretsapi.API = origAPI })
			mockSecretsManager := fakesecrets.NewMockSecretManager(GinkgoT())
			secretsapi.API = func() secretsapi.SecretManager { return mockSecretsManager }
			mockSecretsManager.EXPECT().
				UpsertEnvironment(mock.Anything, testEnv, mock.Anything, mock.Anything).
				Return(map[string]string{"zones/" + testZoneName + "/file/" + testNamespace + "/" + testZoneName + "/clientSecret": "secret-id::v1"}, nil).
				Once()

			mockClient.EXPECT().
				Get(mock.Anything, k8stypes.NamespacedName{Name: testZoneName, Namespace: testEnv}, mock.AnythingOfType("*v1.Zone")).
				Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
					*out.(*adminv1.Zone) = *zone
				}).
				Return(nil).Once()
			// identity Client Get → not found → will be created
			mockClient.EXPECT().
				Get(mock.Anything, mock.Anything, mock.AnythingOfType("*v1.Client")).
				Return(apierrors.NewNotFound(schema.GroupResource{}, "sftp-api--test-zsc")).
				Once()
			mockClient.EXPECT().Scheme().Return(testScheme).Maybe()
			// CreateOrUpdate for identity Client
			mockClient.EXPECT().
				CreateOrUpdate(mock.Anything, mock.AnythingOfType("*v1.Client"), mock.Anything).
				Return(controllerutil.OperationResultCreated, nil).Once()
			// CreateOrUpdate for gateway Consumer
			mockClient.EXPECT().
				CreateOrUpdate(mock.Anything, mock.AnythingOfType("*v1.Consumer"), mock.Anything).
				Return(controllerutil.OperationResultCreated, nil).Once()
			// CreateOrUpdate for gateway Route
			mockClient.EXPECT().
				CreateOrUpdate(mock.Anything, mock.AnythingOfType("*v1.Route"), mock.Anything).
				Return(controllerutil.OperationResultCreated, nil).Once()
			// First AllReady (after identity client + consumer + route) → false
			mockClient.EXPECT().AllReady().Return(false).Once()

			err := handler.CreateOrUpdate(ctx, obj)

			Expect(err).NotTo(HaveOccurred())
			Expect(k8smeta.IsStatusConditionFalse(obj.Status.Conditions, condition.ConditionTypeReady)).To(BeTrue())
		})

		It("returns error when identity Client Get fails with unexpected error", func() {
			obj := testZoneServiceConfig()
			ctx, mockClient := newTestContext()
			zone := testReadyZone()

			mockClient.EXPECT().
				Get(mock.Anything, k8stypes.NamespacedName{Name: testZoneName, Namespace: testEnv}, mock.AnythingOfType("*v1.Zone")).
				Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
					*out.(*adminv1.Zone) = *zone
				}).
				Return(nil).Once()
			mockClient.EXPECT().
				Get(mock.Anything, mock.Anything, mock.AnythingOfType("*v1.Client")).
				Return(fmt.Errorf("connection refused")).Once()

			err := handler.CreateOrUpdate(ctx, obj)

			Expect(err).To(MatchError(ContainSubstring("connection refused")))
		})

		It("continues without secret rotation when identity Client is found but not ready", func() {
			obj := testZoneServiceConfig()
			ctx, mockClient := newTestContext()
			zone := testReadyZone()
			notReadyClient := identityv1.Client{
				ObjectMeta: metav1.ObjectMeta{Name: "sftp-api--" + testZoneName, Namespace: testNamespace},
			}

			mockClient.EXPECT().
				Get(mock.Anything, k8stypes.NamespacedName{Name: testZoneName, Namespace: testEnv}, mock.AnythingOfType("*v1.Zone")).
				Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
					*out.(*adminv1.Zone) = *zone
				}).
				Return(nil).Once()
			mockClient.EXPECT().
				Get(mock.Anything, mock.Anything, mock.AnythingOfType("*v1.Client")).
				Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
					*out.(*identityv1.Client) = notReadyClient
				}).
				Return(nil).Once()
			mockClient.EXPECT().
				CreateOrUpdate(mock.Anything, mock.AnythingOfType("*v1.Consumer"), mock.Anything).
				Return(controllerutil.OperationResultNone, nil).Once()
			mockClient.EXPECT().
				CreateOrUpdate(mock.Anything, mock.AnythingOfType("*v1.Route"), mock.Anything).
				Return(controllerutil.OperationResultNone, nil).Once()
			mockClient.EXPECT().AllReady().Return(false).Once()

			err := handler.CreateOrUpdate(ctx, obj)

			Expect(err).NotTo(HaveOccurred())
		})

		It("continues without secret rotation when Client is found, ready, and SecretExpiresAt is nil", func() {
			obj := testZoneServiceConfig()
			ctx, mockClient := newTestContext()
			zone := testReadyZone()
			readyClient := identityv1.Client{
				ObjectMeta: metav1.ObjectMeta{Name: "sftp-api--" + testZoneName, Namespace: testNamespace},
			}
			k8smeta.SetStatusCondition(&readyClient.Status.Conditions, metav1.Condition{
				Type:   condition.ConditionTypeReady,
				Status: metav1.ConditionTrue,
				Reason: "Ready",
			})

			mockClient.EXPECT().
				Get(mock.Anything, k8stypes.NamespacedName{Name: testZoneName, Namespace: testEnv}, mock.AnythingOfType("*v1.Zone")).
				Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
					*out.(*adminv1.Zone) = *zone
				}).
				Return(nil).Once()
			mockClient.EXPECT().
				Get(mock.Anything, mock.Anything, mock.AnythingOfType("*v1.Client")).
				Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
					*out.(*identityv1.Client) = readyClient
				}).
				Return(nil).Once()
			mockClient.EXPECT().
				CreateOrUpdate(mock.Anything, mock.AnythingOfType("*v1.Consumer"), mock.Anything).
				Return(controllerutil.OperationResultNone, nil).Once()
			mockClient.EXPECT().
				CreateOrUpdate(mock.Anything, mock.AnythingOfType("*v1.Route"), mock.Anything).
				Return(controllerutil.OperationResultNone, nil).Once()
			mockClient.EXPECT().AllReady().Return(false).Once()

			err := handler.CreateOrUpdate(ctx, obj)

			Expect(err).NotTo(HaveOccurred())
		})

		It("continues without secret rotation when Client is found, ready, and secret is far from expiry", func() {
			obj := testZoneServiceConfig()
			ctx, mockClient := newTestContext()
			zone := testReadyZone()
			farExpiry := metav1.Time{Time: time.Now().Add(30 * 24 * time.Hour)}
			readyClient := identityv1.Client{
				ObjectMeta: metav1.ObjectMeta{Name: "sftp-api--" + testZoneName, Namespace: testNamespace},
				Status:     identityv1.ClientStatus{SecretExpiresAt: &farExpiry},
			}
			k8smeta.SetStatusCondition(&readyClient.Status.Conditions, metav1.Condition{
				Type:   condition.ConditionTypeReady,
				Status: metav1.ConditionTrue,
				Reason: "Ready",
			})

			mockClient.EXPECT().
				Get(mock.Anything, k8stypes.NamespacedName{Name: testZoneName, Namespace: testEnv}, mock.AnythingOfType("*v1.Zone")).
				Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
					*out.(*adminv1.Zone) = *zone
				}).
				Return(nil).Once()
			mockClient.EXPECT().
				Get(mock.Anything, mock.Anything, mock.AnythingOfType("*v1.Client")).
				Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
					*out.(*identityv1.Client) = readyClient
				}).
				Return(nil).Once()
			mockClient.EXPECT().
				CreateOrUpdate(mock.Anything, mock.AnythingOfType("*v1.Consumer"), mock.Anything).
				Return(controllerutil.OperationResultNone, nil).Once()
			mockClient.EXPECT().
				CreateOrUpdate(mock.Anything, mock.AnythingOfType("*v1.Route"), mock.Anything).
				Return(controllerutil.OperationResultNone, nil).Once()
			mockClient.EXPECT().AllReady().Return(false).Once()

			err := handler.CreateOrUpdate(ctx, obj)

			Expect(err).NotTo(HaveOccurred())
		})

		It("rotates the Client secret when expiry is within 7 days", func() {
			obj := testZoneServiceConfig()
			ctx, mockClient := newTestContext()
			zone := testReadyZone()
			nearExpiry := metav1.Time{Time: time.Now().Add(3 * 24 * time.Hour)}
			readyClient := identityv1.Client{
				ObjectMeta: metav1.ObjectMeta{Name: "sftp-api--" + testZoneName, Namespace: testNamespace},
				Status:     identityv1.ClientStatus{SecretExpiresAt: &nearExpiry},
			}
			k8smeta.SetStatusCondition(&readyClient.Status.Conditions, metav1.Condition{
				Type:   condition.ConditionTypeReady,
				Status: metav1.ConditionTrue,
				Reason: "Ready",
			})

			origAPI := secretsapi.API
			DeferCleanup(func() { secretsapi.API = origAPI })
			mockSecretsManager := fakesecrets.NewMockSecretManager(GinkgoT())
			secretsapi.API = func() secretsapi.SecretManager { return mockSecretsManager }
			secretPath := "zones/" + testZoneName + "/file/" + testNamespace + "/" + testZoneName + "/clientSecret"
			mockSecretsManager.EXPECT().
				UpsertEnvironment(mock.Anything, testEnv, mock.Anything, mock.Anything).
				Return(map[string]string{secretPath: "secret-id::v2"}, nil).Once()

			mockClient.EXPECT().
				Get(mock.Anything, k8stypes.NamespacedName{Name: testZoneName, Namespace: testEnv}, mock.AnythingOfType("*v1.Zone")).
				Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
					*out.(*adminv1.Zone) = *zone
				}).
				Return(nil).Once()
			mockClient.EXPECT().
				Get(mock.Anything, mock.Anything, mock.AnythingOfType("*v1.Client")).
				Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
					*out.(*identityv1.Client) = readyClient
				}).
				Return(nil).Once()
			mockClient.EXPECT().
				CreateOrUpdate(mock.Anything, mock.AnythingOfType("*v1.Client"), mock.Anything).
				Return(controllerutil.OperationResultNone, nil).Once()
			mockClient.EXPECT().
				CreateOrUpdate(mock.Anything, mock.AnythingOfType("*v1.Consumer"), mock.Anything).
				Return(controllerutil.OperationResultNone, nil).Once()
			mockClient.EXPECT().
				CreateOrUpdate(mock.Anything, mock.AnythingOfType("*v1.Route"), mock.Anything).
				Return(controllerutil.OperationResultNone, nil).Once()
			mockClient.EXPECT().AllReady().Return(false).Once()

			err := handler.CreateOrUpdate(ctx, obj)

			Expect(err).NotTo(HaveOccurred())
		})

		It("returns error when UpsertEnvironment fails", func() {
			obj := testZoneServiceConfig()
			ctx, mockClient := newTestContext()
			zone := testReadyZone()

			origAPI := secretsapi.API
			DeferCleanup(func() { secretsapi.API = origAPI })
			mockSecretsManager := fakesecrets.NewMockSecretManager(GinkgoT())
			secretsapi.API = func() secretsapi.SecretManager { return mockSecretsManager }
			mockSecretsManager.EXPECT().
				UpsertEnvironment(mock.Anything, testEnv, mock.Anything, mock.Anything).
				Return(nil, fmt.Errorf("secret manager unavailable")).Once()

			mockClient.EXPECT().
				Get(mock.Anything, k8stypes.NamespacedName{Name: testZoneName, Namespace: testEnv}, mock.AnythingOfType("*v1.Zone")).
				Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
					*out.(*adminv1.Zone) = *zone
				}).
				Return(nil).Once()
			mockClient.EXPECT().
				Get(mock.Anything, mock.Anything, mock.AnythingOfType("*v1.Client")).
				Return(apierrors.NewNotFound(schema.GroupResource{}, "sftp-api--"+testZoneName)).Once()

			err := handler.CreateOrUpdate(ctx, obj)

			Expect(err).To(MatchError(ContainSubstring("secret manager unavailable")))
		})

		It("returns error when the secret ID is not present in the UpsertEnvironment response", func() {
			obj := testZoneServiceConfig()
			ctx, mockClient := newTestContext()
			zone := testReadyZone()

			origAPI := secretsapi.API
			DeferCleanup(func() { secretsapi.API = origAPI })
			mockSecretsManager := fakesecrets.NewMockSecretManager(GinkgoT())
			secretsapi.API = func() secretsapi.SecretManager { return mockSecretsManager }
			mockSecretsManager.EXPECT().
				UpsertEnvironment(mock.Anything, testEnv, mock.Anything, mock.Anything).
				Return(map[string]string{"unrelated/key": "some-id"}, nil).Once()

			mockClient.EXPECT().
				Get(mock.Anything, k8stypes.NamespacedName{Name: testZoneName, Namespace: testEnv}, mock.AnythingOfType("*v1.Zone")).
				Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
					*out.(*adminv1.Zone) = *zone
				}).
				Return(nil).Once()
			mockClient.EXPECT().
				Get(mock.Anything, mock.Anything, mock.AnythingOfType("*v1.Client")).
				Return(apierrors.NewNotFound(schema.GroupResource{}, "sftp-api--"+testZoneName)).Once()

			err := handler.CreateOrUpdate(ctx, obj)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to find secret ID"))
		})

		It("returns error when identity Client CreateOrUpdate fails", func() {
			obj := testZoneServiceConfig()
			ctx, mockClient := newTestContext()
			zone := testReadyZone()

			origAPI := secretsapi.API
			DeferCleanup(func() { secretsapi.API = origAPI })
			mockSecretsManager := fakesecrets.NewMockSecretManager(GinkgoT())
			secretsapi.API = func() secretsapi.SecretManager { return mockSecretsManager }
			secretPath := "zones/" + testZoneName + "/file/" + testNamespace + "/" + testZoneName + "/clientSecret"
			mockSecretsManager.EXPECT().
				UpsertEnvironment(mock.Anything, testEnv, mock.Anything, mock.Anything).
				Return(map[string]string{secretPath: "secret-id::v1"}, nil).Once()

			mockClient.EXPECT().
				Get(mock.Anything, k8stypes.NamespacedName{Name: testZoneName, Namespace: testEnv}, mock.AnythingOfType("*v1.Zone")).
				Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
					*out.(*adminv1.Zone) = *zone
				}).
				Return(nil).Once()
			mockClient.EXPECT().
				Get(mock.Anything, mock.Anything, mock.AnythingOfType("*v1.Client")).
				Return(apierrors.NewNotFound(schema.GroupResource{}, "sftp-api--"+testZoneName)).Once()
			mockClient.EXPECT().
				CreateOrUpdate(mock.Anything, mock.AnythingOfType("*v1.Client"), mock.Anything).
				Return(controllerutil.OperationResultNone, fmt.Errorf("client creation failed")).Once()

			err := handler.CreateOrUpdate(ctx, obj)

			Expect(err).To(MatchError(ContainSubstring("client creation failed")))
		})

		It("returns error when createConsumer fails", func() {
			obj := testZoneServiceConfig()
			ctx, mockClient := newTestContext()
			zone := testReadyZone()
			notReadyClient := identityv1.Client{
				ObjectMeta: metav1.ObjectMeta{Name: "sftp-api--" + testZoneName, Namespace: testNamespace},
			}

			mockClient.EXPECT().
				Get(mock.Anything, k8stypes.NamespacedName{Name: testZoneName, Namespace: testEnv}, mock.AnythingOfType("*v1.Zone")).
				Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
					*out.(*adminv1.Zone) = *zone
				}).
				Return(nil).Once()
			mockClient.EXPECT().
				Get(mock.Anything, mock.Anything, mock.AnythingOfType("*v1.Client")).
				Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
					*out.(*identityv1.Client) = notReadyClient
				}).
				Return(nil).Once()
			mockClient.EXPECT().
				CreateOrUpdate(mock.Anything, mock.AnythingOfType("*v1.Consumer"), mock.Anything).
				Return(controllerutil.OperationResultNone, fmt.Errorf("consumer creation failed")).Once()

			err := handler.CreateOrUpdate(ctx, obj)

			Expect(err).To(MatchError(ContainSubstring("consumer creation failed")))
		})

		It("returns a BlockedError when Zone has no default Gateway preset", func() {
			obj := testZoneServiceConfig()
			ctx, mockClient := newTestContext()
			zoneNoPreset := testReadyZone()
			zoneNoPreset.Spec.Gateway.Presets = nil
			notReadyClient := identityv1.Client{
				ObjectMeta: metav1.ObjectMeta{Name: "sftp-api--" + testZoneName, Namespace: testNamespace},
			}

			mockClient.EXPECT().
				Get(mock.Anything, k8stypes.NamespacedName{Name: testZoneName, Namespace: testEnv}, mock.AnythingOfType("*v1.Zone")).
				Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
					*out.(*adminv1.Zone) = *zoneNoPreset
				}).
				Return(nil).Once()
			mockClient.EXPECT().
				Get(mock.Anything, mock.Anything, mock.AnythingOfType("*v1.Client")).
				Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
					*out.(*identityv1.Client) = notReadyClient
				}).
				Return(nil).Once()
			mockClient.EXPECT().
				CreateOrUpdate(mock.Anything, mock.AnythingOfType("*v1.Consumer"), mock.Anything).
				Return(controllerutil.OperationResultNone, nil).Once()

			err := handler.CreateOrUpdate(ctx, obj)

			var blocked ctrlerrors.BlockedError
			Expect(errors.As(err, &blocked)).To(BeTrue())
			Expect(err.Error()).To(ContainSubstring("preset"))
		})

		It("returns error when Route CreateOrUpdate fails", func() {
			obj := testZoneServiceConfig()
			ctx, mockClient := newTestContext()
			zone := testReadyZone()
			notReadyClient := identityv1.Client{
				ObjectMeta: metav1.ObjectMeta{Name: "sftp-api--" + testZoneName, Namespace: testNamespace},
			}

			mockClient.EXPECT().
				Get(mock.Anything, k8stypes.NamespacedName{Name: testZoneName, Namespace: testEnv}, mock.AnythingOfType("*v1.Zone")).
				Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
					*out.(*adminv1.Zone) = *zone
				}).
				Return(nil).Once()
			mockClient.EXPECT().
				Get(mock.Anything, mock.Anything, mock.AnythingOfType("*v1.Client")).
				Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
					*out.(*identityv1.Client) = notReadyClient
				}).
				Return(nil).Once()
			mockClient.EXPECT().
				CreateOrUpdate(mock.Anything, mock.AnythingOfType("*v1.Consumer"), mock.Anything).
				Return(controllerutil.OperationResultNone, nil).Once()
			mockClient.EXPECT().
				CreateOrUpdate(mock.Anything, mock.AnythingOfType("*v1.Route"), mock.Anything).
				Return(controllerutil.OperationResultNone, fmt.Errorf("route creation failed")).Once()

			err := handler.CreateOrUpdate(ctx, obj)

			Expect(err).To(MatchError(ContainSubstring("route creation failed")))
		})

		It("sets Ready condition when all child resources are fully provisioned", func() {
			obj := testZoneServiceConfig()
			ctx, mockClient := newTestContext()
			zone := testReadyZone()
			notReadyClient := identityv1.Client{
				ObjectMeta: metav1.ObjectMeta{Name: "sftp-api--" + testZoneName, Namespace: testNamespace},
			}

			mockClient.EXPECT().
				Get(mock.Anything, k8stypes.NamespacedName{Name: testZoneName, Namespace: testEnv}, mock.AnythingOfType("*v1.Zone")).
				Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
					*out.(*adminv1.Zone) = *zone
				}).
				Return(nil).Once()
			mockClient.EXPECT().
				Get(mock.Anything, mock.Anything, mock.AnythingOfType("*v1.Client")).
				Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
					*out.(*identityv1.Client) = notReadyClient
				}).
				Return(nil).Once()
			mockClient.EXPECT().
				CreateOrUpdate(mock.Anything, mock.AnythingOfType("*v1.Consumer"), mock.Anything).
				Return(controllerutil.OperationResultCreated, nil).Once()
			// Populate Hostnames and Paths so sftpAPIEndpointFromManagedRoute can succeed.
			mockClient.EXPECT().
				CreateOrUpdate(mock.Anything, mock.AnythingOfType("*v1.Route"), mock.Anything).
				RunAndReturn(func(_ context.Context, obj client.Object, _ controllerutil.MutateFn) (controllerutil.OperationResult, error) {
					obj.(*gatewayapi.Route).Spec.Hostnames = []string{testGatewayHost}
					obj.(*gatewayapi.Route).Spec.Paths = []string{testAPIPath}
					return controllerutil.OperationResultCreated, nil
				}).Once()
			mockClient.EXPECT().AllReady().Return(true).Once()
			mockClient.EXPECT().
				CreateOrUpdate(mock.Anything, mock.AnythingOfType("*v1.SFTPServiceConfig"), mock.Anything).
				Return(controllerutil.OperationResultCreated, nil).Once()
			mockClient.EXPECT().AllReady().Return(true).Once()

			err := handler.CreateOrUpdate(ctx, obj)

			Expect(err).NotTo(HaveOccurred())
			Expect(k8smeta.IsStatusConditionTrue(obj.Status.Conditions, condition.ConditionTypeReady)).To(BeTrue())
			Expect(obj.Status.SFTPServiceConfigRef).NotTo(BeNil())
		})
	})

	Describe("Delete", func() {
		It("returns nil (secret cleanup is a TODO)", func() {
			obj := testZoneServiceConfig()
			ctx, _ := newTestContext()

			err := handler.Delete(ctx, obj)

			Expect(err).NotTo(HaveOccurred())
		})
	})
})

var _ = Describe("tokenEndpointFromIssuer", func() {
	It("returns the issuer unchanged when it already ends with the token endpoint path", func() {
		issuer := "https://idp.example.com/auth/realms/test/" + tokenEndpointPath
		result, err := tokenEndpointFromIssuer(issuer)
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(Equal(issuer))
	})

	It("appends the token endpoint path to the issuer", func() {
		result, err := tokenEndpointFromIssuer(testIssuerURL)
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(HaveSuffix(tokenEndpointPath))
		Expect(result).To(HavePrefix("https://"))
	})
})

var _ = Describe("sftpAPIEndpointFromManagedRoute", func() {
	It("returns an error when the route has no hostnames", func() {
		route := &gatewayapi.Route{Spec: gatewayapi.RouteSpec{Paths: []string{testAPIPath}}}
		_, err := sftpAPIEndpointFromManagedRoute(route, testReadyZone(), &identityv1.Client{})
		Expect(err).To(MatchError(ContainSubstring("hostname")))
	})

	It("returns an error when the route has no paths", func() {
		route := &gatewayapi.Route{Spec: gatewayapi.RouteSpec{Hostnames: []string{testGatewayHost}}}
		_, err := sftpAPIEndpointFromManagedRoute(route, testReadyZone(), &identityv1.Client{})
		Expect(err).To(MatchError(ContainSubstring("path")))
	})

	It("builds the API endpoint from the route and zone", func() {
		route := &gatewayapi.Route{
			ObjectMeta: metav1.ObjectMeta{Name: "r", Namespace: testNamespace},
			Spec: gatewayapi.RouteSpec{
				Hostnames: []string{testGatewayHost},
				Paths:     []string{testAPIPath},
			},
		}
		apiClient := &identityv1.Client{Spec: identityv1.ClientSpec{ClientId: "my-client"}}

		ep, err := sftpAPIEndpointFromManagedRoute(route, testReadyZone(), apiClient)

		Expect(err).NotTo(HaveOccurred())
		Expect(ep.Endpoint).To(Equal("https://" + testGatewayHost + testAPIPath))
		Expect(ep.ClientID).To(Equal("my-client"))
		Expect(ep.Issuer).NotTo(BeEmpty())
	})
})
