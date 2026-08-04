// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package identityprovider

import (
	"context"
	"errors"

	"github.com/stretchr/testify/mock"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	cc "github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/client/fake"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/errors/ctrlerrors"
	identityv1 "github.com/telekom/controlplane/identity/api/v1"
	"github.com/telekom/controlplane/identity/pkg/keycloak"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("HandlerIdentityProvider", func() {
	Context("CreateOrUpdate", func() {
		It("should return an error when the IdentityProvider is nil", func() {
			handler := &HandlerIdentityProvider{}
			err := handler.CreateOrUpdate(context.Background(), nil)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("IdentityProvider is nil"))
		})

		It("should set status and conditions on success", func() {
			By("creating a valid IdentityProvider")
			idp := &identityv1.IdentityProvider{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-idp",
				},
				Spec: identityv1.IdentityProviderSpec{
					AdminUrl: "https://admin.example.com",
				},
			}

			By("calling CreateOrUpdate")
			handler := &HandlerIdentityProvider{}
			err := handler.CreateOrUpdate(context.Background(), idp)

			By("expecting no error")
			Expect(err).ToNot(HaveOccurred())

			By("verifying the status fields were populated")
			Expect(idp.Status.AdminUrl).To(Equal("https://admin.example.com"))
			Expect(idp.Status.AdminTokenUrl).To(Equal(
				keycloak.DetermineAdminTokenUrlFrom("https://admin.example.com", keycloak.MasterRealm)))
			Expect(idp.Status.AdminConsoleUrl).To(Equal(
				keycloak.DetermineAdminConsoleUrlFrom("https://admin.example.com", keycloak.MasterRealm)))

			By("verifying the conditions are set correctly")
			conditions := idp.GetConditions()
			Expect(conditions).ToNot(BeEmpty())

			var readyFound, doneProcessingFound bool
			for _, c := range conditions {
				if c.Type == condition.ConditionTypeReady && c.Status == metav1.ConditionTrue {
					readyFound = true
				}
				if c.Type == condition.ConditionTypeProcessing && c.Status == metav1.ConditionFalse && c.Reason == "Done" {
					doneProcessingFound = true
				}
			}
			Expect(readyFound).To(BeTrue(), "idp should have Ready=True condition")
			Expect(doneProcessingFound).To(BeTrue(), "idp should have Processing=False/Done condition")
		})
	})

	Context("Delete", func() {
		It("allows deletion when no realm references the IdentityProvider", func() {
			mockClient := fake.NewMockJanitorClient(GinkgoT())
			mockClient.On("List", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)
			idp := &identityv1.IdentityProvider{ObjectMeta: metav1.ObjectMeta{Name: "test-idp"}}

			Expect((&HandlerIdentityProvider{}).Delete(cc.WithClient(context.Background(), mockClient), idp)).To(Succeed())
		})

		It("blocks deletion while a realm references the IdentityProvider", func() {
			mockClient := fake.NewMockJanitorClient(GinkgoT())
			mockClient.On("List", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
				Run(func(args mock.Arguments) {
					args.Get(1).(*identityv1.RealmList).Items = []identityv1.Realm{{}}
				}).Return(nil)
			idp := &identityv1.IdentityProvider{ObjectMeta: metav1.ObjectMeta{Name: "test-idp", Namespace: "default"}}

			err := (&HandlerIdentityProvider{}).Delete(cc.WithClient(context.Background(), mockClient), idp)
			var blocked ctrlerrors.BlockedError
			Expect(errors.As(err, &blocked)).To(BeTrue())
			mockClient.AssertCalled(GinkgoT(), "List", mock.Anything, mock.AnythingOfType("*v1.RealmList"), mock.Anything, mock.Anything)
		})

		It("returns list errors", func() {
			mockClient := fake.NewMockJanitorClient(GinkgoT())
			mockClient.On("List", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
				Return(errors.New("list failed"))
			idp := &identityv1.IdentityProvider{ObjectMeta: metav1.ObjectMeta{Name: "test-idp"}}

			err := (&HandlerIdentityProvider{}).Delete(cc.WithClient(context.Background(), mockClient), idp)
			Expect(err).To(MatchError(ContainSubstring("listing realms")))
		})
	})
})
