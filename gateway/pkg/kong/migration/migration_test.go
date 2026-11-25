// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package migration_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
	"github.com/telekom/controlplane/gateway/pkg/kong/migration"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

type body struct {
	Name   *string        `json:"name,omitempty"`
	Tags   []string       `json:"tags"`
	Paths  []string       `json:"paths"`
	Config map[string]any `json:"config"`
}

func ptr(s string) *string { return &s }

func isBlocked(err error) bool {
	var blocked interface{ IsBlocked() bool }
	return errors.As(err, &blocked) && blocked.IsBlocked()
}

var _ = Describe("Compare", func() {
	paths := func(diffs []migration.Diff) []string {
		out := make([]string, 0, len(diffs))
		for _, d := range diffs {
			out = append(out, d.Path)
		}
		return out
	}

	It("treats null, empty arrays and empty objects as absent", func() {
		diffs, err := migration.Compare(body{Tags: []string{}, Config: map[string]any{"a": nil}}, body{})
		Expect(err).NotTo(HaveOccurred())
		Expect(diffs).To(BeEmpty())
	})

	It("reports nested fields and array elements by path", func() {
		diffs, err := migration.Compare(
			body{Name: ptr("bond--echo"), Paths: []string{"/a", "/b"}, Config: map[string]any{"limit": 10, "keep": "x"}},
			body{Name: ptr("bond.echo"), Paths: []string{"/a", "/c"}, Config: map[string]any{"limit": 5, "keep": "x", "extra": true}},
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(paths(diffs)).To(Equal([]string{"$.config.extra", "$.config.limit", "$.name", "$.paths[1]"}))
		Expect(diffs[0].Desired).To(BeNil())
		Expect(diffs[0].Current).To(BeTrue())
		Expect(diffs[1].Desired).To(Equal(json.Number("10")))
	})

	It("reports arrays of different length as a whole", func() {
		diffs, err := migration.Compare(body{Tags: []string{"a", "b"}}, body{Tags: []string{"a"}})
		Expect(err).NotTo(HaveOccurred())
		Expect(paths(diffs)).To(Equal([]string{"$.tags"}))
	})
})

var _ = Describe("NameOf", func() {
	DescribeTable("returns the naming field",
		func(entity any, expected string) {
			Expect(migration.NameOf(entity)).To(Equal(expected))
		},
		Entry("name", map[string]any{"name": "bond--echo", "target": "x"}, "bond--echo"),
		Entry("target", map[string]any{"target": "host:443"}, "host:443"),
		Entry("username", map[string]any{"username": "eni--team"}, "eni--team"),
		Entry("none", map[string]any{"id": "1"}, ""),
	)
})

var _ = Describe("Guard", func() {
	var (
		ctx  context.Context
		plan *migration.Plan
	)

	BeforeEach(func() {
		plan = migration.NewPlan()
		ctx = migration.WithPlan(context.Background(), plan)
	})

	guard := func(mode migration.Mode, rules ...migration.Rule) *migration.Guard {
		g, err := migration.New(migration.Config{Ignore: rules}, mode)
		Expect(err).NotTo(HaveOccurred())
		return g
	}
	route := func(approved string) *metav1.ObjectMeta {
		meta := &metav1.ObjectMeta{Name: "eni-echo-v1"}
		if approved != "" {
			meta.Annotations = map[string]string{migration.ApprovalAnnotation: approved}
		}
		return meta
	}
	// record plans an update of the route tags and a create of a plugin.
	record := func(g *migration.Guard) {
		write, err := g.CheckWrite(ctx, "route", "bond--eni-echo-v1", body{Tags: []string{"new"}}, body{Tags: []string{"old"}}, true)
		Expect(err).NotTo(HaveOccurred())
		Expect(write).To(BeFalse())
		write, err = g.CheckWrite(ctx, "plugin", "acl", body{Config: map[string]any{"allow": []any{"a"}}}, nil, false)
		Expect(err).NotTo(HaveOccurred())
		Expect(write).To(BeFalse())
	}

	It("allows everything when it is nil", func() {
		var g *migration.Guard
		Expect(g.CheckWrite(ctx, "route", "r", body{Name: ptr("a")}, body{Name: ptr("b")}, true)).To(BeTrue())
		Expect(g.CheckDelete(ctx, "route", "r")).To(BeTrue())
		Expect(g.Decide(ctx, plan, "Route", route(""))).To(BeNil())
		Expect(g.Mode()).To(Equal(migration.ModeOff))
	})

	It("writes without a check while a plan is applied", func() {
		apply := migration.WithApply(context.Background())
		Expect(guard(migration.ModeStrict).CheckWrite(apply, "route", "r", body{Name: ptr("a")}, nil, false)).To(BeTrue())
		Expect(guard(migration.ModeStrict).CheckDelete(apply, "route", "r")).To(BeTrue())
	})

	It("records the whole bundle and blocks it on an unexpected diff", func() {
		g := guard(migration.ModeOn, migration.Rule{Kind: "route", Paths: []string{"$.tags"}})
		record(g)
		Expect(g.CheckDelete(ctx, "upstream", "bond--eni-echo-v1")).To(BeFalse())

		status, err := g.Decide(ctx, plan, "Route", route(""))
		Expect(isBlocked(err)).To(BeTrue())
		Expect(err).To(MatchError(And(
			ContainSubstring("3 kong change(s) have 2 unexpected diff(s)"),
			ContainSubstring(`plugin "acl" $`),
			ContainSubstring(`upstream "bond--eni-echo-v1" $`),
			ContainSubstring(migration.ApprovalAnnotation+`="`+status.Hash+`"`),
		)))
		Expect(status.Mode).To(Equal("on"))
		Expect(status.Changes).To(HaveLen(3))
		Expect(status.Changes[0].Operation).To(Equal("update"))
		Expect(status.Changes[0].Diffs).To(ConsistOf(gatewayv1.MigrationDiff{Path: "$.tags[0]", Desired: `"new"`, Current: `"old"`, Ignored: true}))
		Expect(status.Changes[1].Operation).To(Equal("create"))
		Expect(status.Changes[1].Diffs[0].Desired).To(Equal(`{"config":{"allow":["a"]}}`))
		Expect(status.Changes[2].Operation).To(Equal("delete"))
	})

	It("allows a bundle whose diffs are all ignored in mode on", func() {
		g := guard(migration.ModeOn, migration.Rule{Kind: "route", Paths: []string{"$.tags"}}, migration.Rule{Kind: "plugin", Paths: []string{"$"}})
		record(g)
		Expect(g.Decide(ctx, plan, "Route", route(""))).To(BeNil())
	})

	It("needs an approval of the hash in strict mode even if every diff is ignored", func() {
		g := guard(migration.ModeStrict, migration.Rule{Kind: "route", Paths: []string{"$"}}, migration.Rule{Kind: "plugin", Paths: []string{"$"}})
		record(g)

		status, err := g.Decide(ctx, plan, "Route", route(""))
		Expect(err).To(MatchError(ContainSubstring("migration mode strict: 2 kong change(s) need approval")))
		Expect(status.Mode).To(Equal("strict"))

		_, err = g.Decide(ctx, plan, "Route", route("0123456789abcdef"))
		Expect(err).To(HaveOccurred())
		Expect(g.Decide(ctx, plan, "Route", route(status.Hash))).To(BeNil())
	})

	It("allows a bundle without changes in strict mode", func() {
		Expect(guard(migration.ModeStrict).Decide(ctx, plan, "Route", route(""))).To(BeNil())
	})

	It("changes the hash with any value of the bundle", func() {
		g := guard(migration.ModeOn)
		record(g)
		first := plan.Hash()

		again := migration.NewPlan()
		ctx = migration.WithPlan(context.Background(), again)
		record(g)
		Expect(again.Hash()).To(Equal(first))

		changed := migration.NewPlan()
		ctx = migration.WithPlan(context.Background(), changed)
		Expect(g.CheckWrite(ctx, "route", "bond--eni-echo-v1", body{Tags: []string{"new"}}, body{Tags: []string{"java-edit"}}, true)).To(BeFalse())
		Expect(changed.Hash()).NotTo(Equal(first))
	})

	It("truncates long values in the status", func() {
		long := make([]string, 300)
		for i := range long {
			long[i] = "value"
		}
		g := guard(migration.ModeOn)
		Expect(g.CheckWrite(ctx, "route", "r", body{Tags: long}, nil, false)).To(BeFalse())
		status, _ := g.Decide(ctx, plan, "Route", route(""))
		Expect(status.Changes[0].Diffs[0].Desired).To(HaveSuffix("...(truncated)"))
	})

	It("names at most five unexpected paths", func() {
		g := guard(migration.ModeOn)
		Expect(g.CheckWrite(ctx, "plugin", "rate-limiting",
			body{Config: map[string]any{"a": 1, "b": 1, "c": 1, "d": 1, "e": 1, "f": 1, "g": 1}}, body{}, true)).To(BeFalse())
		_, err := g.Decide(ctx, plan, "Route", route(""))
		Expect(err).To(MatchError(And(ContainSubstring("7 unexpected diff(s)"), ContainSubstring("and 2 more"))))
	})

	Describe("outside a plan", func() {
		BeforeEach(func() { ctx = context.Background() })

		It("writes a change whose diffs are all ignored", func() {
			g := guard(migration.ModeOn, migration.Rule{Kind: "route", Paths: []string{"$.tags"}})
			Expect(g.CheckWrite(ctx, "route", "r", body{Tags: []string{"a"}}, body{}, true)).To(BeTrue())
		})

		It("blocks an unexpected diff because it cannot be approved", func() {
			_, err := guard(migration.ModeOn).CheckWrite(ctx, "route", "r", body{Name: ptr("a")}, body{}, true)
			Expect(isBlocked(err)).To(BeTrue())
			Expect(err).To(MatchError(ContainSubstring("not part of a planned reconcile")))
		})

		It("blocks every change in strict mode", func() {
			_, err := guard(migration.ModeStrict, migration.Rule{Kind: "route", Paths: []string{"$"}}).CheckDelete(ctx, "route", "r")
			Expect(isBlocked(err)).To(BeTrue())
		})
	})

	DescribeTable("ignores the diffs a rule covers",
		func(r migration.Rule, kind, name string, desired, current body, ignored bool) {
			g := guard(migration.ModeOn, r)
			Expect(g.CheckWrite(ctx, kind, name, desired, current, true)).To(BeFalse())
			Expect(plan.Changes()[0].Diffs[0].Ignored).To(Equal(ignored))
		},
		Entry("path below the rule path", migration.Rule{Kind: "plugin", Paths: []string{"$.config"}},
			"plugin", "acl", body{Config: map[string]any{"allow": []any{"a"}}}, body{}, true),
		Entry("key wildcard", migration.Rule{Kind: "plugin", Paths: []string{"$.*.limit"}},
			"plugin", "rate-limiting", body{Config: map[string]any{"limit": 1}}, body{Config: map[string]any{"limit": 2}}, true),
		Entry("index wildcard", migration.Rule{Kind: "route", Paths: []string{"$.paths[*]"}},
			"route", "r", body{Paths: []string{"/a"}}, body{Paths: []string{"/b"}}, true),
		Entry("matching name", migration.Rule{Kind: "route", Name: "bond--.*", Paths: []string{"$.name"}},
			"route", "bond--echo", body{Name: ptr("a")}, body{Name: ptr("b")}, true),
		Entry("other name", migration.Rule{Kind: "route", Name: "bond--.*", Paths: []string{"$.name"}},
			"route", "cit2--echo", body{Name: ptr("a")}, body{Name: ptr("b")}, false),
		Entry("partial name match", migration.Rule{Kind: "route", Name: "bond", Paths: []string{"$.name"}},
			"route", "bond--echo", body{Name: ptr("a")}, body{Name: ptr("b")}, false),
		Entry("other kind", migration.Rule{Kind: "service", Paths: []string{"$.name"}},
			"route", "r", body{Name: ptr("a")}, body{Name: ptr("b")}, false),
		Entry("sibling path", migration.Rule{Kind: "route", Paths: []string{"$.tags"}},
			"route", "r", body{Paths: []string{"/a"}}, body{}, false),
	)

	DescribeTable("rejects an invalid config",
		func(r migration.Rule, message string) {
			_, err := migration.New(migration.Config{Ignore: []migration.Rule{r}}, migration.ModeOn)
			Expect(err).To(MatchError(ContainSubstring(message)))
		},
		Entry("unknown kind", migration.Rule{Kind: "routes", Paths: []string{"$"}}, `unknown kind "routes"`),
		Entry("no paths", migration.Rule{Kind: "route"}, "paths must not be empty"),
		Entry("invalid name", migration.Rule{Kind: "route", Name: "(", Paths: []string{"$"}}, "invalid name pattern"),
		Entry("path without root", migration.Rule{Kind: "route", Paths: []string{"tags"}}, `must start with "$"`),
		Entry("empty key", migration.Rule{Kind: "route", Paths: []string{"$..tags"}}, "empty key"),
		Entry("unclosed index", migration.Rule{Kind: "route", Paths: []string{"$.paths[0"}}, "unclosed index"),
		Entry("invalid index", migration.Rule{Kind: "route", Paths: []string{"$.paths[x]"}}, `invalid index "x"`),
	)

	It("rejects a guard for mode off", func() {
		_, err := migration.New(migration.Config{}, migration.ModeOff)
		Expect(err).To(HaveOccurred())
	})
})

var _ = Describe("Planned entities", func() {
	It("registers a planned ID that is found by name and by ID", func() {
		ctx := migration.WithPlan(context.Background(), migration.NewPlan())
		id := migration.PlannedID(ctx, "route", "bond--echo")
		Expect(migration.IsPlanned(id)).To(BeTrue())
		for _, ref := range []string{"bond--echo", id} {
			found, ok := migration.LookupPlanned(ctx, "route", ref)
			Expect(ok).To(BeTrue())
			Expect(found).To(Equal(id))
		}
		_, ok := migration.LookupPlanned(ctx, "service", "bond--echo")
		Expect(ok).To(BeFalse())
	})

	It("plans nothing without a plan", func() {
		Expect(migration.PlannedID(context.Background(), "route", "r")).To(BeEmpty())
		Expect(migration.IsPlanned("4f1c")).To(BeFalse())
	})
})

var _ = Describe("Load", func() {
	write := func(content string) string {
		path := filepath.Join(GinkgoT().TempDir(), "ignore.yaml")
		Expect(os.WriteFile(path, []byte(content), 0o600)).To(Succeed())
		return path
	}

	It("loads the ignore list", func() {
		g, err := migration.Load(write("ignore:\n  - kind: route\n    paths: [\"$.tags\"]\n"), migration.ModeOn)
		Expect(err).NotTo(HaveOccurred())
		Expect(g.CheckWrite(context.Background(), "route", "r", body{Tags: []string{"a"}}, body{}, true)).To(BeTrue())
	})

	It("blocks every diff without a file", func() {
		g, err := migration.Load("", migration.ModeOn)
		Expect(err).NotTo(HaveOccurred())
		_, err = g.CheckDelete(context.Background(), "route", "r")
		Expect(isBlocked(err)).To(BeTrue())
	})

	It("gives no guard when the mode is off", func() {
		g, err := migration.Load("", migration.ModeOff)
		Expect(err).NotTo(HaveOccurred())
		Expect(g).To(BeNil())
	})

	DescribeTable("rejects an invalid file",
		func(content, message string) {
			_, err := migration.Load(write(content), migration.ModeOn)
			Expect(err).To(MatchError(ContainSubstring(message)))
		},
		Entry("unknown field", "ignored: []\n", "parsing migration config"),
		Entry("invalid rule", "ignore:\n  - kind: route\n", "paths must not be empty"),
	)
})

var _ = Describe("ParseMode", func() {
	DescribeTable("parses the flag value",
		func(value string, expected migration.Mode) {
			Expect(migration.ParseMode(value)).To(Equal(expected))
		},
		Entry("off", "off", migration.ModeOff),
		Entry("on", "on", migration.ModeOn),
		Entry("strict", "strict", migration.ModeStrict),
	)

	It("rejects an unknown value", func() {
		_, err := migration.ParseMode("true")
		Expect(err).To(MatchError(ContainSubstring(`invalid migration mode "true"`)))
	})
})

var _ = Describe("IsApproved", func() {
	DescribeTable("approves only the exact hash",
		func(annotation, hash string, expected bool) {
			meta := &metav1.ObjectMeta{Annotations: map[string]string{migration.ApprovalAnnotation: annotation}}
			Expect(migration.IsApproved(meta, hash)).To(Equal(expected))
		},
		Entry("same hash", "0123456789abcdef", "0123456789abcdef", true),
		Entry("other hash", "0123456789abcdef", "fedcba9876543210", false),
		Entry("empty hash", "", "", false),
	)
})
