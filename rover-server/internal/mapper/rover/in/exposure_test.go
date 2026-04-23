// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package in

import (
	"github.com/gkampitakis/go-snaps/snaps"

	"github.com/telekom/controlplane/rover-server/internal/api"
	roverv1 "github.com/telekom/controlplane/rover/api/v1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Exposure Mapper", func() {
	Context("mapApiExposure", func() {
		It("must map ApiExposure correctly", func() {
			input := apiExposure

			output := mapApiExposure(input)

			Expect(output).ToNot(BeNil())
			snaps.MatchSnapshot(GinkgoT(), output)
		})

		It("must handle empty ApiExposure input", func() {
			input := api.ApiExposure{}

			output := mapApiExposure(input)

			Expect(output).ToNot(BeNil())
			snaps.MatchSnapshot(GinkgoT(), output)
		})

		It("must map load balancing with multiple servers", func() {
			input := api.ApiExposure{
				BasePath:   "/lb/v1",
				Visibility: "World",
				Approval:   "Auto",
				LoadBalancing: api.LoadBalancing{
					Servers: []api.Server{
						{Upstream: "https://server1.example.com", Weight: 70},
						{Upstream: "https://server2.example.com", Weight: 30},
					},
				},
			}

			output := mapApiExposure(input)

			Expect(output.Upstreams).To(HaveLen(2))
			Expect(output.Upstreams[0].URL).To(Equal("https://server1.example.com"))
			Expect(output.Upstreams[0].Weight).To(Equal(70))
			Expect(output.Upstreams[1].URL).To(Equal("https://server2.example.com"))
			Expect(output.Upstreams[1].Weight).To(Equal(30))
		})

		It("must map load balancing with zero weight (omits weight)", func() {
			input := api.ApiExposure{
				BasePath:   "/lb/v1",
				Visibility: "World",
				Approval:   "Auto",
				LoadBalancing: api.LoadBalancing{
					Servers: []api.Server{
						{Upstream: "https://server1.example.com", Weight: 0},
					},
				},
			}

			output := mapApiExposure(input)

			Expect(output.Upstreams).To(HaveLen(1))
			Expect(output.Upstreams[0].URL).To(Equal("https://server1.example.com"))
			Expect(output.Upstreams[0].Weight).To(Equal(0))
		})
	})

	Context("mapExposure", func() {
		It("must map an ApiExposure correctly", func() {
			input := GetApiExposure(apiExposure)
			output := &roverv1.Exposure{}

			err := mapExposure(&input, output)

			Expect(err).ToNot(HaveOccurred())
			snaps.MatchSnapshot(GinkgoT(), output)
		})

		It("must map an EventExposure correctly", func() {
			input := GetEventExposure(eventExposure)
			output := &roverv1.Exposure{}

			err := mapExposure(&input, output)

			Expect(err).ToNot(HaveOccurred())
			snaps.MatchSnapshot(GinkgoT(), output)
		})

		It("must return an error for unknown exposure type", func() {
			input := &api.Exposure{}
			output := &roverv1.Exposure{}

			err := mapExposure(input, output)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to get exposure type"))
			snaps.MatchSnapshot(GinkgoT(), output)
		})
	})

	Context("mapEventExposure", func() {
		It("must map trusted teams correctly", func() {
			input := api.EventExposure{
				EventType:  "de.telekom.test.v1",
				Visibility: "Enterprise",
				Approval:   "Simple",
				TrustedTeams: []api.TrustedTeam{
					{Team: "groupA--teamX"},
					{Team: "groupB--teamY"},
				},
			}

			output := mapEventExposure(input)

			Expect(output.Approval.TrustedTeams).To(HaveLen(2))
			Expect(output.Approval.TrustedTeams[0].Group).To(Equal("groupA"))
			Expect(output.Approval.TrustedTeams[0].Team).To(Equal("teamX"))
			Expect(output.Approval.TrustedTeams[1].Group).To(Equal("groupB"))
			Expect(output.Approval.TrustedTeams[1].Team).To(Equal("teamY"))
		})

		It("must skip trusted teams with invalid format", func() {
			input := api.EventExposure{
				EventType:  "de.telekom.test.v1",
				Visibility: "World",
				Approval:   "Auto",
				TrustedTeams: []api.TrustedTeam{
					{Team: "groupA--teamX"},
					{Team: "invalidformat"},
					{Team: "groupB--teamY"},
				},
			}

			output := mapEventExposure(input)

			Expect(output.Approval.TrustedTeams).To(HaveLen(3))
			// Valid entries are mapped correctly
			Expect(output.Approval.TrustedTeams[0].Group).To(Equal("groupA"))
			Expect(output.Approval.TrustedTeams[0].Team).To(Equal("teamX"))
			// Invalid entry is skipped (zero-value TrustedTeam due to continue)
			Expect(output.Approval.TrustedTeams[1].Group).To(BeEmpty())
			Expect(output.Approval.TrustedTeams[1].Team).To(BeEmpty())
			// Next valid entry
			Expect(output.Approval.TrustedTeams[2].Group).To(Equal("groupB"))
			Expect(output.Approval.TrustedTeams[2].Team).To(Equal("teamY"))
		})

		It("must map scopes with triggers correctly", func() {
			input := api.EventExposure{
				EventType:  "de.telekom.test.v1",
				Visibility: "World",
				Approval:   "Auto",
				Scopes: []api.EventScope{
					{
						Name: "scope1",
						Trigger: api.EventTrigger{
							ResponseFilter:     []string{"$.data.field1"},
							ResponseFilterMode: "Include",
						},
					},
					{
						Name: "scope2",
						Trigger: api.EventTrigger{
							SelectionFilter: map[string]string{"type": "create"},
						},
					},
				},
			}

			output := mapEventExposure(input)

			Expect(output.Scopes).To(HaveLen(2))
			Expect(output.Scopes[0].Name).To(Equal("scope1"))
			Expect(output.Scopes[0].Trigger.ResponseFilter).ToNot(BeNil())
			Expect(output.Scopes[0].Trigger.ResponseFilter.Paths).To(ConsistOf("$.data.field1"))
			Expect(output.Scopes[0].Trigger.ResponseFilter.Mode).To(Equal(roverv1.EventResponseFilterMode("Include")))
			Expect(output.Scopes[1].Name).To(Equal("scope2"))
			Expect(output.Scopes[1].Trigger.SelectionFilter).ToNot(BeNil())
			Expect(output.Scopes[1].Trigger.SelectionFilter.Attributes).To(HaveKeyWithValue("type", "create"))
		})

		It("must map additional publisher IDs", func() {
			input := api.EventExposure{
				EventType:              "de.telekom.test.v1",
				Visibility:             "World",
				Approval:               "Auto",
				AdditionalPublisherIds: []string{"pub-1", "pub-2"},
			}

			output := mapEventExposure(input)

			Expect(output.AdditionalPublisherIds).To(ConsistOf("pub-1", "pub-2"))
		})
	})

	Context("mapEventTrigger", func() {
		It("must map response filter", func() {
			input := api.EventTrigger{
				ResponseFilter:     []string{"$.data.id", "$.data.name"},
				ResponseFilterMode: "Exclude",
			}

			output := mapEventTrigger(input)

			Expect(output).ToNot(BeNil())
			Expect(output.ResponseFilter).ToNot(BeNil())
			Expect(output.ResponseFilter.Paths).To(ConsistOf("$.data.id", "$.data.name"))
			Expect(output.ResponseFilter.Mode).To(Equal(roverv1.EventResponseFilterMode("Exclude")))
		})

		It("must map selection filter with attributes", func() {
			input := api.EventTrigger{
				SelectionFilter: map[string]string{"source": "my-app", "type": "create"},
			}

			output := mapEventTrigger(input)

			Expect(output).ToNot(BeNil())
			Expect(output.SelectionFilter).ToNot(BeNil())
			Expect(output.SelectionFilter.Attributes).To(HaveKeyWithValue("source", "my-app"))
			Expect(output.SelectionFilter.Attributes).To(HaveKeyWithValue("type", "create"))
		})

		It("must map advanced selection filter", func() {
			input := api.EventTrigger{
				AdvancedSelectionFilter: map[string]any{"op": "eq", "field": "type", "value": "create"},
			}

			output := mapEventTrigger(input)

			Expect(output).ToNot(BeNil())
			Expect(output.SelectionFilter).ToNot(BeNil())
			Expect(output.SelectionFilter.Expression).ToNot(BeNil())
			Expect(output.SelectionFilter.Expression.Raw).ToNot(BeNil())
		})

		It("must map both selection filter and advanced selection filter", func() {
			input := api.EventTrigger{
				SelectionFilter:         map[string]string{"source": "my-app"},
				AdvancedSelectionFilter: map[string]any{"op": "and", "children": []any{}},
			}

			output := mapEventTrigger(input)

			Expect(output).ToNot(BeNil())
			Expect(output.SelectionFilter).ToNot(BeNil())
			Expect(output.SelectionFilter.Attributes).To(HaveKeyWithValue("source", "my-app"))
			Expect(output.SelectionFilter.Expression).ToNot(BeNil())
		})
	})

	Context("mapTrustedTeams (API exposure)", func() {
		It("must skip entries with invalid format", func() {
			input := api.ApiExposure{
				TrustedTeams: []api.TrustedTeam{
					{Team: "group1--team1"},
					{Team: "nohyphens"},
					{Team: "too--many--parts"},
				},
			}
			output := &roverv1.ApiExposure{
				Approval: roverv1.Approval{},
			}

			mapTrustedTeams(input, output)

			Expect(output.Approval.TrustedTeams).To(HaveLen(3))
			// First entry: valid
			Expect(output.Approval.TrustedTeams[0].Group).To(Equal("group1"))
			Expect(output.Approval.TrustedTeams[0].Team).To(Equal("team1"))
			// Second entry: invalid format, skipped (zero-value)
			Expect(output.Approval.TrustedTeams[1].Group).To(BeEmpty())
			// Third entry: has 3 parts (too--many--parts splits to 3), so len != 2, skipped
			Expect(output.Approval.TrustedTeams[2].Group).To(BeEmpty())
		})
	})

	Context("toRoverVisibility", func() {
		It("must map WORLD visibility correctly", func() {
			input := api.WORLD

			output := toRoverVisibility(input)

			Expect(output).To(Equal(roverv1.VisibilityWorld))
		})

		It("must map ZONE visibility correctly", func() {
			input := api.ZONE

			output := toRoverVisibility(input)

			Expect(output).To(Equal(roverv1.VisibilityZone))
		})

		It("must map ENTERPRISE visibility correctly", func() {
			input := api.ENTERPRISE

			output := toRoverVisibility(input)

			Expect(output).To(Equal(roverv1.VisibilityEnterprise))
		})

		It("must map unknown visibility", func() {
			input := api.Visibility("unknown")

			output := toRoverVisibility(input)

			Expect(output).To(Equal(roverv1.Visibility("Unknown")))
		})
	})

	Context("toRoverApprovalStrategy", func() {
		It("must map AUTO approval strategy correctly", func() {
			input := api.AUTO

			output := toRoverApprovalStrategy(input)

			Expect(output).To(Equal(roverv1.ApprovalStrategyAuto))
		})

		It("must map MANUAL approval strategy correctly", func() {
			input := api.SIMPLE

			output := toRoverApprovalStrategy(input)

			Expect(output).To(Equal(roverv1.ApprovalStrategySimple))
		})

		It("must map FOUREYES approval strategy correctly", func() {
			input := api.FOUREYES

			output := toRoverApprovalStrategy(input)

			Expect(output).To(Equal(roverv1.ApprovalStrategyFourEyes))
		})

		It("must map unknown approval strategy", func() {
			input := api.ApprovalStrategy("unknown")

			output := toRoverApprovalStrategy(input)

			Expect(output).To(Equal(roverv1.ApprovalStrategy("Unknown")))
		})
	})
})
