// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package application

import (
	"context"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/config"
	"github.com/telekom/controlplane/common/pkg/util/contextutil"

	roverv1 "github.com/telekom/controlplane/rover/api/v1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
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
	newListenerRover := func(sub *roverv1.ListenerSubscription, exps []roverv1.Exposure) *roverv1.Rover {
		rover := newRover(exps, nil)
		rover.Spec.Listeners = []roverv1.RoverListener{{Consumer: "consumer-app", Provider: "provider-app", ApiBasePath: "/echo/v1"}}
		rover.Spec.ListenerSubscription = sub
		return rover
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

	DescribeTable("requires an Identity client for any Rover with listeners",
		func(rover *roverv1.Rover) {
			Expect(isClientNeeded(rover)).To(BeTrue())
		},
		Entry("listeners with default delivery", newListenerRover(nil, nil)),
		Entry("listeners with server_sent_event delivery", newListenerRover(&roverv1.ListenerSubscription{DeliveryType: "server_sent_event"}, nil)),
		Entry("listeners with callback delivery", newListenerRover(&roverv1.ListenerSubscription{DeliveryType: "callback", Callback: "https://example.com/listener"}, nil)),
		Entry("listeners + file exposure", newListenerRover(nil, []roverv1.Exposure{fileExp})),
	)
})

var _ = Describe("ListedPublisherIds", func() {
	It("collects sorted, deduplicated IDs from event exposures only", func() {
		r := &roverv1.Rover{Spec: roverv1.RoverSpec{Exposures: []roverv1.Exposure{
			{Event: &roverv1.EventExposure{AdditionalPublisherIds: []string{"b--team--x", "a--team--y"}}},
			{Event: &roverv1.EventExposure{AdditionalPublisherIds: []string{"b--team--x"}}},
			{File: &roverv1.FileExposure{}},
		}}}
		Expect(ListedPublisherIds(r)).To(Equal([]string{"a--team--y", "b--team--x"}))
		Expect(IndexAdditionalPublisherIds(r)).To(Equal([]string{"a--team--y", "b--team--x"}))
	})

	It("builds the publisher ID as a whole string", func() {
		Expect(PublisherId("hub--team", "app")).To(Equal("hub--team--app"))
	})
})

type listClient struct {
	client.JanitorClient
	inner ctrlclient.Client
}

func (l *listClient) List(ctx context.Context, list ctrlclient.ObjectList, opts ...ctrlclient.ListOption) error {
	return l.inner.List(ctx, list, opts...)
}

var _ = Describe("isListedAsAdditionalPublisher", func() {
	const env = "dev"
	var c *listClient

	mkRover := func(name, ns, environment string, ids ...string) *roverv1.Rover {
		r := &roverv1.Rover{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns, Labels: map[string]string{config.EnvironmentLabelKey: environment}},
		}
		if len(ids) > 0 {
			r.Spec.Exposures = []roverv1.Exposure{{Event: &roverv1.EventExposure{AdditionalPublisherIds: ids}}}
		}
		return r
	}

	build := func(objs ...ctrlclient.Object) {
		s := runtime.NewScheme()
		Expect(roverv1.AddToScheme(s)).To(Succeed())
		c = &listClient{inner: fake.NewClientBuilder().WithScheme(s).WithObjects(objs...).
			WithIndex(&roverv1.Rover{}, AdditionalPublisherIndexKey, IndexAdditionalPublisherIds).Build()}
	}

	BeforeEach(func() {
		config.SetFeatureEnabled(config.FeaturePubSub, true)
	})
	AfterEach(func() {
		config.SetFeatureEnabled(config.FeaturePubSub, false)
	})

	It("is true when another Rover in the same environment lists the whole ID", func() {
		build(mkRover("owner", "ns-o", env, "hub--team--pub"))
		ctx := contextutil.WithEnv(context.Background(), env)
		got, err := isListedAsAdditionalPublisher(ctx, c, mkRover("pub", "ns-p", env), "hub--team")
		Expect(err).NotTo(HaveOccurred())
		Expect(got).To(BeTrue())
	})

	It("ignores other environments, partial IDs and unresolved IDs", func() {
		build(
			mkRover("o1", "ns-o", "prod", "hub--team--pub"),
			mkRover("o2", "ns-o", env, "team--pub", "hub--team--pub2"),
		)
		ctx := contextutil.WithEnv(context.Background(), env)
		got, err := isListedAsAdditionalPublisher(ctx, c, mkRover("pub", "ns-p", env), "hub--team")
		Expect(err).NotTo(HaveOccurred())
		Expect(got).To(BeFalse())
	})

	It("does not count a self-listing", func() {
		self := mkRover("pub", "ns-p", env, "hub--team--pub")
		build(self)
		ctx := contextutil.WithEnv(context.Background(), env)
		got, err := isListedAsAdditionalPublisher(ctx, c, self, "hub--team")
		Expect(err).NotTo(HaveOccurred())
		Expect(got).To(BeFalse())
	})

	It("is false when PubSub is disabled", func() {
		config.SetFeatureEnabled(config.FeaturePubSub, false)
		build(mkRover("owner", "ns-o", env, "hub--team--pub"))
		ctx := contextutil.WithEnv(context.Background(), env)
		got, err := isListedAsAdditionalPublisher(ctx, c, mkRover("pub", "ns-p", env), "hub--team")
		Expect(err).NotTo(HaveOccurred())
		Expect(got).To(BeFalse())
	})

	Context("revert", func() {
		It("is false after the ID is removed from the owner's list", func() {
			build(mkRover("owner", "ns-o", env))
			ctx := contextutil.WithEnv(context.Background(), env)
			got, err := isListedAsAdditionalPublisher(ctx, c, mkRover("pub", "ns-p", env), "hub--team")
			Expect(err).NotTo(HaveOccurred())
			Expect(got).To(BeFalse())
		})

		It("is false after the owner Rover is deleted", func() {
			build()
			ctx := contextutil.WithEnv(context.Background(), env)
			got, err := isListedAsAdditionalPublisher(ctx, c, mkRover("pub", "ns-p", env), "hub--team")
			Expect(err).NotTo(HaveOccurred())
			Expect(got).To(BeFalse())
		})

		It("keeps a Rover with its own need true when the ID is removed", func() {
			build(mkRover("owner", "ns-o", env))
			ctx := contextutil.WithEnv(context.Background(), env)
			pub := mkRover("pub", "ns-p", env)
			pub.Spec.Listeners = []roverv1.RoverListener{{}}
			listed, err := isListedAsAdditionalPublisher(ctx, c, pub, "hub--team")
			Expect(err).NotTo(HaveOccurred())
			Expect(isClientNeeded(pub) || listed).To(BeTrue())
		})
	})
})
