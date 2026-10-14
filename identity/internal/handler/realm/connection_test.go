// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package realm

import (
	"context"
	"fmt"

	"github.com/stretchr/testify/mock"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	pkgclient "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/client/fake"
	common "github.com/telekom/controlplane/common/pkg/types"
	identityv1 "github.com/telekom/controlplane/identity/api/v1"
	"github.com/telekom/controlplane/identity/pkg/keycloak"
	"github.com/telekom/controlplane/identity/test/mocks/keycloakservice"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// recordingFactory records ServiceFor calls and counts releases.
type recordingFactory struct {
	svc      keycloak.KeycloakService
	err      error
	realm    *identityv1.Realm
	status   identityv1.RealmStatus
	calls    int
	releases int
}

//nolint:gocritic // ServiceFactory keeps a value-based status for callers.
func (f *recordingFactory) ServiceFor(_ context.Context, realm *identityv1.Realm, status identityv1.RealmStatus) (keycloak.KeycloakService, func(), error) {
	f.calls++
	f.realm = realm
	f.status = status
	if f.err != nil {
		return nil, nil, f.err
	}
	return f.svc, func() { f.releases++ }, nil
}

var _ = Describe("HandlerRealm connection", func() {
	var (
		mockK8s *fake.MockJanitorClient
		ctx     context.Context
		idp     *identityv1.IdentityProvider
	)

	BeforeEach(func() {
		mockK8s = fake.NewMockJanitorClient(GinkgoT())
		ctx = client.WithClient(context.Background(), mockK8s)
		idp = newValidIdP()
		idp.ObjectMeta = metav1.ObjectMeta{Name: "test-idp", Namespace: "default", UID: "idp-uid"}
		overrideSecretsGet(func(_ context.Context, _ string) (string, error) {
			return "resolved-password", nil
		})
	})

	mockIdPGet := func() {
		mockK8s.EXPECT().
			Get(mock.Anything, types.NamespacedName{Namespace: "default", Name: "test-idp"}, mock.AnythingOfType("*v1.IdentityProvider"), mock.Anything).
			Run(func(_ context.Context, _ types.NamespacedName, obj pkgclient.Object, _ ...pkgclient.GetOption) {
				*obj.(*identityv1.IdentityProvider) = *idp
			}).
			Return(nil)
	}

	successfulService := func() *keycloakservice.MockKeycloakService {
		svc := keycloakservice.NewMockKeycloakService(GinkgoT())
		svc.EXPECT().CreateOrReplaceRealm(mock.Anything, mock.Anything).Return(&keycloak.RealmSyncResult{}, nil)
		svc.EXPECT().ResolveIssuerUrl(mock.Anything, "test-realm", "").Return("https://keycloak.example.com/auth/realms/test-realm", nil)
		svc.EXPECT().DeleteSecretRotationPolicy(mock.Anything, "test-realm").Return(nil)
		svc.EXPECT().ConfigureClientScopes(mock.Anything, "test-realm", mock.Anything).Return(nil)
		return svc
	}

	It("records the IdentityProvider and CA reference before requesting the service", func() {
		idp.Spec.TLS = &common.TLSConfig{ConfigMapRef: common.ConfigMapKeyRef{Name: "zone-ca-bundle", Key: "ca-bundle.pem"}}
		mockIdPGet()
		factory := &recordingFactory{svc: successfulService()}
		realm := newValidRealm()

		Expect(NewHandlerRealm(factory).CreateOrUpdate(ctx, realm)).To(Succeed())

		expected := &identityv1.RealmConnection{
			IdentityProvider: common.ObjectRef{Name: "test-idp", Namespace: "default", UID: "idp-uid"},
			TLS: &identityv1.RealmConnectionTLS{
				Namespace:    "default",
				ConfigMapRef: common.ConfigMapKeyRef{Name: "zone-ca-bundle", Key: "ca-bundle.pem"},
			},
		}
		Expect(realm.Status.Connection).To(Equal(expected))
		Expect(factory.calls).To(Equal(1))
		Expect(factory.realm).To(BeIdenticalTo(realm))
		Expect(factory.status.Connection).To(Equal(expected))
		Expect(factory.status.AdminPassword).To(Equal("resolved-password"))
		Expect(realm.Status.AdminPassword).To(Equal("$<secret-id>"))
		Expect(factory.releases).To(Equal(1))
	})

	It("clears a previous CA reference if the IdentityProvider has none", func() {
		mockIdPGet()
		factory := &recordingFactory{svc: successfulService()}
		realm := newValidRealm()
		realm.Status.Connection = &identityv1.RealmConnection{
			IdentityProvider: common.ObjectRef{Name: "test-idp", Namespace: "default", UID: "idp-uid"},
			TLS: &identityv1.RealmConnectionTLS{
				Namespace:    "default",
				ConfigMapRef: common.ConfigMapKeyRef{Name: "old", Key: "ca.pem"},
			},
		}

		Expect(NewHandlerRealm(factory).CreateOrUpdate(ctx, realm)).To(Succeed())

		Expect(realm.Status.Connection).NotTo(BeNil())
		Expect(realm.Status.Connection.TLS).To(BeNil())
		Expect(factory.status.Connection.TLS).To(BeNil())
	})

	It("releases the service if a Keycloak operation fails", func() {
		mockIdPGet()
		svc := keycloakservice.NewMockKeycloakService(GinkgoT())
		svc.EXPECT().CreateOrReplaceRealm(mock.Anything, mock.Anything).Return(nil, fmt.Errorf("boom"))
		factory := &recordingFactory{svc: svc}

		Expect(NewHandlerRealm(factory).CreateOrUpdate(ctx, newValidRealm())).NotTo(Succeed())
		Expect(factory.releases).To(Equal(1))
	})

	It("deletes a legacy realm without reading the IdentityProvider", func() {
		svc := keycloakservice.NewMockKeycloakService(GinkgoT())
		svc.EXPECT().DeleteRealm(mock.Anything, "test-realm").Return(nil)
		factory := &recordingFactory{svc: svc}
		realm := newValidRealm()
		realm.UID = "realm-uid"
		realm.Status = identityv1.RealmStatus{
			AdminClientId: "admin-cli",
			AdminUserName: "admin",
			AdminPassword: "$<secret-id>",
			AdminUrl:      "https://keycloak.example.com/auth/admin/realms/",
			AdminTokenUrl: "https://keycloak.example.com/auth/realms/master/protocol/openid-connect/token",
		}

		Expect(NewHandlerRealm(factory).Delete(ctx, realm)).To(Succeed())

		Expect(factory.realm).To(BeIdenticalTo(realm))
		Expect(factory.status.Connection).To(BeNil())
		Expect(factory.releases).To(Equal(1))
	})

	It("deletes using the recorded connection without reading the IdentityProvider", func() {
		svc := keycloakservice.NewMockKeycloakService(GinkgoT())
		svc.EXPECT().DeleteRealm(mock.Anything, "test-realm").Return(fmt.Errorf("boom"))
		factory := &recordingFactory{svc: svc}
		realm := newValidRealm()
		conn := &identityv1.RealmConnection{
			IdentityProvider: common.ObjectRef{Name: "test-idp", Namespace: "default", UID: "idp-uid"},
			TLS: &identityv1.RealmConnectionTLS{
				Namespace:    "default",
				ConfigMapRef: common.ConfigMapKeyRef{Name: "zone-ca-bundle", Key: "ca-bundle.pem"},
			},
		}
		realm.Status.Connection = conn

		Expect(NewHandlerRealm(factory).Delete(ctx, realm)).NotTo(Succeed())

		Expect(factory.status.Connection).To(Equal(conn))
		Expect(factory.releases).To(Equal(1))
	})
})
