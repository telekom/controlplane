// SPDX-FileCopyrightText: 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package util_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/telekom/controlplane/spectre/internal/handler/util"
)

var _ = Describe("Naming helpers", func() {
	Describe("MakePublisherName", func() {
		It("should normalize event type preserving dots", func() {
			result := util.MakePublisherName("de.telekom.ei.listener.my-app")
			Expect(result).To(Equal("de.telekom.ei.listener.my-app"))
		})

		It("should handle the generic event type", func() {
			result := util.MakePublisherName("de.telekom.ei.listener")
			Expect(result).To(Equal("de.telekom.ei.listener"))
		})

		It("should lowercase uppercase characters", func() {
			result := util.MakePublisherName("DE.Telekom.EI.Listener")
			Expect(result).To(Equal("de.telekom.ei.listener"))
		})

		It("should replace slashes with dashes", func() {
			result := util.MakePublisherName("some/event/type")
			Expect(result).To(Equal("some-event-type"))
		})
	})

	Describe("MakeSubscriberName", func() {
		It("should preserve double-dash separators", func() {
			result := util.MakeSubscriberName("team-a--my-app--api-v1--callback")
			Expect(result).To(Equal("team-a--my-app--api-v1--callback"))
		})

		It("should normalize slashes to dashes", func() {
			result := util.MakeSubscriberName("consumer/app/v1")
			Expect(result).To(Equal("consumer-app-v1"))
		})

		It("should preserve dots in subscriber ID", func() {
			result := util.MakeSubscriberName("consumer.app.v1")
			Expect(result).To(Equal("consumer.app.v1"))
		})
	})

	Describe("MakeRouteListenerName", func() {
		It("should build correct format with simple names", func() {
			result := util.MakeRouteListenerName("listener-app", "api-v1", "consumer-team", "provider-team")
			Expect(result).To(Equal("listener-app--api-v1--consumer-team--provider-team"))
		})

		It("should normalize slashes in components", func() {
			result := util.MakeRouteListenerName("my-app", "some/api", "cons", "prov")
			Expect(result).To(Equal("my-app--some-api--cons--prov"))
		})

		It("should lowercase all components", func() {
			result := util.MakeRouteListenerName("MyApp", "API", "Cons", "Prov")
			Expect(result).To(Equal("myapp--api--cons--prov"))
		})
	})

	Describe("MakeBridgeSubscriberId", func() {
		It("should build correct format", func() {
			result := util.MakeBridgeSubscriberId("consumer-team", "listener-app", "api-v1", "callback")
			Expect(result).To(Equal("consumer-team--listener-app--api-v1--callback"))
		})

		It("should preserve double-dashes as separators", func() {
			result := util.MakeBridgeSubscriberId("cons", "app", "api", "sse")
			Expect(result).To(Equal("cons--app--api--sse"))
		})
	})

	Describe("BuildListenerEventType", func() {
		It("should build per-app event type", func() {
			result := util.BuildListenerEventType("my-application-id")
			Expect(result).To(Equal("de.telekom.ei.listener.my-application-id"))
		})

		It("should append application ID directly", func() {
			result := util.BuildListenerEventType("abc123")
			Expect(result).To(Equal("de.telekom.ei.listener.abc123"))
		})
	})

	Describe("BuildBridgeCallbackURL", func() {
		It("should wrap callback URL with autoevent endpoint", func() {
			result := util.BuildBridgeCallbackURL("https://gateway.example.com/horizon/callback/v1", "my-app")
			Expect(result).To(Equal("https://gateway.example.com/horizon/callback/v1?callback=http://localhost:8080/autoevent?listener=my-app"))
		})

		It("should handle different app IDs", func() {
			result := util.BuildBridgeCallbackURL("https://gw.local/cb", "app-xyz-123")
			Expect(result).To(Equal("https://gw.local/cb?callback=http://localhost:8080/autoevent?listener=app-xyz-123"))
		})
	})

	Describe("Constants", func() {
		It("should have correct PublisherID", func() {
			Expect(util.PublisherID).To(Equal("gateway"))
		})

		It("should have correct GenericEventType", func() {
			Expect(util.GenericEventType).To(Equal("de.telekom.ei.listener"))
		})
	})
})
