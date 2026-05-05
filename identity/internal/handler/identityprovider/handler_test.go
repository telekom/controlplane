// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package identityprovider

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/telekom/controlplane/common/pkg/condition"
	identityv1 "github.com/telekom/controlplane/identity/api/v1"
	"github.com/telekom/controlplane/identity/pkg/keycloak"
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
})
