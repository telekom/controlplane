// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package in

import (
	"strings"

	"github.com/gkampitakis/go-snaps/snaps"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/telekom/controlplane/common/pkg/config"
	"github.com/telekom/controlplane/rover-server/internal/api"
	"github.com/telekom/controlplane/rover-server/internal/mapper"
	roverv1 "github.com/telekom/controlplane/rover/api/v1"
	"k8s.io/apimachinery/pkg/util/validation"
)

var _ = Describe("Rover Mapper", func() {
	Context("MapRover", func() {
		It("must map the api format to the internal format", func() {
			input := apiRover
			output := &roverv1.Rover{}

			err := MapRover(input, output)

			Expect(err).To(BeNil())
			snaps.MatchSnapshot(GinkgoT(), output)
		})

		It("must return an error if the input rover is nil", func() {
			output := &roverv1.Rover{}

			err := MapRover(nil, output)

			Expect(err).ToNot(BeNil())
			Expect(err.Error()).To(ContainSubstring("input rover is nil"))
		})

	})

	Context("MapExposures", func() {
		It("must map exposures correctly", func() {
			input := apiRover
			output := &roverv1.Rover{}

			err := mapExposures(input, output)

			Expect(err).To(BeNil())
			snaps.MatchSnapshot(GinkgoT(), output)
		})
	})

	Context("MapSubscriptions", func() {
		It("must map subscriptions correctly", func() {
			input := apiRover
			out := &roverv1.Rover{}

			err := mapSubscriptions(input, out)

			Expect(err).To(BeNil())
			snaps.MatchSnapshot(GinkgoT(), out)
		})
	})

	Context("MapPermissions", func() {
		It("must map permissions in flat format", func() {
			input := apiRover
			input.Authorization = []api.AuthorizationInfo{
				{
					Resource: "stargate:payment:v1",
					Role:     "admin",
					Actions:  []string{"read", "write"},
				},
			}
			out := &roverv1.Rover{}

			err := mapPermissions(input, out)

			Expect(err).To(BeNil())
			snaps.MatchSnapshot(GinkgoT(), out)
		})

		It("must map permissions in resource-oriented format", func() {
			input := apiRover
			input.Authorization = []api.AuthorizationInfo{
				{
					Resource: "stargate:payment:v1",
					Permissions: []api.AuthorizationPermissionInfo{
						{
							Role:    "admin",
							Actions: []string{"read", "write", "delete"},
						},
						{
							Role:    "viewer",
							Actions: []string{"read"},
						},
					},
				},
			}
			out := &roverv1.Rover{}

			err := mapPermissions(input, out)

			Expect(err).To(BeNil())
			snaps.MatchSnapshot(GinkgoT(), out)
		})

		It("must map permissions in role-oriented format", func() {
			input := apiRover
			input.Authorization = []api.AuthorizationInfo{
				{
					Role: "admin",
					Permissions: []api.AuthorizationPermissionInfo{
						{
							Resource: "stargate:payment:v1",
							Actions:  []string{"read", "write"},
						},
						{
							Resource: "stargate:billing:v1",
							Actions:  []string{"read", "write"},
						},
					},
				},
			}
			out := &roverv1.Rover{}

			err := mapPermissions(input, out)

			Expect(err).To(BeNil())
			snaps.MatchSnapshot(GinkgoT(), out)
		})

		It("must handle empty permissions", func() {
			input := apiRover
			input.Authorization = []api.AuthorizationInfo{}
			out := &roverv1.Rover{}

			err := mapPermissions(input, out)

			Expect(err).To(BeNil())
			Expect(out.Spec.Permissions).To(BeEmpty())
		})
	})

	Context("MapAuthentication", func() {
		It("must map BASIC to client_secret_basic in CRD", func() {
			input := &api.Rover{
				Zone: "zone",
				Authentication: api.Authentication{
					ClientAuthMethod: api.AuthenticationClientAuthMethodBASIC,
				},
			}
			output := &roverv1.Rover{}

			err := MapRover(input, output)

			Expect(err).ToNot(HaveOccurred())
			Expect(output.Spec.Authentication).ToNot(BeNil())
			Expect(output.Spec.Authentication.M2M).ToNot(BeNil())
			Expect(output.Spec.Authentication.M2M.TokenRequest).To(Equal(roverv1.TokenRequestClientSecretBasic))
		})

		It("must map POST to client_secret_post in CRD", func() {
			input := &api.Rover{
				Zone: "zone",
				Authentication: api.Authentication{
					ClientAuthMethod: api.AuthenticationClientAuthMethodPOST,
				},
			}
			output := &roverv1.Rover{}

			err := MapRover(input, output)

			Expect(err).ToNot(HaveOccurred())
			Expect(output.Spec.Authentication).ToNot(BeNil())
			Expect(output.Spec.Authentication.M2M).ToNot(BeNil())
			Expect(output.Spec.Authentication.M2M.TokenRequest).To(Equal(roverv1.TokenRequestClientSecretPost))
		})

		It("must not set authentication when clientAuthMethod is empty", func() {
			input := &api.Rover{
				Zone: "zone",
			}
			output := &roverv1.Rover{}

			err := MapRover(input, output)

			Expect(err).ToNot(HaveOccurred())
			Expect(output.Spec.Authentication).To(BeNil())
		})

		It("must fuzzy-match 'basic' to client_secret_basic in CRD", func() {
			input := &api.Rover{
				Zone: "zone",
				Authentication: api.Authentication{
					ClientAuthMethod: "basic",
				},
			}
			output := &roverv1.Rover{}

			err := MapRover(input, output)

			Expect(err).ToNot(HaveOccurred())
			Expect(output.Spec.Authentication).ToNot(BeNil())
			Expect(output.Spec.Authentication.M2M).ToNot(BeNil())
			Expect(output.Spec.Authentication.M2M.TokenRequest).To(Equal(roverv1.TokenRequestClientSecretBasic))
		})

		It("must fuzzy-match 'body' to client_secret_post in CRD", func() {
			input := &api.Rover{
				Zone: "zone",
				Authentication: api.Authentication{
					ClientAuthMethod: "body",
				},
			}
			output := &roverv1.Rover{}

			err := MapRover(input, output)

			Expect(err).ToNot(HaveOccurred())
			Expect(output.Spec.Authentication).ToNot(BeNil())
			Expect(output.Spec.Authentication.M2M).ToNot(BeNil())
			Expect(output.Spec.Authentication.M2M.TokenRequest).To(Equal(roverv1.TokenRequestClientSecretPost))
		})
	})

	Context("MapRequest", func() {
		DescribeTable("normalizes names and application labels at Kubernetes length boundaries",
			func(length int) {
				name := strings.Repeat("a", length)
				id := mapper.ResourceIdInfo{Name: name, Environment: "poc", Namespace: "eni--hyperion"}
				request := &api.RoverUpdateRequest{Zone: "zone"}

				output, err := MapRequest(request, id)
				Expect(err).NotTo(HaveOccurred())
				Expect(validation.IsDNS1123Subdomain(output.Name)).To(BeEmpty())
				Expect(output.Namespace).To(Equal("poc--eni--hyperion"))
				if length <= 253 {
					Expect(output.Name).To(Equal(name))
				} else {
					Expect(output.Name).NotTo(Equal(name))
				}

				label := output.Labels[config.BuildLabelKey("application")]
				Expect(label).NotTo(BeEmpty())
				Expect(validation.IsValidLabelValue(label)).To(BeEmpty())
				if length <= 63 {
					Expect(label).To(Equal(name))
				} else {
					Expect(label).NotTo(Equal(name))
				}

				repeated, err := MapRequest(request, id)
				Expect(err).NotTo(HaveOccurred())
				Expect(repeated.ObjectMeta).To(Equal(output.ObjectMeta))
			},
			Entry("at the label limit", 63),
			Entry("above the label limit", 64),
			Entry("at the API name limit", 90),
			Entry("at the Kubernetes name limit", 253),
			Entry("above the Kubernetes name limit", 254),
		)

		It("normalizes name and environment characters", func() {
			id := mapper.ResourceIdInfo{Name: "--My_Rover/Name--", Environment: "--My_Env--", Namespace: "eni--hyperion"}
			output, err := MapRequest(&api.RoverUpdateRequest{Zone: "zone"}, id)
			Expect(err).NotTo(HaveOccurred())
			Expect(output.Name).To(Equal("my-rover-name"))
			Expect(output.Labels).To(HaveKeyWithValue(config.BuildLabelKey("application"), "my-rover-name"))
			Expect(output.Labels).To(HaveKeyWithValue(config.EnvironmentLabelKey, "my-env"))
		})

		It("shortens an oversized environment label", func() {
			id := mapper.ResourceIdInfo{Name: "rover", Environment: strings.Repeat("e", 64), Namespace: "eni--hyperion"}
			output, err := MapRequest(&api.RoverUpdateRequest{Zone: "zone"}, id)
			Expect(err).NotTo(HaveOccurred())
			label := output.Labels[config.EnvironmentLabelKey]
			Expect(label).NotTo(BeEmpty())
			Expect(validation.IsValidLabelValue(label)).To(BeEmpty())
		})

		It("keeps oversized names distinct when only their middle differs", func() {
			id := mapper.ResourceIdInfo{
				Name:        strings.Repeat("a", 127) + "b" + strings.Repeat("z", 127),
				Environment: "poc", Namespace: "eni--hyperion",
			}
			request := &api.RoverUpdateRequest{Zone: "zone"}
			first, err := MapRequest(request, id)
			Expect(err).NotTo(HaveOccurred())
			id.Name = strings.Repeat("a", 127) + "c" + strings.Repeat("z", 127)
			second, err := MapRequest(request, id)
			Expect(err).NotTo(HaveOccurred())
			Expect(first.Name).NotTo(Equal(second.Name))
			Expect(first.Labels[config.BuildLabelKey("application")]).NotTo(Equal(second.Labels[config.BuildLabelKey("application")]))
		})

		It("must map a RoverUpdateRequest to a Rover correctly", func() {
			output, err := MapRequest(roverUpdateRequest, resourceIdInfo)

			Expect(err).To(BeNil())

			Expect(output).ToNot(BeNil())
			snaps.MatchSnapshot(GinkgoT(), output)
		})

		It("must return an error if the input rover is nil", func() {
			output, err := MapRequest(nil, resourceIdInfo)

			Expect(output).ToNot(BeNil())
			snaps.MatchSnapshot(GinkgoT(), output)

			Expect(err).ToNot(BeNil())
			Expect(err.Error()).To(ContainSubstring("input rover update request is nil"))
		})

		It("must set the clientSecret if provided", func() {
			input := roverUpdateRequest
			input.ClientSecret = "supersecret"

			output, err := MapRequest(input, resourceIdInfo)

			Expect(err).To(BeNil())

			Expect(output).ToNot(BeNil())
			snaps.MatchSnapshot(GinkgoT(), output)

			MigrationActive = true
			defer func() { MigrationActive = false }()

			output, err = MapRequest(input, resourceIdInfo)

			Expect(err).To(BeNil())

			Expect(output).ToNot(BeNil())
			snaps.MatchSnapshot(GinkgoT(), output)
		})

	})

	Context("ExternalIds translation", func() {
		It("packs psiid + icto scalars into the internal ExternalIds list", func() {
			input := &api.Rover{
				Zone:  "zone",
				Psiid: "PSI-103596",
				Icto:  "icto-12345",
			}
			output := &roverv1.Rover{}
			Expect(MapRover(input, output)).To(Succeed())
			Expect(output.Spec.ExternalIds).To(ConsistOf(
				roverv1.ExternalId{Scheme: "psi", Id: "PSI-103596"},
				roverv1.ExternalId{Scheme: "icto", Id: "icto-12345"},
			))
		})

		It("produces no ExternalIds when all scalars are empty", func() {
			input := &api.Rover{Zone: "zone"}
			output := &roverv1.Rover{}
			Expect(MapRover(input, output)).To(Succeed())
			Expect(output.Spec.ExternalIds).To(BeNil())
		})
	})
})
