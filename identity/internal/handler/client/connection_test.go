// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package client

import (
	"context"
	"fmt"

	"github.com/stretchr/testify/mock"

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
	realm    *identityv1.Realm
	status   identityv1.RealmStatus
	releases int
}

//nolint:gocritic // ServiceFactory keeps a value-based status for callers.
func (f *recordingFactory) ServiceFor(_ context.Context, realm *identityv1.Realm, status identityv1.RealmStatus) (keycloak.KeycloakService, func(), error) {
	f.realm = realm
	f.status = status
	return f.svc, func() { f.releases++ }, nil
}

var _ = Describe("HandlerClient connection", func() {
	var (
		mockK8s *fake.MockJanitorClient
		ctx     context.Context
		realm   *identityv1.Realm
	)

	BeforeEach(func() {
		mockK8s = fake.NewMockJanitorClient(GinkgoT())
		ctx = client.WithClient(context.Background(), mockK8s)
		realm = newValidRealm()
		realm.UID = "realm-uid"
		realm.Status.Connection = &identityv1.RealmConnection{
			IdentityProvider: common.ObjectRef{Name: "idp", Namespace: "idp-ns", UID: "idp-uid"},
			TLS: &identityv1.RealmConnectionTLS{
				Namespace:    "idp-ns",
				ConfigMapRef: common.ConfigMapKeyRef{Name: "zone-ca-bundle", Key: "ca-bundle.pem"},
			},
		}
		overrideSecretsGet(func(_ context.Context, _ string) (string, error) {
			return "resolved", nil
		})
	})

	It("requests the service with the Realm and its connection and releases it", func() {
		mockRealmGet(mockK8s, realm)
		svc := keycloakservice.NewMockKeycloakService(GinkgoT())
		svc.EXPECT().CreateOrReplaceClient(mock.Anything, realm.Name, mock.Anything, mock.Anything).Return(fmt.Errorf("boom"))
		factory := &recordingFactory{svc: svc}

		Expect(NewHandlerClient(factory).CreateOrUpdate(ctx, newValidClient())).NotTo(Succeed())

		Expect(factory.realm.UID).To(BeEquivalentTo("realm-uid"))
		Expect(factory.status.Connection).To(Equal(realm.Status.Connection))
		Expect(factory.releases).To(Equal(1))
	})

	It("releases the service after deletion", func() {
		mockRealmGet(mockK8s, realm)
		svc := keycloakservice.NewMockKeycloakService(GinkgoT())
		svc.EXPECT().DeleteClient(mock.Anything, realm.Name, mock.Anything).Return(nil)
		factory := &recordingFactory{svc: svc}

		Expect(NewHandlerClient(factory).Delete(ctx, newValidClient())).To(Succeed())

		Expect(factory.status.Connection).To(Equal(realm.Status.Connection))
		Expect(factory.releases).To(Equal(1))
	})
})
