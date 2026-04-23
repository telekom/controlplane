// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package out

import (
	"github.com/gkampitakis/go-snaps/snaps"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"

	"github.com/telekom/controlplane/rover-server/internal/api"
	roverv1 "github.com/telekom/controlplane/rover/api/v1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Exposure Mapper", func() {
	Context("mapApiExposure", func() {
		It("must map ApiExposure correctly", func() {
			input := &apiExposure

			output := mapApiExposure(input)

			Expect(output).ToNot(BeNil())
			snaps.MatchSnapshot(GinkgoT(), output)
		})

		It("must handle empty ApiExposure input", func() {
			input := &roverv1.ApiExposure{}

			output := mapApiExposure(input)

			Expect(output).ToNot(BeNil())
			snaps.MatchSnapshot(GinkgoT(), output)
		})
	})

	Context("mapExposure", func() {
		It("must map an ApiExposure correctly", func() {
			input := GetApiExposure(&apiExposure)
			output := &api.Exposure{}

			err := mapExposure(&input, output)

			Expect(err).ToNot(HaveOccurred())
			snaps.MatchSnapshot(GinkgoT(), output)
		})

		It("must map an EventExposure correctly", func() {
			input := GetEventExposure(&eventExposure)
			output := &api.Exposure{}

			err := mapExposure(&input, output)

			Expect(err).ToNot(HaveOccurred())
			snaps.MatchSnapshot(GinkgoT(), output)
		})

		It("must return an error for unknown exposure type", func() {
			input := &roverv1.Exposure{}
			output := &api.Exposure{}

			err := mapExposure(input, output)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("unknown exposure type"))
			snaps.MatchSnapshot(GinkgoT(), output)
		})
	})

	Context("mapApiExposure with load balancing", func() {
		It("must map multiple upstreams as load balancing servers", func() {
			input := &roverv1.ApiExposure{
				BasePath:   "/lb/v1",
				Visibility: roverv1.VisibilityWorld,
				Approval:   roverv1.Approval{Strategy: roverv1.ApprovalStrategyAuto},
				Upstreams: []roverv1.Upstream{
					{URL: "https://server1.example.com", Weight: 70},
					{URL: "https://server2.example.com", Weight: 30},
				},
			}

			output := mapApiExposure(input)

			Expect(output.Upstream).To(BeEmpty())
			Expect(output.LoadBalancing.Servers).To(HaveLen(2))
			Expect(output.LoadBalancing.Servers[0].Upstream).To(Equal("https://server1.example.com"))
			Expect(output.LoadBalancing.Servers[0].Weight).To(Equal(70))
			Expect(output.LoadBalancing.Servers[1].Upstream).To(Equal("https://server2.example.com"))
			Expect(output.LoadBalancing.Servers[1].Weight).To(Equal(30))
		})
	})

	Context("mapEventExposure", func() {
		It("must map trusted teams correctly", func() {
			input := &roverv1.EventExposure{
				EventType:  "de.telekom.test.v1",
				Visibility: roverv1.VisibilityEnterprise,
				Approval: roverv1.Approval{
					Strategy: roverv1.ApprovalStrategySimple,
					TrustedTeams: []roverv1.TrustedTeam{
						{Group: "groupA", Team: "teamX"},
						{Group: "groupB", Team: "teamY"},
					},
				},
			}

			output := mapEventExposure(input)

			Expect(output.TrustedTeams).To(HaveLen(2))
			Expect(output.TrustedTeams[0].Team).To(Equal("groupA--teamX"))
			Expect(output.TrustedTeams[1].Team).To(Equal("groupB--teamY"))
		})

		It("must map scopes with triggers correctly", func() {
			input := &roverv1.EventExposure{
				EventType:  "de.telekom.test.v1",
				Visibility: roverv1.VisibilityWorld,
				Approval:   roverv1.Approval{Strategy: roverv1.ApprovalStrategyAuto},
				Scopes: []roverv1.EventScope{
					{
						Name: "scope1",
						Trigger: roverv1.EventTrigger{
							ResponseFilter: &roverv1.EventResponseFilter{
								Paths: []string{"$.data.field1"},
								Mode:  roverv1.EventResponseFilterModeInclude,
							},
						},
					},
					{
						Name: "scope2",
						Trigger: roverv1.EventTrigger{
							SelectionFilter: &roverv1.EventSelectionFilter{
								Attributes: map[string]string{"type": "create"},
							},
						},
					},
				},
			}

			output := mapEventExposure(input)

			Expect(output.Scopes).To(HaveLen(2))
			Expect(output.Scopes[0].Name).To(Equal("scope1"))
			Expect(output.Scopes[0].Trigger.ResponseFilter).To(ConsistOf("$.data.field1"))
			Expect(output.Scopes[1].Name).To(Equal("scope2"))
			Expect(output.Scopes[1].Trigger.SelectionFilter).To(HaveKeyWithValue("type", "create"))
		})

		It("must map additional publisher IDs", func() {
			input := &roverv1.EventExposure{
				EventType:              "de.telekom.test.v1",
				Visibility:             roverv1.VisibilityWorld,
				Approval:               roverv1.Approval{Strategy: roverv1.ApprovalStrategyAuto},
				AdditionalPublisherIds: []string{"pub-1", "pub-2"},
			}

			output := mapEventExposure(input)

			Expect(output.AdditionalPublisherIds).To(ConsistOf("pub-1", "pub-2"))
		})
	})

	Context("mapEventTriggerOut", func() {
		It("must map response filter", func() {
			input := &roverv1.EventTrigger{
				ResponseFilter: &roverv1.EventResponseFilter{
					Paths: []string{"$.data.id", "$.data.name"},
					Mode:  roverv1.EventResponseFilterModeExclude,
				},
			}

			output := mapEventTriggerOut(input)

			Expect(output.ResponseFilter).To(ConsistOf("$.data.id", "$.data.name"))
			Expect(output.ResponseFilterMode).To(Equal(api.EventTriggerResponseFilterMode("Exclude")))
		})

		It("must map selection filter with attributes and expression", func() {
			raw := []byte(`{"op":"eq","field":"type","value":"create"}`)
			input := &roverv1.EventTrigger{
				SelectionFilter: &roverv1.EventSelectionFilter{
					Attributes: map[string]string{"source": "my-app"},
					Expression: &apiextensionsv1.JSON{Raw: raw},
				},
			}

			output := mapEventTriggerOut(input)

			Expect(output.SelectionFilter).To(HaveKeyWithValue("source", "my-app"))
			Expect(output.AdvancedSelectionFilter).ToNot(BeNil())
			Expect(output.AdvancedSelectionFilter["op"]).To(Equal("eq"))
		})
	})

	Context("toApiVisibility", func() {
		It("must map WORLD visibility correctly", func() {
			input := roverv1.VisibilityWorld

			output := toApiVisibility(input)

			Expect(output).To(Equal(api.WORLD))
		})

		It("must map ZONE visibility correctly", func() {
			input := roverv1.VisibilityZone

			output := toApiVisibility(input)

			Expect(output).To(Equal(api.ZONE))
		})

		It("must map ENTERPRISE visibility correctly", func() {
			input := roverv1.VisibilityEnterprise

			output := toApiVisibility(input)

			Expect(output).To(Equal(api.ENTERPRISE))
		})

		It("must map unknown visibility", func() {
			input := roverv1.Visibility("Unknown")

			output := toApiVisibility(input)

			Expect(output).To(Equal(api.Visibility("UNKNOWN")))
		})

		Context("toApiApprovalStrategy", func() {
			It("must map AUTO approval strategy correctly", func() {
				input := roverv1.ApprovalStrategyAuto

				output := toApiApprovalStrategy(input)

				Expect(output).To(Equal(api.AUTO))
			})

			It("must map MANUAL approval strategy correctly", func() {
				input := roverv1.ApprovalStrategySimple

				output := toApiApprovalStrategy(input)

				Expect(output).To(Equal(api.SIMPLE))
			})

			It("must map FOUREYES approval strategy correctly", func() {
				input := roverv1.ApprovalStrategyFourEyes

				output := toApiApprovalStrategy(input)

				Expect(output).To(Equal(api.FOUREYES))
			})

			It("must map unknown approval strategy", func() {
				input := roverv1.ApprovalStrategy("Unknown")

				output := toApiApprovalStrategy(input)

				Expect(output).To(Equal(api.ApprovalStrategy("UNKNOWN")))
			})
		})
	})
})
