// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package naming_test

import (
	"github.com/telekom/controlplane/gateway/pkg/kong/legacy"
	"github.com/telekom/controlplane/gateway/pkg/kong/naming"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

type fixedMigrator []legacy.Candidate

func (m fixedMigrator) Candidates(legacy.Route) []legacy.Candidate { return m }

var eventLabels = map[string]string{"cp.ei.telekom.de/domain": "event"}

func snapshotMigrator() legacy.Migrator {
	m, err := legacy.LoadMigrator("../legacy/testdata/legacy-naming.yaml")
	Expect(err).NotTo(HaveOccurred())
	return m
}

var _ = Describe("NameScheme", func() {
	DescribeTable("DesiredName",
		func(prefix, name, expected string) {
			Expect(naming.DesiredName(prefix, name)).To(Equal(expected))
		},
		Entry("bond", "bond", "eni-example-v1", "bond--eni-example-v1"),
		Entry("default", "default", "eni-foo-v1", "default--eni-foo-v1"),
		Entry("default AI", "default", "ai-gateway--mcp-v1", "default--ai-gateway--mcp-v1"),
		Entry("no prefix", "", "eni-foo-v1", "eni-foo-v1"),
	)

	DescribeTable("Names from the snapshot config",
		func(route legacy.Route, expected naming.Names) {
			Expect(naming.NewNameScheme(snapshotMigrator()).Names(route)).To(Equal(expected))
		},
		Entry("default API route: Java name equals metadata.name",
			legacy.Route{Name: "eni-example-v2", NamePrefix: "default"},
			naming.Names{
				Desired:               "default--eni-example-v2",
				PreMigration:          "eni-example-v2",
				PreMigrationOwnerTags: []string{"ei__telekom__de--realm---default"},
				Candidates: []legacy.Candidate{
					{Name: "default-ai.eni-example-v2", OwnerTag: "ei__telekom__de--realm---default-ai"},
				},
			}),
		Entry("default AI route",
			legacy.Route{Name: "ai-gateway--example-mcp-v1", NamePrefix: "default"},
			naming.Names{
				Desired:               "default--ai-gateway--example-mcp-v1",
				PreMigration:          "ai-gateway--example-mcp-v1",
				PreMigrationOwnerTags: []string{"ei__telekom__de--realm---default"},
				Candidates: []legacy.Candidate{
					{Name: "default-ai.ai-gateway--example-mcp-v1", OwnerTag: "ei__telekom__de--realm---default-ai"},
				},
			}),
		Entry("bond API route",
			legacy.Route{Name: "eni-example-v1", NamePrefix: "bond"},
			naming.Names{
				Desired:      "bond--eni-example-v1",
				PreMigration: "eni-example-v1",
				Candidates: []legacy.Candidate{
					{Name: "bond.eni-example-v1", OwnerTag: "ei__telekom__de--realm---bond"},
					{Name: "bond-ai.eni-example-v1", OwnerTag: "ei__telekom__de--realm---bond-ai"},
				},
			}),
		Entry("bond publish event route",
			legacy.Route{Name: "publish", NamePrefix: "bond", Labels: eventLabels},
			naming.Names{
				Desired:      "bond--publish",
				PreMigration: "publish",
				Candidates: []legacy.Candidate{
					{Name: "bond.horizon-events", OwnerTag: "ei__telekom__de--realm---bond"},
					{Name: "bond-ai.horizon-events", OwnerTag: "ei__telekom__de--realm---bond-ai"},
				},
			}),
		Entry("bond callback event route",
			legacy.Route{Name: "callback--alpha", NamePrefix: "bond", Labels: eventLabels},
			naming.Names{
				Desired:      "bond--callback--alpha",
				PreMigration: "callback--alpha",
				Candidates: []legacy.Candidate{
					{Name: "bond.horizon-callback-alpha", OwnerTag: "ei__telekom__de--realm---bond"},
					{Name: "bond-ai.horizon-callback-alpha", OwnerTag: "ei__telekom__de--realm---bond-ai"},
				},
			}),
	)

	It("drops candidates equal to the desired name and duplicates", func() {
		scheme := naming.NewNameScheme(fixedMigrator{
			{Name: "p--r", OwnerTag: "a"},
			{Name: "x", OwnerTag: "b"},
			{Name: "x", OwnerTag: "c"},
			{Name: "r", OwnerTag: "d"},
			{Name: "r", OwnerTag: "d"},
		})
		Expect(scheme.Names(legacy.Route{Name: "r", NamePrefix: "p"})).To(Equal(naming.Names{
			Desired:               "p--r",
			PreMigration:          "r",
			PreMigrationOwnerTags: []string{"d"},
			Candidates:            []legacy.Candidate{{Name: "x", OwnerTag: "b"}},
		}))
	})

	It("works without a Migrator", func() {
		Expect(naming.NewNameScheme(nil).Names(legacy.Route{Name: "r", NamePrefix: "p"})).To(Equal(naming.Names{
			Desired: "p--r", PreMigration: "r",
		}))
	})
})
