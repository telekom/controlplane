// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package util

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apiapi "github.com/telekom/controlplane/api/api/v1"
)

var _ = Describe("Subscription Util", func() {
	Context("HasM2MRemote", func() {
		It("should return true if SubscriberMachine2MachineAuthentication exists", func() {

			subscription := &apiapi.RemoteApiSubscription{}
			Expect(HasM2MRemote(subscription)).To(BeFalse())

			subscription.Spec = apiapi.RemoteApiSubscriptionSpec{}
			Expect(HasM2MRemote(subscription)).To(BeFalse())

			subscription.Spec.Security = &apiapi.SubscriberSecurity{}
			Expect(HasM2MRemote(subscription)).To(BeFalse())

			subscription.Spec.Security.M2M = &apiapi.SubscriberMachine2MachineAuthentication{}
			Expect(HasM2MRemote(subscription)).To(BeTrue())
		})
	})

	Context("HasM2MClientRemote", func() {
		It("should return true if SubscriberMachine2MachineAuthentication exists", func() {

			subscription := &apiapi.RemoteApiSubscription{}
			Expect(HasM2MRemote(subscription)).To(BeFalse())

			subscription.Spec = apiapi.RemoteApiSubscriptionSpec{}
			Expect(HasM2MRemote(subscription)).To(BeFalse())

			subscription.Spec.Security = &apiapi.SubscriberSecurity{}
			Expect(HasM2MClientRemote(subscription)).To(BeFalse())

			subscription.Spec.Security.M2M = &apiapi.SubscriberMachine2MachineAuthentication{}
			Expect(HasM2MClientRemote(subscription)).To(BeFalse())

			subscription.Spec.Security.M2M.Client = &apiapi.OAuth2ClientCredentials{}
			Expect(HasM2MClientRemote(subscription)).To(BeTrue())
		})
	})
})
