// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package client

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
	"github.com/telekom/controlplane/identity/pkg/api"
	"github.com/telekom/controlplane/identity/pkg/keycloak"
	"github.com/telekom/controlplane/identity/test/mocks"
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
			handler := NewHandlerClient(keycloak.NewClientFactory())
			err := handler.CreateOrUpdate(context.Background(), nil)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("client is nil"))
		})

		It("should return an error when secret resolution fails", func() {
			cl := newValidClient()

			overrideSecretsGet(func(_ context.Context, _ string) (string, error) {
				return "", fmt.Errorf("secret manager unavailable")
			})

			handler := NewHandlerClient(keycloak.NewClientFactory())
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

			handler := NewHandlerClient(keycloak.NewClientFactory())
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

			handler := NewHandlerClient(keycloak.NewClientFactory())
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

			handler := NewHandlerClient(keycloak.NewClientFactory())
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
			factory := keycloak.ClientFactoryFunc(func(_ identityv1.RealmStatus) (keycloak.RealmClient, error) {
				return nil, factoryErr
			})

			handler := NewHandlerClient(factory)
			err := handler.CreateOrUpdate(ctx, cl)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to get keycloak client"))
			Expect(errors.Is(err, factoryErr)).To(BeTrue())
		})

		It("should return an error when Keycloak CreateOrUpdateRealmClient fails", func() {
			cl := newValidClient()
			realm := newValidRealm()

			overrideSecretsGet(func(_ context.Context, _ string) (string, error) {
				return "resolved-secret", nil
			})
			mockRealmGet(mockK8s, realm)

			mockRealmClient := mocks.NewMockRealmClient(GinkgoT())
			mockRealmClient.EXPECT().
				CreateOrUpdateRealmClient(mock.Anything, mock.Anything, mock.Anything).
				Return(fmt.Errorf("keycloak 503: service unavailable"))

			factory := keycloak.ClientFactoryFunc(func(_ identityv1.RealmStatus) (keycloak.RealmClient, error) {
				return mockRealmClient, nil
			})

			handler := NewHandlerClient(factory)
			err := handler.CreateOrUpdate(ctx, cl)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to create or update client"))
			Expect(err.Error()).To(ContainSubstring("keycloak 503"))
		})

		It("should set Ready condition and status on success", func() {
			cl := newValidClient()
			realm := newValidRealm()

			overrideSecretsGet(func(_ context.Context, _ string) (string, error) {
				return "resolved-secret", nil
			})
			mockRealmGet(mockK8s, realm)

			mockRealmClient := mocks.NewMockRealmClient(GinkgoT())
			mockRealmClient.EXPECT().
				CreateOrUpdateRealmClient(mock.Anything, mock.Anything, mock.Anything).
				Return(nil)

			factory := keycloak.ClientFactoryFunc(func(_ identityv1.RealmStatus) (keycloak.RealmClient, error) {
				return mockRealmClient, nil
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

			handler := NewHandlerClient(keycloak.NewClientFactory())
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

			handler := NewHandlerClient(keycloak.NewClientFactory())
			err := handler.Delete(ctx, cl)

			Expect(err).ToNot(HaveOccurred())
		})

		It("should return an error when the client factory fails", func() {
			cl := newValidClient()
			realm := newValidRealm()
			mockRealmGet(mockK8s, realm)

			factoryErr := fmt.Errorf("oauth2 token failure")
			factory := keycloak.ClientFactoryFunc(func(_ identityv1.RealmStatus) (keycloak.RealmClient, error) {
				return nil, factoryErr
			})

			handler := NewHandlerClient(factory)
			err := handler.Delete(ctx, cl)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to get keycloak client"))
			Expect(errors.Is(err, factoryErr)).To(BeTrue())
		})

		It("should return an error when GetRealmClients fails", func() {
			cl := newValidClient()
			realm := newValidRealm()
			mockRealmGet(mockK8s, realm)

			mockRealmClient := mocks.NewMockRealmClient(GinkgoT())
			mockRealmClient.EXPECT().
				GetRealmClients(mock.Anything, "test-realm", cl).
				Return(nil, fmt.Errorf("keycloak 500: internal server error"))

			factory := keycloak.ClientFactoryFunc(func(_ identityv1.RealmStatus) (keycloak.RealmClient, error) {
				return mockRealmClient, nil
			})

			handler := NewHandlerClient(factory)
			err := handler.Delete(ctx, cl)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to get realm clients"))
		})

		It("should return an error when DeleteRealmClient fails", func() {
			cl := newValidClient()
			realm := newValidRealm()
			mockRealmGet(mockK8s, realm)

			clientId := "keycloak-internal-id"
			existingClients := []api.ClientRepresentation{
				{Id: &clientId},
			}

			mockRealmClient := mocks.NewMockRealmClient(GinkgoT())
			mockRealmClient.EXPECT().
				GetRealmClients(mock.Anything, "test-realm", cl).
				Return(&api.GetRealmClientsResponse{JSON2XX: &existingClients}, nil)
			mockRealmClient.EXPECT().
				DeleteRealmClient(mock.Anything, "test-realm", clientId).
				Return(fmt.Errorf("keycloak 500: internal server error"))

			factory := keycloak.ClientFactoryFunc(func(_ identityv1.RealmStatus) (keycloak.RealmClient, error) {
				return mockRealmClient, nil
			})

			handler := NewHandlerClient(factory)
			err := handler.Delete(ctx, cl)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to delete realm client"))
		})

		It("should succeed when Keycloak deletion completes", func() {
			cl := newValidClient()
			realm := newValidRealm()
			mockRealmGet(mockK8s, realm)

			clientId := "keycloak-internal-id"
			existingClients := []api.ClientRepresentation{
				{Id: &clientId},
			}

			mockRealmClient := mocks.NewMockRealmClient(GinkgoT())
			mockRealmClient.EXPECT().
				GetRealmClients(mock.Anything, "test-realm", cl).
				Return(&api.GetRealmClientsResponse{JSON2XX: &existingClients}, nil)
			mockRealmClient.EXPECT().
				DeleteRealmClient(mock.Anything, "test-realm", clientId).
				Return(nil)

			factory := keycloak.ClientFactoryFunc(func(_ identityv1.RealmStatus) (keycloak.RealmClient, error) {
				return mockRealmClient, nil
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
