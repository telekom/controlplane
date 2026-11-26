// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package resolvers_test

import (
	"github.com/telekom/controlplane/controlplane-api/ent"
	"github.com/telekom/controlplane/controlplane-api/internal/resolvers"
	"github.com/telekom/controlplane/controlplane-api/internal/service"
	"github.com/telekom/controlplane/controlplane-api/internal/testutil"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Api changelog and roadmap URLs", func() {
	var (
		client *ent.Client
		s      *testutil.SeedData
	)

	BeforeEach(func() {
		client = testutil.NewTestClient(GinkgoT())
		s = testutil.SeedStandard(client)
	})

	AfterEach(func() {
		client.Close()
	})

	It("builds both URLs from the stored file IDs", func() {
		ctx := testutil.AllowContext()
		r := resolvers.NewResolver(client, service.Services{}, nil, "https://files.example.com/api")
		api, err := client.Api.Create().
			SetNamespace("default").
			SetBasePath("/eni/docs/v1").
			SetVersion("1.0.0").
			SetChangelog("changelog-id").
			SetRoadmap("roadmap-id").
			SetOwner(s.TeamAlpha).
			Save(ctx)
		Expect(err).NotTo(HaveOccurred())

		changelogURL, err := r.Api().ChangelogURL(ctx, api)
		Expect(err).NotTo(HaveOccurred())
		Expect(changelogURL).To(HaveValue(Equal("https://files.example.com/api/v1/files/changelog-id")))

		roadmapURL, err := r.Api().RoadmapURL(ctx, api)
		Expect(err).NotTo(HaveOccurred())
		Expect(roadmapURL).To(HaveValue(Equal("https://files.example.com/api/v1/files/roadmap-id")))
	})

	It("returns nil when no changelog or roadmap exists", func() {
		ctx := testutil.AllowContext()
		r := resolvers.NewResolver(client, service.Services{}, nil, "https://files.example.com/api")
		api, err := client.Api.Create().
			SetNamespace("default").
			SetBasePath("/eni/plain/v1").
			SetVersion("1.0.0").
			SetOwner(s.TeamAlpha).
			Save(ctx)
		Expect(err).NotTo(HaveOccurred())

		changelogURL, err := r.Api().ChangelogURL(ctx, api)
		Expect(err).NotTo(HaveOccurred())
		Expect(changelogURL).To(BeNil())
		roadmapURL, err := r.Api().RoadmapURL(ctx, api)
		Expect(err).NotTo(HaveOccurred())
		Expect(roadmapURL).To(BeNil())
	})

	It("returns nil when the file-manager base URL is not configured", func() {
		ctx := testutil.AllowContext()
		r := resolvers.NewResolver(client, service.Services{}, nil, "")
		api, err := client.Api.Create().
			SetNamespace("default").
			SetBasePath("/eni/nobase/v1").
			SetVersion("1.0.0").
			SetChangelog("changelog-id").
			SetOwner(s.TeamAlpha).
			Save(ctx)
		Expect(err).NotTo(HaveOccurred())

		changelogURL, err := r.Api().ChangelogURL(ctx, api)
		Expect(err).NotTo(HaveOccurred())
		Expect(changelogURL).To(BeNil())
	})
})
