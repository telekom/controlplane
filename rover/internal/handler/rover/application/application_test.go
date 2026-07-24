// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package application

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	roverv1 "github.com/telekom/controlplane/rover/api/v1"
)

func TestApplication(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Application Handler Suite")
}

var _ = Describe("RoverNeedsClient", func() {
	newRover := func(exps []roverv1.Exposure, subs []roverv1.Subscription) *roverv1.Rover {
		return &roverv1.Rover{
			ObjectMeta: metav1.ObjectMeta{Name: "test-rover", Namespace: "test--eni--hyperion"},
			Spec:       roverv1.RoverSpec{Exposures: exps, Subscriptions: subs},
		}
	}

	// The concrete field values are irrelevant: Subscription.Type()/Exposure.Type()
	// dispatch only on which pointer is non-nil.
	apiSub := roverv1.Subscription{Api: &roverv1.ApiSubscription{}}
	eventSub := roverv1.Subscription{Event: &roverv1.EventSubscription{}}
	fileSub := roverv1.Subscription{File: &roverv1.FileSubscription{}}
	eventExp := roverv1.Exposure{Event: &roverv1.EventExposure{}}
	fileExp := roverv1.Exposure{File: &roverv1.FileExposure{}}

	DescribeTable("decides whether the derived Application requires an Identity client",
		func(exps []roverv1.Exposure, subs []roverv1.Subscription, expected bool) {
			Expect(isClientNeeded(newRover(exps, subs))).To(Equal(expected))
		},
		// Logical Application (file-only or empty) => no client/consumer.
		Entry("empty rover", nil, nil, false),
		Entry("file-only subscription", nil, []roverv1.Subscription{fileSub}, false),
		Entry("file-only exposure", []roverv1.Exposure{fileExp}, nil, false),
		Entry("file exposure + file subscription", []roverv1.Exposure{fileExp}, []roverv1.Subscription{fileSub}, false),
		// Non-file subscription or any event exposure => needs client.
		Entry("api subscription", nil, []roverv1.Subscription{apiSub}, true),
		Entry("event subscription", nil, []roverv1.Subscription{eventSub}, true),
		Entry("event exposure", []roverv1.Exposure{eventExp}, nil, true),
		// Mixed: file plus a non-file entry still forces a client (story edge case).
		Entry("mixed file + api subscription", nil, []roverv1.Subscription{fileSub, apiSub}, true),
		Entry("file subscription + event exposure", []roverv1.Exposure{eventExp}, []roverv1.Subscription{fileSub}, true),
	)
})
