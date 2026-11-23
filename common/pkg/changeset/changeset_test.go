// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package changeset_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/telekom/controlplane/common/pkg/changeset"
	"github.com/telekom/controlplane/common/pkg/types"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

type body struct {
	Name   *string        `json:"name,omitempty"`
	Tags   []string       `json:"tags"`
	Paths  []string       `json:"paths"`
	Config map[string]any `json:"config"`
}

// kinds are the entity kinds the tests register.
var kinds = []string{"route", "service", "upstream", "plugin", "consumer"}

func ptr(s string) *string { return &s }

func isBlocked(err error) bool {
	var blocked interface{ IsBlocked() bool }
	return errors.As(err, &blocked) && blocked.IsBlocked()
}

var _ = Describe("Compare", func() {
	paths := func(diffs []changeset.Diff) []string {
		out := make([]string, 0, len(diffs))
		for _, d := range diffs {
			out = append(out, d.Path)
		}
		return out
	}

	It("treats null, empty arrays and empty objects as absent", func() {
		diffs, err := changeset.Compare(body{Tags: []string{}, Config: map[string]any{"a": nil}}, body{})
		Expect(err).NotTo(HaveOccurred())
		Expect(diffs).To(BeEmpty())
	})

	It("reports nested fields and array elements by path", func() {
		diffs, err := changeset.Compare(
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
		diffs, err := changeset.Compare(body{Tags: []string{"a", "b"}}, body{Tags: []string{"a"}})
		Expect(err).NotTo(HaveOccurred())
		Expect(paths(diffs)).To(Equal([]string{"$.tags"}))
	})
})

var _ = Describe("NameOf", func() {
	DescribeTable("returns the naming field",
		func(entity any, expected string) {
			Expect(changeset.NameOf(entity)).To(Equal(expected))
		},
		Entry("name", map[string]any{"name": "bond--echo", "target": "x"}, "bond--echo"),
		Entry("target", map[string]any{"target": "host:443"}, "host:443"),
		Entry("username", map[string]any{"username": "eni--team"}, "eni--team"),
		Entry("id", map[string]any{"id": "sub-1"}, "sub-1"),
		Entry("none", map[string]any{"weight": 1}, ""),
	)
})

var _ = Describe("Gate", func() {
	var (
		ctx  context.Context
		plan *changeset.Plan
	)

	BeforeEach(func() {
		plan = changeset.NewPlan()
		ctx = changeset.WithPlan(context.Background(), plan)
	})

	guard := func(mode changeset.Mode, rules ...changeset.Rule) *changeset.Gate {
		g, err := changeset.New(changeset.Options{Mode: mode, Kinds: kinds}, changeset.Config{Ignore: rules})
		Expect(err).NotTo(HaveOccurred())
		return g
	}
	route := func(approved string) *metav1.ObjectMeta {
		meta := &metav1.ObjectMeta{Name: "eni-echo-v1"}
		if approved != "" {
			meta.Annotations = map[string]string{changeset.ApprovalAnnotation: approved}
		}
		return meta
	}
	// record plans an update of the route tags and a create of a plugin.
	record := func(g *changeset.Gate) {
		write, err := g.CheckWrite(ctx, "route", "bond--eni-echo-v1", body{Tags: []string{"new"}}, body{Tags: []string{"old"}}, true)
		Expect(err).NotTo(HaveOccurred())
		Expect(write).To(BeFalse())
		write, err = g.CheckWrite(ctx, "plugin", "acl", body{Config: map[string]any{"allow": []any{"a"}}}, nil, false)
		Expect(err).NotTo(HaveOccurred())
		Expect(write).To(BeFalse())
	}

	It("writes everything directly when it is nil", func() {
		var g *changeset.Gate
		Expect(g.CheckWrite(ctx, "route", "r", body{Name: ptr("a")}, body{Name: ptr("b")}, true)).To(BeTrue())
		Expect(g.CheckDelete(ctx, "route", "r")).To(BeTrue())
		Expect(g.Mode()).To(Equal(changeset.ModeOff))
	})

	It("allows every bundle in mode off", func() {
		g := guard(changeset.ModeOff)
		record(g)
		decision, status, err := g.Decide(ctx, plan, "Route", route(""))
		Expect(err).NotTo(HaveOccurred())
		Expect(decision).To(Equal(changeset.DecisionAllowed))
		Expect(status).To(BeNil())
	})

	It("writes without a check while a plan is applied", func() {
		apply := changeset.WithApply(context.Background())
		Expect(guard(changeset.ModeStrict).CheckWrite(apply, "route", "r", body{Name: ptr("a")}, nil, false)).To(BeTrue())
		Expect(guard(changeset.ModeStrict).CheckDelete(apply, "route", "r")).To(BeTrue())
	})

	It("records the whole bundle and blocks it on an unexpected diff", func() {
		g := guard(changeset.ModeOn, changeset.Rule{Kind: "route", Paths: []string{"$.tags"}})
		record(g)
		Expect(g.CheckDelete(ctx, "upstream", "bond--eni-echo-v1")).To(BeFalse())

		decision, status, err := g.Decide(ctx, plan, "Route", route(""))
		Expect(decision).To(Equal(changeset.DecisionBlocked))
		Expect(isBlocked(err)).To(BeTrue())
		Expect(err).To(MatchError(And(
			ContainSubstring("3 backend change(s) have 2 unexpected diff(s)"),
			ContainSubstring(`plugin "acl" $`),
			ContainSubstring(`upstream "bond--eni-echo-v1" $`),
			ContainSubstring(changeset.ApprovalAnnotation+`="`+status.Hash+`"`),
		)))
		Expect(status.Mode).To(Equal("on"))
		Expect(status.Changes).To(HaveLen(3))
		Expect(status.Changes[0].Operation).To(Equal("update"))
		Expect(status.Changes[0].Diffs).To(ConsistOf(types.Diff{Path: "$.tags[0]", Desired: `"new"`, Current: `"old"`, Ignored: true}))
		Expect(status.Changes[1].Operation).To(Equal("create"))
		Expect(status.Changes[1].Diffs[0].Desired).To(Equal(`{"config":{"allow":["a"]}}`))
		Expect(status.Changes[2].Operation).To(Equal("delete"))
	})

	It("allows a bundle whose diffs are all ignored in mode on", func() {
		g := guard(changeset.ModeOn, changeset.Rule{Kind: "route", Paths: []string{"$.tags"}}, changeset.Rule{Kind: "plugin", Paths: []string{"$"}})
		record(g)
		Expect(g.Decide(ctx, plan, "Route", route(""))).To(Equal(changeset.DecisionAllowed))
	})

	It("needs an approval of the hash in strict mode even if every diff is ignored", func() {
		g := guard(changeset.ModeStrict, changeset.Rule{Kind: "route", Paths: []string{"$"}}, changeset.Rule{Kind: "plugin", Paths: []string{"$"}})
		record(g)

		_, status, err := g.Decide(ctx, plan, "Route", route(""))
		Expect(err).To(MatchError(ContainSubstring("migration mode strict: 2 backend change(s) need approval")))
		Expect(status.Mode).To(Equal("strict"))

		_, _, err = g.Decide(ctx, plan, "Route", route("0123456789abcdef"))
		Expect(err).To(HaveOccurred())
		Expect(g.Decide(ctx, plan, "Route", route(status.Hash))).To(Equal(changeset.DecisionApproved))
	})

	It("allows a bundle without changes in strict mode", func() {
		Expect(guard(changeset.ModeStrict).Decide(ctx, plan, "Route", route(""))).To(Equal(changeset.DecisionUnchanged))
	})

	It("changes the hash with any value of the bundle", func() {
		g := guard(changeset.ModeOn)
		record(g)
		first := plan.Hash()

		again := changeset.NewPlan()
		ctx = changeset.WithPlan(context.Background(), again)
		record(g)
		Expect(again.Hash()).To(Equal(first))

		changed := changeset.NewPlan()
		ctx = changeset.WithPlan(context.Background(), changed)
		Expect(g.CheckWrite(ctx, "route", "bond--eni-echo-v1", body{Tags: []string{"new"}}, body{Tags: []string{"java-edit"}}, true)).To(BeFalse())
		Expect(changed.Hash()).NotTo(Equal(first))
	})

	It("truncates long values in the status", func() {
		long := make([]string, 300)
		for i := range long {
			long[i] = "value"
		}
		g := guard(changeset.ModeOn)
		Expect(g.CheckWrite(ctx, "route", "r", body{Tags: long}, nil, false)).To(BeFalse())
		_, status, _ := g.Decide(ctx, plan, "Route", route(""))
		Expect(status.Changes[0].Diffs[0].Desired).To(HaveSuffix("...(truncated)"))
		Expect(status.Truncated).To(BeTrue())
	})

	It("names at most five unexpected paths", func() {
		g := guard(changeset.ModeOn)
		Expect(g.CheckWrite(ctx, "plugin", "rate-limiting",
			body{Config: map[string]any{"a": 1, "b": 1, "c": 1, "d": 1, "e": 1, "f": 1, "g": 1}}, body{}, true)).To(BeFalse())
		_, _, err := g.Decide(ctx, plan, "Route", route(""))
		Expect(err).To(MatchError(And(ContainSubstring("7 unexpected diff(s)"), ContainSubstring("and 2 more"))))
	})

	Describe("outside a plan", func() {
		BeforeEach(func() { ctx = context.Background() })

		It("writes a change whose diffs are all ignored", func() {
			g := guard(changeset.ModeOn, changeset.Rule{Kind: "route", Paths: []string{"$.tags"}})
			Expect(g.CheckWrite(ctx, "route", "r", body{Tags: []string{"a"}}, body{}, true)).To(BeTrue())
		})

		It("blocks an unexpected diff because it cannot be approved", func() {
			_, err := guard(changeset.ModeOn).CheckWrite(ctx, "route", "r", body{Name: ptr("a")}, body{}, true)
			Expect(isBlocked(err)).To(BeTrue())
			Expect(err).To(MatchError(ContainSubstring("not part of a planned reconcile")))
		})

		It("blocks every change in strict mode", func() {
			_, err := guard(changeset.ModeStrict, changeset.Rule{Kind: "route", Paths: []string{"$"}}).CheckDelete(ctx, "route", "r")
			Expect(isBlocked(err)).To(BeTrue())
		})

		It("writes every change in mode off", func() {
			Expect(guard(changeset.ModeOff).CheckWrite(ctx, "route", "r", body{Name: ptr("a")}, body{}, true)).To(BeTrue())
		})
	})

	DescribeTable("ignores the diffs a rule covers",
		func(r changeset.Rule, kind, name string, desired, current body, ignored bool) {
			g := guard(changeset.ModeOn, r)
			Expect(g.CheckWrite(ctx, kind, name, desired, current, true)).To(BeFalse())
			Expect(plan.Changes()[0].Diffs[0].Ignored).To(Equal(ignored))
		},
		Entry("path below the rule path", changeset.Rule{Kind: "plugin", Paths: []string{"$.config"}},
			"plugin", "acl", body{Config: map[string]any{"allow": []any{"a"}}}, body{}, true),
		Entry("key wildcard", changeset.Rule{Kind: "plugin", Paths: []string{"$.*.limit"}},
			"plugin", "rate-limiting", body{Config: map[string]any{"limit": 1}}, body{Config: map[string]any{"limit": 2}}, true),
		Entry("index wildcard", changeset.Rule{Kind: "route", Paths: []string{"$.paths[*]"}},
			"route", "r", body{Paths: []string{"/a"}}, body{Paths: []string{"/b"}}, true),
		Entry("matching name", changeset.Rule{Kind: "route", Name: "bond--.*", Paths: []string{"$.name"}},
			"route", "bond--echo", body{Name: ptr("a")}, body{Name: ptr("b")}, true),
		Entry("other name", changeset.Rule{Kind: "route", Name: "bond--.*", Paths: []string{"$.name"}},
			"route", "cit2--echo", body{Name: ptr("a")}, body{Name: ptr("b")}, false),
		Entry("partial name match", changeset.Rule{Kind: "route", Name: "bond", Paths: []string{"$.name"}},
			"route", "bond--echo", body{Name: ptr("a")}, body{Name: ptr("b")}, false),
		Entry("other kind", changeset.Rule{Kind: "service", Paths: []string{"$.name"}},
			"route", "r", body{Name: ptr("a")}, body{Name: ptr("b")}, false),
		Entry("sibling path", changeset.Rule{Kind: "route", Paths: []string{"$.tags"}},
			"route", "r", body{Paths: []string{"/a"}}, body{}, false),
	)

	DescribeTable("rejects an invalid config",
		func(r changeset.Rule, message string) {
			_, err := changeset.New(changeset.Options{Mode: changeset.ModeOn, Kinds: kinds}, changeset.Config{Ignore: []changeset.Rule{r}})
			Expect(err).To(MatchError(ContainSubstring(message)))
		},
		Entry("unknown kind", changeset.Rule{Kind: "routes", Paths: []string{"$"}}, `unknown kind "routes"`),
		Entry("no paths", changeset.Rule{Kind: "route"}, "paths must not be empty"),
		Entry("invalid name", changeset.Rule{Kind: "route", Name: "(", Paths: []string{"$"}}, "invalid name pattern"),
		Entry("path without root", changeset.Rule{Kind: "route", Paths: []string{"tags"}}, `must start with "$"`),
		Entry("empty key", changeset.Rule{Kind: "route", Paths: []string{"$..tags"}}, "empty key"),
		Entry("unclosed index", changeset.Rule{Kind: "route", Paths: []string{"$.paths[0"}}, "unclosed index"),
		Entry("invalid index", changeset.Rule{Kind: "route", Paths: []string{"$.paths[x]"}}, `invalid index "x"`),
	)

	It("rejects an unknown mode", func() {
		_, err := changeset.New(changeset.Options{Mode: "maybe", Kinds: kinds}, changeset.Config{})
		Expect(err).To(MatchError(ContainSubstring(`invalid migration mode "maybe"`)))
	})
})

var _ = Describe("Planned entities", func() {
	It("registers a planned ID that is found by name and by ID", func() {
		ctx := changeset.WithPlan(context.Background(), changeset.NewPlan())
		id := changeset.PlannedID(ctx, "route", "bond--echo")
		Expect(changeset.IsPlanned(id)).To(BeTrue())
		for _, ref := range []string{"bond--echo", id} {
			found, ok := changeset.LookupPlanned(ctx, "route", ref)
			Expect(ok).To(BeTrue())
			Expect(found).To(Equal(id))
		}
		_, ok := changeset.LookupPlanned(ctx, "service", "bond--echo")
		Expect(ok).To(BeFalse())
	})

	It("plans nothing without a plan", func() {
		Expect(changeset.PlannedID(context.Background(), "route", "r")).To(BeEmpty())
		Expect(changeset.IsPlanned("4f1c")).To(BeFalse())
	})
})

var _ = Describe("Load", func() {
	write := func(content string) string {
		path := filepath.Join(GinkgoT().TempDir(), "ignore.yaml")
		Expect(os.WriteFile(path, []byte(content), 0o600)).To(Succeed())
		return path
	}

	It("loads the ignore list", func() {
		g, err := changeset.Load(changeset.Options{Mode: changeset.ModeOn, Kinds: kinds}, write("ignore:\n  - kind: route\n    paths: [\"$.tags\"]\n"))
		Expect(err).NotTo(HaveOccurred())
		Expect(g.CheckWrite(context.Background(), "route", "r", body{Tags: []string{"a"}}, body{}, true)).To(BeTrue())
	})

	It("blocks every diff without a file", func() {
		g, err := changeset.Load(changeset.Options{Mode: changeset.ModeOn, Kinds: kinds}, "")
		Expect(err).NotTo(HaveOccurred())
		_, err = g.CheckDelete(context.Background(), "route", "r")
		Expect(isBlocked(err)).To(BeTrue())
	})

	It("gives a gate that records but allows everything in mode off", func() {
		g, err := changeset.Load(changeset.Options{Mode: changeset.ModeOff, Kinds: kinds}, "")
		Expect(err).NotTo(HaveOccurred())
		Expect(g.Mode()).To(Equal(changeset.ModeOff))
	})

	DescribeTable("rejects an invalid file",
		func(content, message string) {
			_, err := changeset.Load(changeset.Options{Mode: changeset.ModeOn, Kinds: kinds}, write(content))
			Expect(err).To(MatchError(ContainSubstring(message)))
		},
		Entry("unknown field", "ignored: []\n", "parsing migration config"),
		Entry("invalid rule", "ignore:\n  - kind: route\n", "paths must not be empty"),
	)
})

var _ = Describe("ParseMode", func() {
	DescribeTable("parses the flag value",
		func(value string, expected changeset.Mode) {
			Expect(changeset.ParseMode(value)).To(Equal(expected))
		},
		Entry("off", "off", changeset.ModeOff),
		Entry("on", "on", changeset.ModeOn),
		Entry("strict", "strict", changeset.ModeStrict),
	)

	It("rejects an unknown value", func() {
		_, err := changeset.ParseMode("true")
		Expect(err).To(MatchError(ContainSubstring(`invalid migration mode "true"`)))
	})
})

var _ = Describe("IsApproved", func() {
	DescribeTable("approves only the exact hash",
		func(annotation, hash string, expected bool) {
			meta := &metav1.ObjectMeta{Annotations: map[string]string{changeset.ApprovalAnnotation: annotation}}
			Expect(changeset.IsApproved(meta, hash)).To(Equal(expected))
		},
		Entry("same hash", "0123456789abcdef", "0123456789abcdef", true),
		Entry("other hash", "0123456789abcdef", "fedcba9876543210", false),
		Entry("empty hash", "", "", false),
	)
})
