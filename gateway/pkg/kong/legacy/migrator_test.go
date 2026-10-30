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

// snapshotDir holds synthetic Kong exports of routes written by the Java operator.
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

var eventLabels = map[string]string{"cp.ei.telekom.de/domain": "event"}

var _ = Describe("Migrator", func() {
	var migrator legacy.Migrator

	BeforeEach(func() {
		var err error
		migrator, err = legacy.LoadMigrator("testdata/legacy-naming.yaml")
		Expect(err).NotTo(HaveOccurred())
	})

	DescribeTable("finds the Java name of every snapshot",
		func(file, prefix, name string, labels map[string]string) {
			s := loadSnapshot(file)
			Expect(s.Route.Name).To(Equal(s.Service.Name))
			expected := legacy.Candidate{Name: s.Route.Name, OwnerTag: realmTag(s.Route.Tags)}
			Expect(realmTag(s.Service.Tags)).To(Equal(expected.OwnerTag))

			candidates := migrator.Candidates(legacy.Route{Name: name, NamePrefix: prefix, Labels: labels})
			Expect(candidates).To(ContainElement(expected))
		},
		Entry("default realm API route", "default-api.yaml", "default", "eni-example-v2", nil),
		Entry("bond API route", "bond-api.yaml", "bond", "eni-example-v1", nil),
		Entry("default-ai route", "default-ai.yaml", "default", "ai-gateway--example-mcp-v1", nil),
		Entry("event publish route", "bond-publish.yaml", "bond", "publish", eventLabels),
		Entry("event callback route", "bond-callback.yaml", "bond", "callback--alpha", eventLabels),
		Entry("event sse route", "bond-sse.yaml", "bond", "sse--de-example-orders-v1", eventLabels),
		Entry("event voyager route", "bond-voyager.yaml", "bond", "voyager--alpha", eventLabels),
	)

	DescribeTable("returns candidates in realm suffix order",
		func(prefix, name string, labels map[string]string, expected []legacy.Candidate) {
			Expect(migrator.Candidates(legacy.Route{Name: name, NamePrefix: prefix, Labels: labels})).To(Equal(expected))
		},
		Entry("bond publish event", "bond", "publish", eventLabels, []legacy.Candidate{
			{Name: "bond.horizon-events", OwnerTag: "ei__telekom__de--realm---bond"},
			{Name: "bond-ai.horizon-events", OwnerTag: "ei__telekom__de--realm---bond-ai"},
		}),
		Entry("API route named publish is not an event route", "bond", "publish", map[string]string{"cp.ei.telekom.de/domain": "api"}, []legacy.Candidate{
			{Name: "bond.publish", OwnerTag: "ei__telekom__de--realm---bond"},
			{Name: "bond-ai.publish", OwnerTag: "ei__telekom__de--realm---bond-ai"},
		}),
		Entry("API route named publish without labels", "bond", "publish", nil, []legacy.Candidate{
			{Name: "bond.publish", OwnerTag: "ei__telekom__de--realm---bond"},
			{Name: "bond-ai.publish", OwnerTag: "ei__telekom__de--realm---bond-ai"},
		}),
		Entry("default AI route", "default", "ai-gateway--example-mcp-v1", nil, []legacy.Candidate{
			{Name: "ai-gateway--example-mcp-v1", OwnerTag: "ei__telekom__de--realm---default"},
			{Name: "default-ai.ai-gateway--example-mcp-v1", OwnerTag: "ei__telekom__de--realm---default-ai"},
		}),
		Entry("bond API route", "bond", "eni-example-v1", nil, []legacy.Candidate{
			{Name: "bond.eni-example-v1", OwnerTag: "ei__telekom__de--realm---bond"},
			{Name: "bond-ai.eni-example-v1", OwnerTag: "ei__telekom__de--realm---bond-ai"},
		}),
		Entry("default API route", "default", "eni-example-v2", nil, []legacy.Candidate{
			{Name: "eni-example-v2", OwnerTag: "ei__telekom__de--realm---default"},
			{Name: "default-ai.eni-example-v2", OwnerTag: "ei__telekom__de--realm---default-ai"},
		}),
		Entry("no name prefix", "", "eni-example-v1", nil, nil),
	)

	It("does not map an API route named publish to the event route", func() {
		candidates := migrator.Candidates(legacy.Route{Name: "publish", NamePrefix: "bond"})
		for _, c := range candidates {
			Expect(c.Name).NotTo(ContainSubstring("horizon-events"))
		}
	})

	It("removes duplicate names", func() {
		m, err := legacy.NewMigrator(&legacy.Config{DefaultRealm: "default", Separator: ".", RealmSuffixes: []string{"", ""}})
		Expect(err).NotTo(HaveOccurred())
		Expect(m.Candidates(legacy.Route{Name: "r", NamePrefix: "bond"})).To(HaveLen(1))
	})

	DescribeTable("requires a selector label to be present",
		func(labels map[string]string, expected string) {
			m, err := legacy.NewMigrator(&legacy.Config{Separator: ".", RealmSuffixes: []string{""}, Names: []legacy.NameRule{
				{Selector: map[string]string{"tier": ""}, Match: "^r$", Replace: "matched"},
			}})
			Expect(err).NotTo(HaveOccurred())
			Expect(m.Candidates(legacy.Route{Name: "r", NamePrefix: "p", Labels: labels})[0].Name).To(Equal(expected))
		},
		Entry("absent", nil, "p.r"),
		Entry("present and empty", map[string]string{"tier": ""}, "p.matched"),
		Entry("present and not empty", map[string]string{"tier": "x"}, "p.r"),
	)

	It("applies the first matching rule only", func() {
		m, err := legacy.NewMigrator(&legacy.Config{Separator: ".", RealmSuffixes: []string{""}, Names: []legacy.NameRule{
			{Match: "^a(.*)$", Replace: "first$1"},
			{Match: "^ab$", Replace: "second"},
		}})
		Expect(err).NotTo(HaveOccurred())
		Expect(m.Candidates(legacy.Route{Name: "ab", NamePrefix: "p"})).To(Equal([]legacy.Candidate{{Name: "p.firstb", OwnerTag: "ei__telekom__de--realm---p"}}))
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
			_, err := legacy.LoadMigrator(write("names: [unclosed"))
			Expect(err).To(MatchError(ContainSubstring("parsing legacy naming config")))
		})

		It("fails for unknown fields", func() {
			_, err := legacy.LoadMigrator(write("unknown: true\n"))
			Expect(err).To(MatchError(ContainSubstring("parsing legacy naming config")))
		})

		It("fails for an invalid regular expression", func() {
			_, err := legacy.LoadMigrator(write("names:\n  - match: '^(unclosed$'\n    replace: x\n"))
			Expect(err).To(MatchError(ContainSubstring("compiling match")))
		})

		It("fails for an empty match", func() {
			_, err := legacy.LoadMigrator(write("names:\n  - replace: x\n"))
			Expect(err).To(MatchError(ContainSubstring("match is empty")))
		})
	})
})
