// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package client

import (
	"reflect"

	"github.com/emirpasic/gods/sets/hashset"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("normalizeSet", func() {
	It("sorts and removes duplicates without mutating the input", func() {
		input := []string{"beta", "alpha", "beta"}

		normalized := normalizeSet(&input)

		Expect(*normalized).To(Equal([]string{"alpha", "beta"}))
		Expect(input).To(Equal([]string{"beta", "alpha", "beta"}))
	})

	It("normalizes nil and empty slices to nil", func() {
		empty := []string{}

		Expect(normalizeSet[string](nil)).To(BeNil())
		Expect(normalizeSet(&empty)).To(BeNil())
	})
})

var _ = Describe("plugin config normalization", func() {
	It("normalizes equivalent JSON numbers identically", func() {
		integer, err := normalizeConfig(map[string]any{"limit": 1})
		Expect(err).NotTo(HaveOccurred())

		decimal, err := normalizeConfig(map[string]any{"limit": float64(1)})
		Expect(err).NotTo(HaveOccurred())

		Expect(reflect.DeepEqual(integer, decimal)).To(BeTrue())
	})

	It("sorts set-like string arrays so that ordering cannot look like a change", func() {
		desired, err := normalizeConfig(map[string]any{
			"allow": []string{"gamma", "alpha", "beta"},
		})
		Expect(err).NotTo(HaveOccurred())

		current := map[string]any{"allow": []any{"beta", "gamma", "alpha"}}
		sortConfigSets(current)

		Expect(reflect.DeepEqual(desired, current)).To(BeTrue())
	})

	It("sorts arrays nested inside the configuration", func() {
		desired, err := normalizeConfig(map[string]any{
			"append": map[string]any{"headers": []string{"b:2", "a:1"}},
		})
		Expect(err).NotTo(HaveOccurred())

		current := map[string]any{"append": map[string]any{"headers": []any{"a:1", "b:2"}}}
		sortConfigSets(current)

		Expect(reflect.DeepEqual(desired, current)).To(BeTrue())
	})

	It("leaves arrays that are not all strings untouched", func() {
		config := map[string]any{"statuses": []any{float64(3), float64(1), float64(2)}}

		sortConfigSets(config)

		Expect(config["statuses"]).To(Equal([]any{float64(3), float64(1), float64(2)}))
	})

	It("sorts values produced by set types, whose iteration order is randomized", func() {
		// An ACL allow list is built from a hash set, which marshals in Go map
		// iteration order. Without sorting, the desired config would differ
		// from the one read back from Kong on nearly every reconciliation.
		config := map[string]any{
			"allow": hashset.New("gamma", "alpha", "epsilon", "beta", "delta"),
		}

		normalized, err := normalizeConfig(config)

		Expect(err).NotTo(HaveOccurred())
		Expect(normalized["allow"]).To(Equal([]any{"alpha", "beta", "delta", "epsilon", "gamma"}))
	})

	It("normalizes a nil configuration to nil", func() {
		normalized, err := normalizeConfig(nil)

		Expect(err).NotTo(HaveOccurred())
		Expect(normalized).To(BeNil())
	})

	It("copies the configuration it normalizes", func() {
		config := map[string]any{"allow": []any{"beta", "alpha"}}

		_, err := normalizeConfig(config)

		Expect(err).NotTo(HaveOccurred())
		Expect(config["allow"]).To(Equal([]any{"beta", "alpha"}))
	})
})

var _ = Describe("narrowToDesired", func() {
	It("drops the keys Kong fills in from its defaults", func() {
		desired := map[string]any{"allow": []any{"group-a"}}
		current := map[string]any{"allow": []any{"group-a"}, "deny": nil, "hide_groups_header": false}

		Expect(narrowToDesired(desired, current)).To(Equal(desired))
	})

	It("drops defaults nested inside a managed group", func() {
		desired := map[string]any{"limit": map[string]any{"second": float64(5)}}
		current := map[string]any{"limit": map[string]any{"second": float64(5), "minute": nil}}

		Expect(narrowToDesired(desired, current)).To(Equal(desired))
	})

	It("keeps a differing value so that it still counts as a change", func() {
		desired := map[string]any{"allow": []any{"group-a"}}
		current := map[string]any{"allow": []any{"group-b"}, "deny": nil}

		Expect(narrowToDesired(desired, current)).To(Equal(map[string]any{"allow": []any{"group-b"}}))
	})

	It("leaves out a desired key Kong does not report", func() {
		narrowed := narrowToDesired(map[string]any{"allow": []any{"group-a"}}, map[string]any{})

		Expect(narrowed).To(BeEmpty())
		Expect(reflect.DeepEqual(map[string]any{"allow": []any{"group-a"}}, narrowed)).To(BeFalse())
	})

	It("narrows a nil desired configuration to nil", func() {
		Expect(narrowToDesired(nil, map[string]any{"deny": nil})).To(BeNil())
	})

	It("does not modify the configuration it narrows", func() {
		current := map[string]any{"allow": []any{"group-a"}, "deny": nil}

		narrowToDesired(map[string]any{"allow": []any{"group-a"}}, current)

		Expect(current).To(HaveKey("deny"))
	})
})

var _ = Describe("summarizeBody", func() {
	It("keeps the description Kong gives for a rejected request", func() {
		Expect(summarizeBody([]byte(`{"name":"schema violation","message":"2 schema violations"}`))).
			To(Equal("schema violation: 2 schema violations"))
	})

	It("drops the echoed request, which carries the credentials of an auth plugin", func() {
		body := []byte(`{"name":"schema violation","message":"config.secret: required",` +
			`"fields":{"config":{"secret":"s3cr3t-value","key":"client-key"}}}`)

		summary := summarizeBody(body)

		Expect(summary).NotTo(ContainSubstring("s3cr3t-value"))
		Expect(summary).NotTo(ContainSubstring("client-key"))
		Expect(summary).To(ContainSubstring("config.secret: required"))
	})

	It("falls back to whichever description Kong gives", func() {
		Expect(summarizeBody([]byte(`{"message":"no route matched"}`))).To(Equal("no route matched"))
		Expect(summarizeBody([]byte(`{"name":"unique violation"}`))).To(Equal("unique violation"))
		Expect(summarizeBody([]byte(`{}`))).To(Equal("<no description>"))
	})

	It("reports a body it cannot read without echoing it", func() {
		Expect(summarizeBody([]byte(`<html>s3cr3t</html>`))).To(Equal("<unreadable body>"))
		Expect(summarizeBody(nil)).To(Equal("<unreadable body>"))
	})
})
