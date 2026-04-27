// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package client

import (
	"context"
	"errors"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/mock"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	pkgclient "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/client/fake"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/errors/ctrlerrors"
	common "github.com/telekom/controlplane/common/pkg/types"
	"github.com/telekom/controlplane/common/pkg/util/contextutil"
	identityv1 "github.com/telekom/controlplane/identity/api/v1"
	"github.com/telekom/controlplane/identity/pkg/keycloak"
	"github.com/telekom/controlplane/identity/test/mocks/keycloakservice"
	secrets "github.com/telekom/controlplane/secret-manager/api"
)

// notFoundError implements the apierrors.APIStatus interface for testing.
type notFoundError struct{}

func (e *notFoundError) Error() string { return "not found" }
func (e *notFoundError) Status() metav1.Status {
	return metav1.Status{Reason: metav1.StatusReasonNotFound, Code: 404}
}

func newTestContext(mockClient *fake.MockJanitorClient) (context.Context, *record.FakeRecorder) {
	recorder := record.NewFakeRecorder(10)
	ctx := context.Background()
	ctx = client.WithClient(ctx, mockClient)
	ctx = contextutil.WithRecorder(ctx, recorder)
	return ctx, recorder
}

func overrideSecretsGet(fn func(ctx context.Context, secretRef string) (string, error)) {
	original := secrets.Get
	secrets.Get = fn
	DeferCleanup(func() { secrets.Get = original })
}

func newValidClient() *identityv1.Client {
	return &identityv1.Client{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-client",
			Namespace: "default",
		},
		Spec: identityv1.ClientSpec{
			Realm:        &common.ObjectRef{Namespace: "default", Name: "test-realm"},
			ClientId:     "my-app",
			ClientSecret: "$<client-secret>",
		},
	}
}

func newValidRealm() *identityv1.Realm {
	realm := &identityv1.Realm{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-realm",
			Namespace: "default",
		},
		Status: identityv1.RealmStatus{
			IssuerUrl:     "https://keycloak.example.com/auth/realms/test-realm",
			AdminClientId: "admin-cli",
			AdminUserName: "admin",
			AdminPassword: "resolved-password",
			AdminUrl:      "https://keycloak.example.com/auth/admin/realms/",
			AdminTokenUrl: "https://keycloak.example.com/auth/realms/master/protocol/openid-connect/token",
		},
	}
	realm.SetCondition(condition.NewReadyCondition("Ready", "Realm is ready"))
	return realm
}

func mockRealmGet(mockK8s *fake.MockJanitorClient, realm *identityv1.Realm) {
	mockK8s.EXPECT().
		Get(mock.Anything, types.NamespacedName{Namespace: "default", Name: "test-realm"}, mock.AnythingOfType("*v1.Realm"), mock.Anything).
		Run(func(_ context.Context, _ types.NamespacedName, obj pkgclient.Object, _ ...pkgclient.GetOption) {
			*obj.(*identityv1.Realm) = *realm
		}).
		Return(nil)
}

var _ = Describe("HandlerClient", func() {

	var (
		mockK8s *fake.MockJanitorClient
		ctx     context.Context
	)

	BeforeEach(func() {
		mockK8s = fake.NewMockJanitorClient(GinkgoT())
		ctx, _ = newTestContext(mockK8s)
	})

	Context("CreateOrUpdate", func() {

		It("should return an error when the client is nil", func() {
			handler := NewHandlerClient(keycloak.NewServiceFactory())
			err := handler.CreateOrUpdate(context.Background(), nil)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("client is nil"))
		})

		It("should return an error when secret resolution fails", func() {
			cl := newValidClient()

			overrideSecretsGet(func(_ context.Context, _ string) (string, error) {
				return "", fmt.Errorf("secret manager unavailable")
			})

			handler := NewHandlerClient(keycloak.NewServiceFactory())
			err := handler.CreateOrUpdate(ctx, cl)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to get client secret from secret-manager"))
		})

		It("should return a BlockedError when the realm is not found", func() {
			cl := newValidClient()

			overrideSecretsGet(func(_ context.Context, _ string) (string, error) {
				return "resolved-secret", nil
			})

			By("simulating realm not found")
			mockK8s.EXPECT().
				Get(mock.Anything, types.NamespacedName{Namespace: "default", Name: "test-realm"}, mock.AnythingOfType("*v1.Realm"), mock.Anything).
				Return(&notFoundError{})

			handler := NewHandlerClient(keycloak.NewServiceFactory())
			err := handler.CreateOrUpdate(ctx, cl)

			By("expecting a BlockedError")
			Expect(err).To(HaveOccurred())
			var be ctrlerrors.BlockedError
			Expect(errors.As(err, &be)).To(BeTrue(), "error should be a BlockedError")
			Expect(err.Error()).To(ContainSubstring("not found"))
		})

		It("should return a BlockedError when the realm is not ready", func() {
			cl := newValidClient()

			overrideSecretsGet(func(_ context.Context, _ string) (string, error) {
				return "resolved-secret", nil
			})

			By("returning a Realm without Ready condition")
			realmNotReady := &identityv1.Realm{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-realm",
					Namespace: "default",
				},
				Status: identityv1.RealmStatus{
					AdminPassword: "some-pw",
				},
			}
			mockRealmGet(mockK8s, realmNotReady)

			handler := NewHandlerClient(keycloak.NewServiceFactory())
			err := handler.CreateOrUpdate(ctx, cl)

			By("expecting a BlockedError")
			Expect(err).To(HaveOccurred())
			var be ctrlerrors.BlockedError
			Expect(errors.As(err, &be)).To(BeTrue(), "error should be a BlockedError")
			Expect(err.Error()).To(ContainSubstring("not ready"))
		})

		It("should return a BlockedError when realm validation fails", func() {
			cl := newValidClient()

			overrideSecretsGet(func(_ context.Context, _ string) (string, error) {
				return "resolved-secret", nil
			})

			By("returning a Ready Realm with empty required fields")
			realmInvalid := &identityv1.Realm{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-realm",
					Namespace: "default",
				},
				Status: identityv1.RealmStatus{
					AdminPassword: "resolved-password",
				},
			}
			realmInvalid.SetCondition(condition.NewReadyCondition("Ready", "Realm is ready"))
			mockRealmGet(mockK8s, realmInvalid)

			handler := NewHandlerClient(keycloak.NewServiceFactory())
			err := handler.CreateOrUpdate(ctx, cl)

			Expect(err).To(HaveOccurred())
			var be ctrlerrors.BlockedError
			Expect(errors.As(err, &be)).To(BeTrue(), "error should be a BlockedError")
			Expect(err.Error()).To(ContainSubstring("not valid"))
		})

		It("should return an error when the client factory fails", func() {
			cl := newValidClient()
			realm := newValidRealm()

			overrideSecretsGet(func(_ context.Context, _ string) (string, error) {
				return "resolved-secret", nil
			})
			mockRealmGet(mockK8s, realm)

			factoryErr := fmt.Errorf("oauth2 token failure")
			factory := keycloak.ServiceFactoryFunc(func(_ identityv1.RealmStatus) (keycloak.KeycloakService, error) {
				return nil, factoryErr
			})

			handler := NewHandlerClient(factory)
			err := handler.CreateOrUpdate(ctx, cl)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to get keycloak client"))
			Expect(errors.Is(err, factoryErr)).To(BeTrue())
		})

		It("should return an error when Keycloak CreateOrReplaceClient fails", func() {
			cl := newValidClient()
			realm := newValidRealm()

			overrideSecretsGet(func(_ context.Context, _ string) (string, error) {
				return "resolved-secret", nil
			})
			mockRealmGet(mockK8s, realm)

			mockSvc := keycloakservice.NewMockKeycloakService(GinkgoT())
			mockSvc.EXPECT().
				CreateOrReplaceClient(mock.Anything, mock.Anything, mock.Anything, mock.Anything).
				Return(fmt.Errorf("keycloak 503: service unavailable"))

			factory := keycloak.ServiceFactoryFunc(func(_ identityv1.RealmStatus) (keycloak.KeycloakService, error) {
				return mockSvc, nil
			})

			handler := NewHandlerClient(factory)
			err := handler.CreateOrUpdate(ctx, cl)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to create or update client"))
			Expect(err.Error()).To(ContainSubstring("keycloak 503"))
		})

		It("should set Ready condition and status on success", func() {
			cl := newValidClient()
			cl.Annotations = map[string]string{
				identityv1.DisableSecretRotationAnnotation: "true",
			}
			realm := newValidRealm()

			overrideSecretsGet(func(_ context.Context, _ string) (string, error) {
				return "resolved-secret", nil
			})
			mockRealmGet(mockK8s, realm)

			mockSvc := keycloakservice.NewMockKeycloakService(GinkgoT())
			mockSvc.EXPECT().
				CreateOrReplaceClient(mock.Anything, mock.Anything, mock.Anything, mock.Anything).
				Return(nil)
			// No GetClientSecretRotationInfo call expected — SecretRotation is not enabled.

			factory := keycloak.ServiceFactoryFunc(func(_ identityv1.RealmStatus) (keycloak.KeycloakService, error) {
				return mockSvc, nil
			})

			handler := NewHandlerClient(factory)
			err := handler.CreateOrUpdate(ctx, cl)

			Expect(err).ToNot(HaveOccurred())

			By("verifying the Ready condition is set")
			conditions := cl.GetConditions()
			Expect(conditions).ToNot(BeEmpty())

			var readyFound bool
			for _, c := range conditions {
				if c.Type == condition.ConditionTypeReady && c.Status == metav1.ConditionTrue {
					readyFound = true
				}
			}
			Expect(readyFound).To(BeTrue(), "client should have Ready=True condition after success")

			By("verifying IssuerUrl was set from realm status")
			Expect(cl.Status.IssuerUrl).To(Equal(realm.Status.IssuerUrl))

			By("verifying the original ClientSecret was not mutated")
			Expect(cl.Spec.ClientSecret).To(Equal("$<client-secret>"),
				"client.Spec.ClientSecret should not be mutated with resolved plaintext")
		})

		It("should return an error when GetClientSecretRotationInfo fails (A2)", func() {
			cl := newValidClient()

			realm := newValidRealm()
			realm.Spec.SecretRotation = &identityv1.SecretRotationConfig{
				ExpirationPeriod:        metav1.Duration{Duration: 29 * 24 * time.Hour},
				GracePeriod:             metav1.Duration{Duration: 1 * time.Hour},
				RemainingRotationPeriod: metav1.Duration{Duration: 10 * 24 * time.Hour},
			}

			overrideSecretsGet(func(_ context.Context, _ string) (string, error) {
				return "resolved-secret", nil
			})
			mockRealmGet(mockK8s, realm)

			mockSvc := keycloakservice.NewMockKeycloakService(GinkgoT())
			mockSvc.EXPECT().
				CreateOrReplaceClient(mock.Anything, mock.Anything, mock.Anything, mock.Anything).
				Return(nil)
			mockSvc.EXPECT().
				GetClientSecretRotationInfo(mock.Anything, mock.Anything, mock.Anything).
				Return((*keycloak.ClientSecretRotationInfo)(nil), fmt.Errorf("keycloak 500: internal server error"))

			factory := keycloak.ServiceFactoryFunc(func(_ identityv1.RealmStatus) (keycloak.KeycloakService, error) {
				return mockSvc, nil
			})

			handler := NewHandlerClient(factory)
			err := handler.CreateOrUpdate(ctx, cl)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to check client secret rotation info"))
			Expect(err.Error()).To(ContainSubstring("keycloak 500"))
		})

		It("should populate rotated secret status fields when rotated secret exists (A4)", func() {
			cl := newValidClient()
			cl.Spec.ClientSecret = "plain-secret" // use non-reference so rotated secret is exposed in status

			realm := newValidRealm()
			realm.Spec.SecretRotation = &identityv1.SecretRotationConfig{
				ExpirationPeriod:        metav1.Duration{Duration: 29 * 24 * time.Hour},
				GracePeriod:             metav1.Duration{Duration: 1 * time.Hour},
				RemainingRotationPeriod: metav1.Duration{Duration: 10 * 24 * time.Hour},
			}

			overrideSecretsGet(func(_ context.Context, _ string) (string, error) {
				return "resolved-secret", nil
			})
			mockRealmGet(mockK8s, realm)

			mockSvc := keycloakservice.NewMockKeycloakService(GinkgoT())
			mockSvc.EXPECT().
				CreateOrReplaceClient(mock.Anything, mock.Anything, mock.Anything, mock.Anything).
				Return(nil)
			mockSvc.EXPECT().
				GetClientSecretRotationInfo(mock.Anything, mock.Anything, mock.Anything).
				Return(&keycloak.ClientSecretRotationInfo{RotatedSecret: "old-secret-value"}, nil)

			factory := keycloak.ServiceFactoryFunc(func(_ identityv1.RealmStatus) (keycloak.KeycloakService, error) {
				return mockSvc, nil
			})

			handler := NewHandlerClient(factory)
			err := handler.CreateOrUpdate(ctx, cl)

			Expect(err).ToNot(HaveOccurred())

			By("verifying RotatedClientSecret is stored in status")
			Expect(cl.Status.RotatedClientSecret).To(Equal("old-secret-value"))

			By("verifying the client is still Ready")
			conditions := cl.GetConditions()
			var readyFound bool
			for _, c := range conditions {
				if c.Type == condition.ConditionTypeReady && c.Status == metav1.ConditionTrue {
					readyFound = true
				}
			}
			Expect(readyFound).To(BeTrue(), "client should remain Ready=True during rotation")
		})

		It("should set RotatedSecretExpiresAt directly from ExpiresAt attribute", func() {
			cl := newValidClient()
			cl.Spec.ClientSecret = "plain-secret" // use non-reference so rotated secret is exposed in status

			realm := newValidRealm()
			realm.Spec.SecretRotation = &identityv1.SecretRotationConfig{
				ExpirationPeriod:        metav1.Duration{Duration: 29 * 24 * time.Hour},
				GracePeriod:             metav1.Duration{Duration: 1 * time.Hour},
				RemainingRotationPeriod: metav1.Duration{Duration: 10 * 24 * time.Hour},
			}

			overrideSecretsGet(func(_ context.Context, _ string) (string, error) {
				return "resolved-secret", nil
			})
			mockRealmGet(mockK8s, realm)

			// Simulate a rotated secret that expires at 2025-06-16T12:05:00Z
			// = 1750075500 epoch seconds
			var expiresAt int64 = 1750075500
			var createdAt int64 = 1750075200
			mockSvc := keycloakservice.NewMockKeycloakService(GinkgoT())
			mockSvc.EXPECT().
				CreateOrReplaceClient(mock.Anything, mock.Anything, mock.Anything, mock.Anything).
				Return(nil)
			mockSvc.EXPECT().
				GetClientSecretRotationInfo(mock.Anything, mock.Anything, mock.Anything).
				Return(&keycloak.ClientSecretRotationInfo{RotatedSecret: "old-secret-value", RotatedCreatedAt: &createdAt, RotatedExpiresAt: &expiresAt}, nil)

			factory := keycloak.ServiceFactoryFunc(func(_ identityv1.RealmStatus) (keycloak.KeycloakService, error) {
				return mockSvc, nil
			})

			handler := NewHandlerClient(factory)
			err := handler.CreateOrUpdate(ctx, cl)

			Expect(err).ToNot(HaveOccurred())

			By("verifying RotatedSecretExpiresAt is set directly from ExpiresAt")
			expectedTime := time.Date(2025, 6, 16, 12, 5, 0, 0, time.UTC)
			Expect(cl.Status.RotatedSecretExpiresAt).ToNot(BeNil())
			Expect(cl.Status.RotatedSecretExpiresAt.Time.Equal(expectedTime)).To(BeTrue(),
				"expected %v but got %v", expectedTime, cl.Status.RotatedSecretExpiresAt.Time)

			By("verifying RotatedClientSecret is stored")
			Expect(cl.Status.RotatedClientSecret).To(Equal("old-secret-value"))
		})

		It("should not set RotatedSecretExpiresAt when ExpiresAt is nil", func() {
			cl := newValidClient()
			cl.Spec.ClientSecret = "plain-secret" // use non-reference so rotated secret is exposed in status

			realm := newValidRealm()
			realm.Spec.SecretRotation = &identityv1.SecretRotationConfig{
				ExpirationPeriod:        metav1.Duration{Duration: 29 * 24 * time.Hour},
				GracePeriod:             metav1.Duration{Duration: 1 * time.Hour},
				RemainingRotationPeriod: metav1.Duration{Duration: 10 * 24 * time.Hour},
			}

			overrideSecretsGet(func(_ context.Context, _ string) (string, error) {
				return "resolved-secret", nil
			})
			mockRealmGet(mockK8s, realm)

			// ExpiresAt is nil — only CreatedAt is set
			var createdAt int64 = 1750075200
			mockSvc := keycloakservice.NewMockKeycloakService(GinkgoT())
			mockSvc.EXPECT().
				CreateOrReplaceClient(mock.Anything, mock.Anything, mock.Anything, mock.Anything).
				Return(nil)
			mockSvc.EXPECT().
				GetClientSecretRotationInfo(mock.Anything, mock.Anything, mock.Anything).
				Return(&keycloak.ClientSecretRotationInfo{RotatedSecret: "old-secret-value", RotatedCreatedAt: &createdAt}, nil)

			factory := keycloak.ServiceFactoryFunc(func(_ identityv1.RealmStatus) (keycloak.KeycloakService, error) {
				return mockSvc, nil
			})

			handler := NewHandlerClient(factory)
			err := handler.CreateOrUpdate(ctx, cl)

			Expect(err).ToNot(HaveOccurred())

			By("verifying RotatedSecretExpiresAt is nil (no ExpiresAt in RotatedSecretInfo)")
			Expect(cl.Status.RotatedSecretExpiresAt).To(BeNil())

			By("verifying RotatedClientSecret is still stored")
			Expect(cl.Status.RotatedClientSecret).To(Equal("old-secret-value"))
		})

		It("should not set RotatedSecretExpiresAt when both CreatedAt and ExpiresAt are nil", func() {
			cl := newValidClient()
			cl.Spec.ClientSecret = "plain-secret" // use non-reference so rotated secret is exposed in status

			realm := newValidRealm()
			realm.Spec.SecretRotation = &identityv1.SecretRotationConfig{
				ExpirationPeriod:        metav1.Duration{Duration: 29 * 24 * time.Hour},
				GracePeriod:             metav1.Duration{Duration: 1 * time.Hour},
				RemainingRotationPeriod: metav1.Duration{Duration: 10 * 24 * time.Hour},
			}

			overrideSecretsGet(func(_ context.Context, _ string) (string, error) {
				return "resolved-secret", nil
			})
			mockRealmGet(mockK8s, realm)

			mockSvc := keycloakservice.NewMockKeycloakService(GinkgoT())
			mockSvc.EXPECT().
				CreateOrReplaceClient(mock.Anything, mock.Anything, mock.Anything, mock.Anything).
				Return(nil)
			mockSvc.EXPECT().
				GetClientSecretRotationInfo(mock.Anything, mock.Anything, mock.Anything).
				Return(&keycloak.ClientSecretRotationInfo{RotatedSecret: "old-secret-value"}, nil)

			factory := keycloak.ServiceFactoryFunc(func(_ identityv1.RealmStatus) (keycloak.KeycloakService, error) {
				return mockSvc, nil
			})

			handler := NewHandlerClient(factory)
			err := handler.CreateOrUpdate(ctx, cl)

			Expect(err).ToNot(HaveOccurred())

			By("verifying RotatedSecretExpiresAt is nil (no timestamps available)")
			Expect(cl.Status.RotatedSecretExpiresAt).To(BeNil())

			By("verifying RotatedClientSecret is still stored")
			Expect(cl.Status.RotatedClientSecret).To(Equal("old-secret-value"))
		})

		It("should set SecretExpiresAt when creation time attribute is present", func() {
			cl := newValidClient()
			cl.Spec.ClientSecret = "plain-secret"

			realm := newValidRealm()
			realm.Spec.SecretRotation = &identityv1.SecretRotationConfig{
				ExpirationPeriod:        metav1.Duration{Duration: 29 * 24 * time.Hour}, // 29 days
				GracePeriod:             metav1.Duration{Duration: 1 * time.Hour},
				RemainingRotationPeriod: metav1.Duration{Duration: 10 * 24 * time.Hour},
			}

			overrideSecretsGet(func(_ context.Context, _ string) (string, error) {
				return "resolved-secret", nil
			})
			mockRealmGet(mockK8s, realm)

			// Secret created at 2025-06-16T12:00:00Z = 1750075200
			var creationEpoch int64 = 1750075200
			mockSvc := keycloakservice.NewMockKeycloakService(GinkgoT())
			mockSvc.EXPECT().
				CreateOrReplaceClient(mock.Anything, mock.Anything, mock.Anything, mock.Anything).
				Return(nil)
			mockSvc.EXPECT().
				GetClientSecretRotationInfo(mock.Anything, mock.Anything, mock.Anything).
				Return(&keycloak.ClientSecretRotationInfo{SecretCreationTime: &creationEpoch}, nil)

			factory := keycloak.ServiceFactoryFunc(func(_ identityv1.RealmStatus) (keycloak.KeycloakService, error) {
				return mockSvc, nil
			})

			handler := NewHandlerClient(factory)
			err := handler.CreateOrUpdate(ctx, cl)

			Expect(err).ToNot(HaveOccurred())

			By("verifying SecretExpiresAt = creationTime + expirationPeriod")
			expectedExpiry := time.Unix(creationEpoch, 0).Add(29 * 24 * time.Hour).UTC()
			Expect(cl.Status.SecretExpiresAt).ToNot(BeNil())
			Expect(cl.Status.SecretExpiresAt.Time.Equal(expectedExpiry)).To(BeTrue(),
				"expected %v but got %v", expectedExpiry, cl.Status.SecretExpiresAt.Time)
		})

		It("should set SecretExpiresAt to nil when creation time attribute is missing", func() {
			cl := newValidClient()
			cl.Spec.ClientSecret = "plain-secret"

			realm := newValidRealm()
			realm.Spec.SecretRotation = &identityv1.SecretRotationConfig{
				ExpirationPeriod:        metav1.Duration{Duration: 29 * 24 * time.Hour},
				GracePeriod:             metav1.Duration{Duration: 1 * time.Hour},
				RemainingRotationPeriod: metav1.Duration{Duration: 10 * 24 * time.Hour},
			}

			overrideSecretsGet(func(_ context.Context, _ string) (string, error) {
				return "resolved-secret", nil
			})
			mockRealmGet(mockK8s, realm)

			mockSvc := keycloakservice.NewMockKeycloakService(GinkgoT())
			mockSvc.EXPECT().
				CreateOrReplaceClient(mock.Anything, mock.Anything, mock.Anything, mock.Anything).
				Return(nil)
			mockSvc.EXPECT().
				GetClientSecretRotationInfo(mock.Anything, mock.Anything, mock.Anything).
				Return(&keycloak.ClientSecretRotationInfo{}, nil)

			factory := keycloak.ServiceFactoryFunc(func(_ identityv1.RealmStatus) (keycloak.KeycloakService, error) {
				return mockSvc, nil
			})

			handler := NewHandlerClient(factory)
			err := handler.CreateOrUpdate(ctx, cl)

			Expect(err).ToNot(HaveOccurred())
			Expect(cl.Status.SecretExpiresAt).To(BeNil())
		})

		It("should set SecretExpiresAt to nil when realm has no SecretRotation config", func() {
			cl := newValidClient()
			cl.Spec.ClientSecret = "plain-secret"

			realm := newValidRealm()
			// realm.Spec.SecretRotation is nil

			overrideSecretsGet(func(_ context.Context, _ string) (string, error) {
				return "resolved-secret", nil
			})
			mockRealmGet(mockK8s, realm)

			mockSvc := keycloakservice.NewMockKeycloakService(GinkgoT())
			mockSvc.EXPECT().
				CreateOrReplaceClient(mock.Anything, mock.Anything, mock.Anything, mock.Anything).
				Return(nil)
			// No GetClientSecretRotationInfo call expected — realm has no SecretRotation config.

			factory := keycloak.ServiceFactoryFunc(func(_ identityv1.RealmStatus) (keycloak.KeycloakService, error) {
				return mockSvc, nil
			})

			handler := NewHandlerClient(factory)
			err := handler.CreateOrUpdate(ctx, cl)

			Expect(err).ToNot(HaveOccurred())
			Expect(cl.Status.SecretExpiresAt).To(BeNil())
		})

		// --- SecretRotation lifecycle: condition handling & idempotency ---

		It("should set Accepted condition and pass correct ClientUpdateOptions", func() {
			cl := newValidClient()
			cl.Spec.ClientSecret = "plain-secret"

			realm := newValidRealm()
			realm.Spec.SecretRotation = &identityv1.SecretRotationConfig{
				ExpirationPeriod:        metav1.Duration{Duration: 29 * 24 * time.Hour},
				GracePeriod:             metav1.Duration{Duration: 1 * time.Hour},
				RemainingRotationPeriod: metav1.Duration{Duration: 10 * 24 * time.Hour},
			}

			overrideSecretsGet(func(_ context.Context, _ string) (string, error) {
				return "resolved-secret", nil
			})
			mockRealmGet(mockK8s, realm)

			var capturedOpts keycloak.ClientUpdateOptions
			mockSvc := keycloakservice.NewMockKeycloakService(GinkgoT())
			mockSvc.EXPECT().
				CreateOrReplaceClient(mock.Anything, mock.Anything, mock.Anything, mock.Anything).
				Run(func(_ context.Context, _ string, _ *identityv1.Client, opts keycloak.ClientUpdateOptions) {
					capturedOpts = opts
				}).
				Return(nil)
			mockSvc.EXPECT().
				GetClientSecretRotationInfo(mock.Anything, mock.Anything, mock.Anything).
				Return(&keycloak.ClientSecretRotationInfo{}, nil)

			factory := keycloak.ServiceFactoryFunc(func(_ identityv1.RealmStatus) (keycloak.KeycloakService, error) {
				return mockSvc, nil
			})

			handler := NewHandlerClient(factory)
			err := handler.CreateOrUpdate(ctx, cl)

			Expect(err).ToNot(HaveOccurred())

			By("verifying ClientUpdateOptions flags")
			Expect(capturedOpts.SupportsGracefulRotation).To(BeTrue())
			Expect(capturedOpts.SkipForceRotation).To(BeFalse())

			By("verifying SecretRotation condition was set")
			cond := meta.FindStatusCondition(cl.GetConditions(), identityv1.SecretRotationConditionType)
			Expect(cond).ToNot(BeNil())
			// The handler sets Accepted before CreateOrReplaceClient (line 93).
			// Because GetClientSecretRotationInfo returns no rotated secret and the
			// existing condition has reason Accepted, the handler transitions it to
			// Completed (line 143-144).
			Expect(cond.Reason).To(Equal(identityv1.SecretRotationReasonCompleted))
		})

		It("should skip force rotation when Accepted condition matches current generation", func() {
			cl := newValidClient()
			cl.Spec.ClientSecret = "plain-secret"
			cl.Generation = 3

			// Pre-set Accepted condition with matching ObservedGeneration.
			meta.SetStatusCondition(&cl.Status.Conditions, metav1.Condition{
				Type:               identityv1.SecretRotationConditionType,
				Status:             metav1.ConditionTrue,
				Reason:             identityv1.SecretRotationReasonAccepted,
				Message:            "Secret change detected, rotation accepted",
				LastTransitionTime: metav1.Now(),
				ObservedGeneration: 3,
			})

			realm := newValidRealm()
			realm.Spec.SecretRotation = &identityv1.SecretRotationConfig{
				ExpirationPeriod:        metav1.Duration{Duration: 29 * 24 * time.Hour},
				GracePeriod:             metav1.Duration{Duration: 1 * time.Hour},
				RemainingRotationPeriod: metav1.Duration{Duration: 10 * 24 * time.Hour},
			}

			overrideSecretsGet(func(_ context.Context, _ string) (string, error) {
				return "resolved-secret", nil
			})
			mockRealmGet(mockK8s, realm)

			var capturedOpts keycloak.ClientUpdateOptions
			mockSvc := keycloakservice.NewMockKeycloakService(GinkgoT())
			mockSvc.EXPECT().
				CreateOrReplaceClient(mock.Anything, mock.Anything, mock.Anything, mock.Anything).
				Run(func(_ context.Context, _ string, _ *identityv1.Client, opts keycloak.ClientUpdateOptions) {
					capturedOpts = opts
				}).
				Return(nil)
			mockSvc.EXPECT().
				GetClientSecretRotationInfo(mock.Anything, mock.Anything, mock.Anything).
				Return(&keycloak.ClientSecretRotationInfo{}, nil)

			factory := keycloak.ServiceFactoryFunc(func(_ identityv1.RealmStatus) (keycloak.KeycloakService, error) {
				return mockSvc, nil
			})

			handler := NewHandlerClient(factory)
			err := handler.CreateOrUpdate(ctx, cl)

			Expect(err).ToNot(HaveOccurred())

			By("verifying SkipForceRotation is true")
			Expect(capturedOpts.SkipForceRotation).To(BeTrue())
			Expect(capturedOpts.SupportsGracefulRotation).To(BeTrue())
		})

		It("should not skip force rotation when Accepted condition has stale generation", func() {
			cl := newValidClient()
			cl.Spec.ClientSecret = "plain-secret"
			cl.Generation = 5

			// Pre-set Accepted condition with stale ObservedGeneration.
			meta.SetStatusCondition(&cl.Status.Conditions, metav1.Condition{
				Type:               identityv1.SecretRotationConditionType,
				Status:             metav1.ConditionTrue,
				Reason:             identityv1.SecretRotationReasonAccepted,
				Message:            "Secret change detected, rotation accepted",
				LastTransitionTime: metav1.Now(),
				ObservedGeneration: 4, // stale
			})

			realm := newValidRealm()
			realm.Spec.SecretRotation = &identityv1.SecretRotationConfig{
				ExpirationPeriod:        metav1.Duration{Duration: 29 * 24 * time.Hour},
				GracePeriod:             metav1.Duration{Duration: 1 * time.Hour},
				RemainingRotationPeriod: metav1.Duration{Duration: 10 * 24 * time.Hour},
			}

			overrideSecretsGet(func(_ context.Context, _ string) (string, error) {
				return "resolved-secret", nil
			})
			mockRealmGet(mockK8s, realm)

			var capturedOpts keycloak.ClientUpdateOptions
			mockSvc := keycloakservice.NewMockKeycloakService(GinkgoT())
			mockSvc.EXPECT().
				CreateOrReplaceClient(mock.Anything, mock.Anything, mock.Anything, mock.Anything).
				Run(func(_ context.Context, _ string, _ *identityv1.Client, opts keycloak.ClientUpdateOptions) {
					capturedOpts = opts
				}).
				Return(nil)
			mockSvc.EXPECT().
				GetClientSecretRotationInfo(mock.Anything, mock.Anything, mock.Anything).
				Return(&keycloak.ClientSecretRotationInfo{}, nil)

			factory := keycloak.ServiceFactoryFunc(func(_ identityv1.RealmStatus) (keycloak.KeycloakService, error) {
				return mockSvc, nil
			})

			handler := NewHandlerClient(factory)
			err := handler.CreateOrUpdate(ctx, cl)

			Expect(err).ToNot(HaveOccurred())

			By("verifying SkipForceRotation is false (stale generation)")
			Expect(capturedOpts.SkipForceRotation).To(BeFalse())
		})

		It("should preserve Accepted condition when CreateOrReplaceClient fails", func() {
			cl := newValidClient()
			cl.Spec.ClientSecret = "plain-secret"

			realm := newValidRealm()
			realm.Spec.SecretRotation = &identityv1.SecretRotationConfig{
				ExpirationPeriod:        metav1.Duration{Duration: 29 * 24 * time.Hour},
				GracePeriod:             metav1.Duration{Duration: 1 * time.Hour},
				RemainingRotationPeriod: metav1.Duration{Duration: 10 * 24 * time.Hour},
			}

			overrideSecretsGet(func(_ context.Context, _ string) (string, error) {
				return "resolved-secret", nil
			})
			mockRealmGet(mockK8s, realm)

			mockSvc := keycloakservice.NewMockKeycloakService(GinkgoT())
			mockSvc.EXPECT().
				CreateOrReplaceClient(mock.Anything, mock.Anything, mock.Anything, mock.Anything).
				Return(fmt.Errorf("keycloak 503: service unavailable"))

			factory := keycloak.ServiceFactoryFunc(func(_ identityv1.RealmStatus) (keycloak.KeycloakService, error) {
				return mockSvc, nil
			})

			handler := NewHandlerClient(factory)
			err := handler.CreateOrUpdate(ctx, cl)

			Expect(err).To(HaveOccurred())

			By("verifying Accepted condition is still present for retry idempotency")
			cond := meta.FindStatusCondition(cl.GetConditions(), identityv1.SecretRotationConditionType)
			Expect(cond).ToNot(BeNil())
			Expect(cond.Reason).To(Equal(identityv1.SecretRotationReasonAccepted))
			Expect(cond.Status).To(Equal(metav1.ConditionTrue))
		})

		It("should set Completed condition when rotated secret disappears after Rotated state", func() {
			cl := newValidClient()
			cl.Spec.ClientSecret = "plain-secret"

			// Pre-set Rotated condition (simulating active grace period that has now expired).
			meta.SetStatusCondition(&cl.Status.Conditions, metav1.Condition{
				Type:               identityv1.SecretRotationConditionType,
				Status:             metav1.ConditionTrue,
				Reason:             identityv1.SecretRotationReasonRotated,
				Message:            "Client secret was rotated at 2025-06-16T12:00:00Z",
				LastTransitionTime: metav1.Now(),
			})

			realm := newValidRealm()
			realm.Spec.SecretRotation = &identityv1.SecretRotationConfig{
				ExpirationPeriod:        metav1.Duration{Duration: 29 * 24 * time.Hour},
				GracePeriod:             metav1.Duration{Duration: 1 * time.Hour},
				RemainingRotationPeriod: metav1.Duration{Duration: 10 * 24 * time.Hour},
			}

			overrideSecretsGet(func(_ context.Context, _ string) (string, error) {
				return "resolved-secret", nil
			})
			mockRealmGet(mockK8s, realm)

			mockSvc := keycloakservice.NewMockKeycloakService(GinkgoT())
			mockSvc.EXPECT().
				CreateOrReplaceClient(mock.Anything, mock.Anything, mock.Anything, mock.Anything).
				Return(nil)
			mockSvc.EXPECT().
				GetClientSecretRotationInfo(mock.Anything, mock.Anything, mock.Anything).
				Return(&keycloak.ClientSecretRotationInfo{}, nil) // no rotated secret

			factory := keycloak.ServiceFactoryFunc(func(_ identityv1.RealmStatus) (keycloak.KeycloakService, error) {
				return mockSvc, nil
			})

			handler := NewHandlerClient(factory)
			err := handler.CreateOrUpdate(ctx, cl)

			Expect(err).ToNot(HaveOccurred())

			By("verifying condition transitioned to Completed")
			cond := meta.FindStatusCondition(cl.GetConditions(), identityv1.SecretRotationConditionType)
			Expect(cond).ToNot(BeNil())
			Expect(cond.Reason).To(Equal(identityv1.SecretRotationReasonCompleted))
			Expect(cond.Status).To(Equal(metav1.ConditionFalse))

			By("verifying rotation status fields are cleared")
			Expect(cl.Status.RotatedClientSecret).To(BeEmpty())
			Expect(cl.Status.RotatedSecretExpiresAt).To(BeNil())
		})

		It("should set Completed condition when rotated secret disappears after Accepted state", func() {
			cl := newValidClient()
			cl.Spec.ClientSecret = "plain-secret"
			cl.Generation = 2

			// Pre-set Accepted condition with matching generation (simulating retry).
			meta.SetStatusCondition(&cl.Status.Conditions, metav1.Condition{
				Type:               identityv1.SecretRotationConditionType,
				Status:             metav1.ConditionTrue,
				Reason:             identityv1.SecretRotationReasonAccepted,
				Message:            "Secret change detected, rotation accepted",
				LastTransitionTime: metav1.Now(),
				ObservedGeneration: 2,
			})

			realm := newValidRealm()
			realm.Spec.SecretRotation = &identityv1.SecretRotationConfig{
				ExpirationPeriod:        metav1.Duration{Duration: 29 * 24 * time.Hour},
				GracePeriod:             metav1.Duration{Duration: 1 * time.Hour},
				RemainingRotationPeriod: metav1.Duration{Duration: 10 * 24 * time.Hour},
			}

			overrideSecretsGet(func(_ context.Context, _ string) (string, error) {
				return "resolved-secret", nil
			})
			mockRealmGet(mockK8s, realm)

			mockSvc := keycloakservice.NewMockKeycloakService(GinkgoT())
			mockSvc.EXPECT().
				CreateOrReplaceClient(mock.Anything, mock.Anything, mock.Anything, mock.Anything).
				Return(nil)
			mockSvc.EXPECT().
				GetClientSecretRotationInfo(mock.Anything, mock.Anything, mock.Anything).
				Return(&keycloak.ClientSecretRotationInfo{}, nil)

			factory := keycloak.ServiceFactoryFunc(func(_ identityv1.RealmStatus) (keycloak.KeycloakService, error) {
				return mockSvc, nil
			})

			handler := NewHandlerClient(factory)
			err := handler.CreateOrUpdate(ctx, cl)

			Expect(err).ToNot(HaveOccurred())

			By("verifying condition transitioned to Completed from Accepted")
			cond := meta.FindStatusCondition(cl.GetConditions(), identityv1.SecretRotationConditionType)
			Expect(cond).ToNot(BeNil())
			Expect(cond.Reason).To(Equal(identityv1.SecretRotationReasonCompleted))
			Expect(cond.Status).To(Equal(metav1.ConditionFalse))
		})

		It("should transition to Completed when first rotation produces no rotated secret", func() {
			cl := newValidClient()
			cl.Spec.ClientSecret = "plain-secret"

			realm := newValidRealm()
			realm.Spec.SecretRotation = &identityv1.SecretRotationConfig{
				ExpirationPeriod:        metav1.Duration{Duration: 29 * 24 * time.Hour},
				GracePeriod:             metav1.Duration{Duration: 1 * time.Hour},
				RemainingRotationPeriod: metav1.Duration{Duration: 10 * 24 * time.Hour},
			}

			overrideSecretsGet(func(_ context.Context, _ string) (string, error) {
				return "resolved-secret", nil
			})
			mockRealmGet(mockK8s, realm)

			mockSvc := keycloakservice.NewMockKeycloakService(GinkgoT())
			mockSvc.EXPECT().
				CreateOrReplaceClient(mock.Anything, mock.Anything, mock.Anything, mock.Anything).
				Return(nil)
			mockSvc.EXPECT().
				GetClientSecretRotationInfo(mock.Anything, mock.Anything, mock.Anything).
				Return(&keycloak.ClientSecretRotationInfo{}, nil)

			factory := keycloak.ServiceFactoryFunc(func(_ identityv1.RealmStatus) (keycloak.KeycloakService, error) {
				return mockSvc, nil
			})

			handler := NewHandlerClient(factory)
			err := handler.CreateOrUpdate(ctx, cl)

			Expect(err).ToNot(HaveOccurred())

			By("verifying condition transitions from Accepted (set before CreateOrReplaceClient) to Completed")
			// The handler sets Accepted at line 93. When GetClientSecretRotationInfo
			// returns no rotated secret and the existing condition has reason Accepted,
			// line 143-144 transitions it to Completed. This is correct for a rotation
			// where the old and new secret happen to be the same.
			cond := meta.FindStatusCondition(cl.GetConditions(), identityv1.SecretRotationConditionType)
			Expect(cond).ToNot(BeNil())
			Expect(cond.Reason).To(Equal(identityv1.SecretRotationReasonCompleted))
			Expect(cond.Status).To(Equal(metav1.ConditionFalse))
		})

		It("should not expose rotated secret in status when clientSecret is a secret-manager reference", func() {
			cl := newValidClient()
			// cl.Spec.ClientSecret is "$<client-secret>" by default (a secret-manager ref)
			Expect(secrets.IsRef(cl.Spec.ClientSecret)).To(BeTrue(), "precondition: default client uses secret-manager ref")

			realm := newValidRealm()
			realm.Spec.SecretRotation = &identityv1.SecretRotationConfig{
				ExpirationPeriod:        metav1.Duration{Duration: 29 * 24 * time.Hour},
				GracePeriod:             metav1.Duration{Duration: 1 * time.Hour},
				RemainingRotationPeriod: metav1.Duration{Duration: 10 * 24 * time.Hour},
			}

			overrideSecretsGet(func(_ context.Context, _ string) (string, error) {
				return "resolved-secret", nil
			})
			mockRealmGet(mockK8s, realm)

			var expiresAt int64 = 1750078800
			var createdAt int64 = 1750075200
			mockSvc := keycloakservice.NewMockKeycloakService(GinkgoT())
			mockSvc.EXPECT().
				CreateOrReplaceClient(mock.Anything, mock.Anything, mock.Anything, mock.Anything).
				Return(nil)
			mockSvc.EXPECT().
				GetClientSecretRotationInfo(mock.Anything, mock.Anything, mock.Anything).
				Return(&keycloak.ClientSecretRotationInfo{
					RotatedSecret:    "old-secret-value",
					RotatedCreatedAt: &createdAt,
					RotatedExpiresAt: &expiresAt,
				}, nil)

			factory := keycloak.ServiceFactoryFunc(func(_ identityv1.RealmStatus) (keycloak.KeycloakService, error) {
				return mockSvc, nil
			})

			handler := NewHandlerClient(factory)
			err := handler.CreateOrUpdate(ctx, cl)

			Expect(err).ToNot(HaveOccurred())

			By("verifying rotated secret is suppressed in status")
			Expect(cl.Status.RotatedClientSecret).To(BeEmpty(),
				"rotated secret should not be exposed when clientSecret is a secret-manager reference")

			By("verifying metadata (expiry) is still set")
			Expect(cl.Status.RotatedSecretExpiresAt).ToNot(BeNil(),
				"rotation metadata should still be populated")
		})

		It("should preserve Accepted condition when GetClientSecretRotationInfo fails", func() {
			cl := newValidClient()
			cl.Spec.ClientSecret = "plain-secret"

			realm := newValidRealm()
			realm.Spec.SecretRotation = &identityv1.SecretRotationConfig{
				ExpirationPeriod:        metav1.Duration{Duration: 29 * 24 * time.Hour},
				GracePeriod:             metav1.Duration{Duration: 1 * time.Hour},
				RemainingRotationPeriod: metav1.Duration{Duration: 10 * 24 * time.Hour},
			}

			overrideSecretsGet(func(_ context.Context, _ string) (string, error) {
				return "resolved-secret", nil
			})
			mockRealmGet(mockK8s, realm)

			mockSvc := keycloakservice.NewMockKeycloakService(GinkgoT())
			mockSvc.EXPECT().
				CreateOrReplaceClient(mock.Anything, mock.Anything, mock.Anything, mock.Anything).
				Return(nil)
			mockSvc.EXPECT().
				GetClientSecretRotationInfo(mock.Anything, mock.Anything, mock.Anything).
				Return((*keycloak.ClientSecretRotationInfo)(nil), fmt.Errorf("keycloak 500"))

			factory := keycloak.ServiceFactoryFunc(func(_ identityv1.RealmStatus) (keycloak.KeycloakService, error) {
				return mockSvc, nil
			})

			handler := NewHandlerClient(factory)
			err := handler.CreateOrUpdate(ctx, cl)

			Expect(err).To(HaveOccurred())

			By("verifying Accepted condition survives for retry")
			cond := meta.FindStatusCondition(cl.GetConditions(), identityv1.SecretRotationConditionType)
			Expect(cond).ToNot(BeNil())
			Expect(cond.Reason).To(Equal(identityv1.SecretRotationReasonAccepted))
		})

		It("should clear stale rotation fields when SecretRotation is disabled", func() {
			cl := newValidClient()
			cl.Annotations = map[string]string{
				identityv1.DisableSecretRotationAnnotation: "true",
			}
			// Simulate a client that previously had rotation enabled and has stale status fields.
			cl.Status.RotatedClientSecret = "stale-old-secret"
			cl.Status.RotatedSecretExpiresAt = &metav1.Time{Time: time.Now()}
			realm := newValidRealm()

			overrideSecretsGet(func(_ context.Context, _ string) (string, error) {
				return "resolved-secret", nil
			})
			mockRealmGet(mockK8s, realm)

			mockSvc := keycloakservice.NewMockKeycloakService(GinkgoT())
			mockSvc.EXPECT().
				CreateOrReplaceClient(mock.Anything, mock.Anything, mock.Anything, mock.Anything).
				Return(nil)
			// No GetClientSecretRotationInfo call expected — rotation is disabled.

			factory := keycloak.ServiceFactoryFunc(func(_ identityv1.RealmStatus) (keycloak.KeycloakService, error) {
				return mockSvc, nil
			})

			handler := NewHandlerClient(factory)
			err := handler.CreateOrUpdate(ctx, cl)

			Expect(err).ToNot(HaveOccurred())

			By("verifying stale rotation fields are cleared")
			Expect(cl.Status.RotatedClientSecret).To(BeEmpty())
			Expect(cl.Status.RotatedSecretExpiresAt).To(BeNil())
			Expect(cl.Status.SecretExpiresAt).To(BeNil())
		})

	})

	Context("Delete", func() {

		BeforeEach(func() {
			overrideSecretsGet(func(_ context.Context, _ string) (string, error) {
				return "resolved-secret", nil
			})
		})

		It("should skip deletion when the realm is not found", func() {
			cl := newValidClient()

			mockK8s.EXPECT().
				Get(mock.Anything, types.NamespacedName{Namespace: "default", Name: "test-realm"}, mock.AnythingOfType("*v1.Realm"), mock.Anything).
				Return(&notFoundError{})

			handler := NewHandlerClient(keycloak.NewServiceFactory())
			err := handler.Delete(ctx, cl)

			Expect(err).ToNot(HaveOccurred())
		})

		It("should skip deletion when the realm is not ready", func() {
			cl := newValidClient()

			realmNotReady := &identityv1.Realm{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-realm",
					Namespace: "default",
				},
				Status: identityv1.RealmStatus{
					AdminPassword: "some-pw",
				},
			}
			mockRealmGet(mockK8s, realmNotReady)

			handler := NewHandlerClient(keycloak.NewServiceFactory())
			err := handler.Delete(ctx, cl)

			Expect(err).ToNot(HaveOccurred())
		})

		It("should return an error when the client factory fails", func() {
			cl := newValidClient()
			realm := newValidRealm()
			mockRealmGet(mockK8s, realm)

			factoryErr := fmt.Errorf("oauth2 token failure")
			factory := keycloak.ServiceFactoryFunc(func(_ identityv1.RealmStatus) (keycloak.KeycloakService, error) {
				return nil, factoryErr
			})

			handler := NewHandlerClient(factory)
			err := handler.Delete(ctx, cl)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to get keycloak client"))
			Expect(errors.Is(err, factoryErr)).To(BeTrue())
		})

		It("should return an error when DeleteClient fails", func() {
			cl := newValidClient()
			realm := newValidRealm()
			mockRealmGet(mockK8s, realm)

			mockSvc := keycloakservice.NewMockKeycloakService(GinkgoT())
			mockSvc.EXPECT().
				DeleteClient(mock.Anything, "test-realm", cl).
				Return(fmt.Errorf("keycloak 500: internal server error"))

			factory := keycloak.ServiceFactoryFunc(func(_ identityv1.RealmStatus) (keycloak.KeycloakService, error) {
				return mockSvc, nil
			})

			handler := NewHandlerClient(factory)
			err := handler.Delete(ctx, cl)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to delete client"))
		})

		It("should succeed when Keycloak deletion completes", func() {
			cl := newValidClient()
			realm := newValidRealm()
			mockRealmGet(mockK8s, realm)

			mockSvc := keycloakservice.NewMockKeycloakService(GinkgoT())
			mockSvc.EXPECT().
				DeleteClient(mock.Anything, "test-realm", cl).
				Return(nil)

			factory := keycloak.ServiceFactoryFunc(func(_ identityv1.RealmStatus) (keycloak.KeycloakService, error) {
				return mockSvc, nil
			})

			handler := NewHandlerClient(factory)
			err := handler.Delete(ctx, cl)

			Expect(err).ToNot(HaveOccurred())
		})
	})

	Context("mapToClientStatus", func() {

		It("should set IssuerUrl from realm status", func() {
			realmStatus := &identityv1.RealmStatus{
				IssuerUrl: "https://issuer.example.com",
			}
			var clientStatus identityv1.ClientStatus
			mapToClientStatus(realmStatus, &clientStatus)

			Expect(clientStatus.IssuerUrl).To(Equal("https://issuer.example.com"))
		})
	})
})
