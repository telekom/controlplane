// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package legacy_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"

	"sigs.k8s.io/yaml"

	"github.com/telekom/controlplane/gateway/pkg/kong/legacy"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// snapshotDir holds synthetic Kong exports of routes using the legacy naming scheme.
const snapshotDir = "testdata/java-routes"

type snapshotEntity struct {
	Name string   `json:"name"`
	Tags []string `json:"tags"`
}

type snapshot struct {
	Service snapshotEntity `json:"service"`
	Route   snapshotEntity `json:"route"`
}

func loadSnapshot(file string) snapshot {
	data, err := os.ReadFile(filepath.Join(snapshotDir, file))
	Expect(err).NotTo(HaveOccurred())
	s := snapshot{}
	Expect(yaml.Unmarshal(data, &s)).To(Succeed())
	return s
}

func realmTag(tags []string) string {
	i := slices.IndexFunc(tags, func(tag string) bool { return strings.HasPrefix(tag, legacy.RealmTagPrefix) })
	Expect(i).To(BeNumerically(">=", 0))
	return tags[i]
}

var _ = Describe("Migrator", func() {
	var migrator legacy.Migrator

	BeforeEach(func() {
		var err error
		migrator, err = legacy.LoadMigrator("testdata/legacy-naming.yaml")
		Expect(err).NotTo(HaveOccurred())
	})

	DescribeTable("finds the legacy name of every snapshot",
		func(file, prefix, name string) {
			s := loadSnapshot(file)
			Expect(s.Route.Name).To(Equal(s.Service.Name))
			expected := legacy.Candidate{Name: s.Route.Name, OwnerTag: realmTag(s.Route.Tags)}
			Expect(realmTag(s.Service.Tags)).To(Equal(expected.OwnerTag))

			candidates := migrator.Candidates(legacy.Route{Name: name, NamePrefix: prefix})
			Expect(candidates).To(ContainElement(expected))
		},
		Entry("default realm API route", "default-api.yaml", "default", "eni-example-v2"),
		Entry("bond API route", "bond-api.yaml", "bond", "eni-example-v1"),
		Entry("default-ai route", "default-ai.yaml", "default", "ai-gateway--example-mcp-v1"),
		Entry("event publish route", "bond-publish.yaml", "bond", "horizon-events"),
		Entry("event callback route", "bond-callback.yaml", "bond", "horizon-callback-alpha"),
		Entry("event sse route", "bond-sse.yaml", "bond", "horizon-sse--de-example-orders-v1"),
		Entry("event voyager route", "bond-voyager.yaml", "bond", "horizon-voyager-alpha"),
	)

	DescribeTable("returns candidates in realm suffix order",
		func(prefix, name string, expected []legacy.Candidate) {
			Expect(migrator.Candidates(legacy.Route{Name: name, NamePrefix: prefix})).To(Equal(expected))
		},
		Entry("bond event route", "bond", "horizon-events", []legacy.Candidate{
			{Name: "bond.horizon-events", OwnerTag: "ei__telekom__de--realm---bond"},
			{Name: "bond-ai.horizon-events", OwnerTag: "ei__telekom__de--realm---bond-ai"},
		}),
		Entry("route named publish", "bond", "publish", []legacy.Candidate{
			{Name: "bond.publish", OwnerTag: "ei__telekom__de--realm---bond"},
			{Name: "bond-ai.publish", OwnerTag: "ei__telekom__de--realm---bond-ai"},
		}),
		Entry("default AI route", "default", "ai-gateway--example-mcp-v1", []legacy.Candidate{
			{Name: "ai-gateway--example-mcp-v1", OwnerTag: "ei__telekom__de--realm---default"},
			{Name: "default-ai.ai-gateway--example-mcp-v1", OwnerTag: "ei__telekom__de--realm---default-ai"},
		}),
		Entry("bond API route", "bond", "eni-example-v1", []legacy.Candidate{
			{Name: "bond.eni-example-v1", OwnerTag: "ei__telekom__de--realm---bond"},
			{Name: "bond-ai.eni-example-v1", OwnerTag: "ei__telekom__de--realm---bond-ai"},
		}),
		Entry("default API route", "default", "eni-example-v2", []legacy.Candidate{
			{Name: "eni-example-v2", OwnerTag: "ei__telekom__de--realm---default"},
			{Name: "default-ai.eni-example-v2", OwnerTag: "ei__telekom__de--realm---default-ai"},
		}),
		Entry("no name prefix", "", "eni-example-v1", nil),
	)

	It("removes duplicate names", func() {
		m, err := legacy.NewMigrator(&legacy.Config{DefaultRealm: "default", Separator: ".", RealmSuffixes: []string{"", ""}})
		Expect(err).NotTo(HaveOccurred())
		Expect(m.Candidates(legacy.Route{Name: "r", NamePrefix: "bond"})).To(HaveLen(1))
	})

	Context("LoadMigrator", func() {
		write := func(content string) string {
			path := filepath.Join(GinkgoT().TempDir(), "cfg.yaml")
			Expect(os.WriteFile(path, []byte(content), 0o600)).To(Succeed())
			return path
		}

		It("gives no candidates for an empty path", func() {
			m, err := legacy.LoadMigrator("")
			Expect(err).NotTo(HaveOccurred())
			Expect(m.Candidates(legacy.Route{Name: "r", NamePrefix: "bond"})).To(BeEmpty())
		})

		It("fails for a missing file", func() {
			_, err := legacy.LoadMigrator(filepath.Join(GinkgoT().TempDir(), "missing.yaml"))
			Expect(err).To(MatchError(ContainSubstring("reading legacy naming config")))
		})

		It("fails for invalid YAML", func() {
			_, err := legacy.LoadMigrator(write("realmSuffixes: [unclosed"))
			Expect(err).To(MatchError(ContainSubstring("parsing legacy naming config")))
		})

		It("fails for unknown fields", func() {
			_, err := legacy.LoadMigrator(write("unknown: true\n"))
			Expect(err).To(MatchError(ContainSubstring("parsing legacy naming config")))
		})

		It("fails for name rules", func() {
			_, err := legacy.LoadMigrator(write("names:\n  - match: '^publish$'\n    replace: horizon-events\n"))
			Expect(err).To(MatchError(ContainSubstring("parsing legacy naming config")))
		})
	})
})
