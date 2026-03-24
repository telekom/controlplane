// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package realm

import (
	"context"
	"errors"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/mock"
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
	"github.com/telekom/controlplane/identity/test/mocks"
	secrets "github.com/telekom/controlplane/secret-manager/api"
)

// notFoundError implements the apierrors interface for testing.
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

func newValidIdP() *identityv1.IdentityProvider {
	idp := &identityv1.IdentityProvider{
		Spec: identityv1.IdentityProviderSpec{
			AdminUrl:      "https://keycloak.example.com/auth/admin/realms/",
			AdminClientId: "admin-cli",
			AdminUserName: "admin",
			AdminPassword: "$<secret-id>",
		},
		Status: identityv1.IdentityProviderStatus{
			AdminUrl:      "https://keycloak.example.com/auth/admin/realms/",
			AdminTokenUrl: "https://keycloak.example.com/auth/realms/master/protocol/openid-connect/token",
		},
	}
	idp.SetCondition(condition.NewReadyCondition("Ready", "IdP is ready"))
	return idp
}

func newValidRealm() *identityv1.Realm {
	return &identityv1.Realm{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-realm",
			Namespace: "default",
		},
		Spec: identityv1.RealmSpec{
			IdentityProvider: &common.ObjectRef{Namespace: "default", Name: "test-idp"},
		},
	}
}

var _ = Describe("HandlerRealm", func() {

	var (
		mockK8s *fake.MockJanitorClient
		ctx     context.Context
	)

	BeforeEach(func() {
		mockK8s = fake.NewMockJanitorClient(GinkgoT())
		ctx, _ = newTestContext(mockK8s)
	})

	Context("CreateOrUpdate", func() {

		It("should return an error when the realm is nil", func() {
			handler := NewHandlerRealm(keycloak.NewClientFactory())
			err := handler.CreateOrUpdate(context.Background(), nil)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("realm is nil"))
		})

		It("should return a BlockedError when the IdP is not found", func() {
			realm := newValidRealm()

			By("simulating IdP not found")
			mockK8s.EXPECT().
				Get(mock.Anything, types.NamespacedName{Namespace: "default", Name: "test-idp"}, mock.AnythingOfType("*v1.IdentityProvider"), mock.Anything).
				Return(&notFoundError{})

			handler := NewHandlerRealm(keycloak.NewClientFactory())
			err := handler.CreateOrUpdate(ctx, realm)

			By("expecting a BlockedError")
			Expect(err).To(HaveOccurred())
			var be ctrlerrors.BlockedError
			Expect(errors.As(err, &be)).To(BeTrue(), "error should be a BlockedError")
			Expect(err.Error()).To(ContainSubstring("not found"))
		})

		It("should panic when the IdP is not ready (known latent bug)", func() {
			realm := newValidRealm()

			By("returning an IdP without Ready condition")
			idpNotReady := &identityv1.IdentityProvider{
				Spec: identityv1.IdentityProviderSpec{
					AdminUrl:      "https://keycloak.example.com/auth/admin/realms/",
					AdminClientId: "admin-cli",
					AdminUserName: "admin",
					AdminPassword: "secret",
				},
			}
			mockK8s.EXPECT().
				Get(mock.Anything, types.NamespacedName{Namespace: "default", Name: "test-idp"}, mock.AnythingOfType("*v1.IdentityProvider"), mock.Anything).
				Run(func(_ context.Context, _ types.NamespacedName, obj pkgclient.Object, _ ...pkgclient.GetOption) {
					*obj.(*identityv1.IdentityProvider) = *idpNotReady
				}).
				Return(nil)

			handler := NewHandlerRealm(keycloak.NewClientFactory())

			Expect(func() {
				_ = handler.CreateOrUpdate(ctx, realm)
			}).To(Panic(), "expected panic when IdP is not ready (nil dereference in mapToRealmStatus)")
		})

		It("should return a BlockedError when IdP validation fails", func() {
			realm := newValidRealm()

			By("returning an IdP that is Ready but has empty required fields")
			idp := &identityv1.IdentityProvider{}
			idp.SetCondition(condition.NewReadyCondition("Ready", "IdP is ready"))

			mockK8s.EXPECT().
				Get(mock.Anything, types.NamespacedName{Namespace: "default", Name: "test-idp"}, mock.AnythingOfType("*v1.IdentityProvider"), mock.Anything).
				Run(func(_ context.Context, _ types.NamespacedName, obj pkgclient.Object, _ ...pkgclient.GetOption) {
					*obj.(*identityv1.IdentityProvider) = *idp
				}).
				Return(nil)

			handler := NewHandlerRealm(keycloak.NewClientFactory())
			err := handler.CreateOrUpdate(ctx, realm)

			Expect(err).To(HaveOccurred())
			var be ctrlerrors.BlockedError
			Expect(errors.As(err, &be)).To(BeTrue(), "error should be a BlockedError")
			Expect(err.Error()).To(ContainSubstring("not valid"))
		})

		It("should return an error when secret resolution fails", func() {
			realm := newValidRealm()
			idp := newValidIdP()

			mockK8s.EXPECT().
				Get(mock.Anything, types.NamespacedName{Namespace: "default", Name: "test-idp"}, mock.AnythingOfType("*v1.IdentityProvider"), mock.Anything).
				Run(func(_ context.Context, _ types.NamespacedName, obj pkgclient.Object, _ ...pkgclient.GetOption) {
					*obj.(*identityv1.IdentityProvider) = *idp
				}).
				Return(nil)

			overrideSecretsGet(func(_ context.Context, _ string) (string, error) {
				return "", fmt.Errorf("secret manager unavailable")
			})

			handler := NewHandlerRealm(keycloak.NewClientFactory())
			err := handler.CreateOrUpdate(ctx, realm)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to retrieve password from secret manager"))
		})

		It("should return an error when the client factory fails", func() {
			realm := newValidRealm()
			idp := newValidIdP()

			mockK8s.EXPECT().
				Get(mock.Anything, types.NamespacedName{Namespace: "default", Name: "test-idp"}, mock.AnythingOfType("*v1.IdentityProvider"), mock.Anything).
				Run(func(_ context.Context, _ types.NamespacedName, obj pkgclient.Object, _ ...pkgclient.GetOption) {
					*obj.(*identityv1.IdentityProvider) = *idp
				}).
				Return(nil)

			overrideSecretsGet(func(_ context.Context, _ string) (string, error) {
				return "resolved-password", nil
			})

			factoryErr := fmt.Errorf("oauth2 token failure")
			factory := keycloak.ClientFactoryFunc(func(_ identityv1.RealmStatus) (keycloak.RealmClient, error) {
				return nil, factoryErr
			})

			handler := NewHandlerRealm(factory)
			err := handler.CreateOrUpdate(ctx, realm)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to get keycloak client"))
			Expect(errors.Is(err, factoryErr)).To(BeTrue())
		})

		It("should return an error when Keycloak CreateOrUpdateRealm fails", func() {
			realm := newValidRealm()
			idp := newValidIdP()

			mockK8s.EXPECT().
				Get(mock.Anything, types.NamespacedName{Namespace: "default", Name: "test-idp"}, mock.AnythingOfType("*v1.IdentityProvider"), mock.Anything).
				Run(func(_ context.Context, _ types.NamespacedName, obj pkgclient.Object, _ ...pkgclient.GetOption) {
					*obj.(*identityv1.IdentityProvider) = *idp
				}).
				Return(nil)

			overrideSecretsGet(func(_ context.Context, _ string) (string, error) {
				return "resolved-password", nil
			})

			mockRealmClient := mocks.NewMockRealmClient(GinkgoT())
			mockRealmClient.EXPECT().
				CreateOrUpdateRealm(mock.Anything, mock.Anything).
				Return(fmt.Errorf("keycloak 503: service unavailable"))

			factory := keycloak.ClientFactoryFunc(func(_ identityv1.RealmStatus) (keycloak.RealmClient, error) {
				return mockRealmClient, nil
			})

			handler := NewHandlerRealm(factory)
			err := handler.CreateOrUpdate(ctx, realm)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to create or update realm"))
			Expect(err.Error()).To(ContainSubstring("keycloak 503"))
		})

		It("should set Ready condition on success", func() {
			realm := newValidRealm()
			idp := newValidIdP()

			mockK8s.EXPECT().
				Get(mock.Anything, types.NamespacedName{Namespace: "default", Name: "test-idp"}, mock.AnythingOfType("*v1.IdentityProvider"), mock.Anything).
				Run(func(_ context.Context, _ types.NamespacedName, obj pkgclient.Object, _ ...pkgclient.GetOption) {
					*obj.(*identityv1.IdentityProvider) = *idp
				}).
				Return(nil)

			overrideSecretsGet(func(_ context.Context, _ string) (string, error) {
				return "resolved-password", nil
			})

			mockRealmClient := mocks.NewMockRealmClient(GinkgoT())
			mockRealmClient.EXPECT().
				CreateOrUpdateRealm(mock.Anything, mock.Anything).
				Return(nil)

			factory := keycloak.ClientFactoryFunc(func(_ identityv1.RealmStatus) (keycloak.RealmClient, error) {
				return mockRealmClient, nil
			})

			handler := NewHandlerRealm(factory)
			err := handler.CreateOrUpdate(ctx, realm)

			Expect(err).ToNot(HaveOccurred())

			By("verifying the Ready condition is set")
			conditions := realm.GetConditions()
			Expect(conditions).ToNot(BeEmpty())

			var readyFound bool
			for _, c := range conditions {
				if c.Type == condition.ConditionTypeReady && c.Status == metav1.ConditionTrue {
					readyFound = true
				}
			}
			Expect(readyFound).To(BeTrue(), "realm should have Ready=True condition after successful CreateOrUpdate")
		})
	})

	Context("Delete", func() {

		It("should succeed and not mutate the admin password", func() {
			realm := newValidRealm()
			realm.Status = identityv1.RealmStatus{
				IssuerUrl:     "https://keycloak.example.com/auth/realms/test-realm",
				AdminClientId: "admin-cli",
				AdminUserName: "admin",
				AdminPassword: "$<secret-id>",
				AdminUrl:      "https://keycloak.example.com/auth/admin/realms/",
				AdminTokenUrl: "https://keycloak.example.com/auth/realms/master/protocol/openid-connect/token",
			}

			overrideSecretsGet(func(_ context.Context, _ string) (string, error) {
				return "resolved-password", nil
			})

			mockRealmClient := mocks.NewMockRealmClient(GinkgoT())
			mockRealmClient.EXPECT().
				DeleteRealm(mock.Anything, "test-realm").
				Return(nil)

			factory := keycloak.ClientFactoryFunc(func(_ identityv1.RealmStatus) (keycloak.RealmClient, error) {
				return mockRealmClient, nil
			})

			handler := NewHandlerRealm(factory)
			err := handler.Delete(ctx, realm)

			Expect(err).ToNot(HaveOccurred())

			By("verifying the admin password was not mutated")
			Expect(realm.Status.AdminPassword).To(Equal("$<secret-id>"),
				"realm.Status.AdminPassword should not be mutated with resolved plaintext")
		})

		It("should return an error when secret resolution fails", func() {
			realm := newValidRealm()
			realm.Status.AdminPassword = "$<secret-id>"

			overrideSecretsGet(func(_ context.Context, _ string) (string, error) {
				return "", fmt.Errorf("secret manager unavailable")
			})

			handler := NewHandlerRealm(keycloak.NewClientFactory())
			err := handler.Delete(ctx, realm)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to retrieve password from secret manager"))
		})

		It("should return an error when the client factory fails", func() {
			realm := newValidRealm()
			realm.Status = identityv1.RealmStatus{
				AdminPassword: "plain-password",
				AdminUrl:      "https://keycloak.example.com/auth/admin/realms/",
				AdminTokenUrl: "https://keycloak.example.com/auth/realms/master/protocol/openid-connect/token",
				IssuerUrl:     "https://keycloak.example.com/auth/realms/test-realm",
				AdminClientId: "admin-cli",
				AdminUserName: "admin",
			}

			overrideSecretsGet(func(_ context.Context, ref string) (string, error) {
				return ref, nil
			})

			factoryErr := fmt.Errorf("oauth2 token failure")
			factory := keycloak.ClientFactoryFunc(func(_ identityv1.RealmStatus) (keycloak.RealmClient, error) {
				return nil, factoryErr
			})

			handler := NewHandlerRealm(factory)
			err := handler.Delete(ctx, realm)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to get keycloak client"))
			Expect(errors.Is(err, factoryErr)).To(BeTrue())
		})

		It("should return an error when Keycloak DeleteRealm fails", func() {
			realm := newValidRealm()
			realm.Status = identityv1.RealmStatus{
				AdminPassword: "plain-password",
				AdminUrl:      "https://keycloak.example.com/auth/admin/realms/",
				AdminTokenUrl: "https://keycloak.example.com/auth/realms/master/protocol/openid-connect/token",
				IssuerUrl:     "https://keycloak.example.com/auth/realms/test-realm",
				AdminClientId: "admin-cli",
				AdminUserName: "admin",
			}

			overrideSecretsGet(func(_ context.Context, ref string) (string, error) {
				return ref, nil
			})

			mockRealmClient := mocks.NewMockRealmClient(GinkgoT())
			mockRealmClient.EXPECT().
				DeleteRealm(mock.Anything, "test-realm").
				Return(fmt.Errorf("keycloak 500: internal server error"))

			factory := keycloak.ClientFactoryFunc(func(_ identityv1.RealmStatus) (keycloak.RealmClient, error) {
				return mockRealmClient, nil
			})

			handler := NewHandlerRealm(factory)
			err := handler.Delete(ctx, realm)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to delete realm"))
			Expect(err.Error()).To(ContainSubstring("keycloak 500"))
		})
	})

	Context("mapToRealmStatus", func() {

		It("should map IdentityProvider fields correctly", func() {
			identityProvider := &identityv1.IdentityProvider{
				Spec: identityv1.IdentityProviderSpec{
					AdminUrl:      "https://admin.example.com",
					AdminClientId: "admin-client-id",
					AdminUserName: "admin-username",
					AdminPassword: "admin-password",
				},
				Status: identityv1.IdentityProviderStatus{
					AdminUrl:      "https://admin.example.com",
					AdminTokenUrl: "https://admin.example.com/token",
				},
			}
			realmName := "test-realm"

			realmStatus := mapToRealmStatus(identityProvider, realmName)

			Expect(realmStatus.IssuerUrl).To(Equal(keycloak.DetermineIssuerUrlFrom(identityProvider.Spec.AdminUrl, realmName)))
			Expect(realmStatus.AdminClientId).To(Equal("admin-client-id"))
			Expect(realmStatus.AdminUserName).To(Equal("admin-username"))
			Expect(realmStatus.AdminPassword).To(Equal("admin-password"))
			Expect(realmStatus.AdminUrl).To(Equal("https://admin.example.com"))
			Expect(realmStatus.AdminTokenUrl).To(Equal("https://admin.example.com/token"))
		})
	})
})
