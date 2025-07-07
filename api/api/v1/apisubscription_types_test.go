// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v1_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apiapi "github.com/telekom/controlplane/api/api/v1"
)

var _ = Describe("Subscription Util", func() {
	Context("HasM2M", func() {
		It("should return true if SubscriberMachine2MachineAuthentication exists", func() {

			subscription := &apiapi.ApiSubscription{}
			Expect(subscription.Spec.HasM2M()).To(BeFalse())

			subscription.Spec = apiapi.ApiSubscriptionSpec{}
			Expect(subscription.Spec.HasM2M()).To(BeFalse())

			subscription.Spec.Security = &apiapi.SubscriberSecurity{}
			Expect(subscription.Spec.HasM2M()).To(BeFalse())

			subscription.Spec.Security.M2M = &apiapi.SubscriberMachine2MachineAuthentication{}
			Expect(subscription.Spec.HasM2M()).To(BeTrue())
		})
	})

	Context("HasM2MClient", func() {
		It("should return true if SubscriberMachine2MachineAuthentication exists", func() {

			subscription := &apiapi.ApiSubscription{}
			Expect(subscription.Spec.HasM2MClient()).To(BeFalse())

			subscription.Spec = apiapi.ApiSubscriptionSpec{}
			Expect(subscription.Spec.HasM2MClient()).To(BeFalse())

			subscription.Spec.Security = &apiapi.SubscriberSecurity{}
			Expect(subscription.Spec.HasM2MClient()).To(BeFalse())

			subscription.Spec.Security.M2M = &apiapi.SubscriberMachine2MachineAuthentication{}
			Expect(subscription.Spec.HasM2MClient()).To(BeFalse())

			subscription.Spec.Security.M2M.Client = &apiapi.OAuth2ClientCredentials{}
			Expect(subscription.Spec.HasM2MClient()).To(BeTrue())
		})
	})
})
