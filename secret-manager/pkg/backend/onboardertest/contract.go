// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

// Package onboardertest holds a backend-agnostic conformance suite for the
// backend.Onboarder contract. Both the Kubernetes and Conjur onboarders are
// expected to behave identically for the three observable surfaces:
//
//   - the OnboardResponse (which SecretRefs a call returns),
//   - what gets stored, and
//   - what Backend.Get returns when the resulting ref is retrieved.
//
// "Stored" and "retrieved" are the same observable here: the suite never peeks
// at backend-specific storage, it always reads back through Get (see Harness).
//
// Each backend provides a Harness and calls RunContractSpecs from its own
// Ginkgo suite. Anything that is genuinely backend-specific (Conjur policy
// YAML / bouncer, Kubernetes finalizers / stray-key handling) stays in that
// backend's own *_test.go files.
package onboardertest

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/telekom/controlplane/secret-manager/pkg/backend"
)

// Fixed identifiers used across all shared scenarios so ref formats
// (env:team:app:path:checksum) are stable and comparable between backends.
const (
	Env  = "test-env"
	Team = "test-team"
	App  = "test-app"
)

// Harness adapts one backend to the shared contract specs.
//
// The onboarder returned by Onboarder() and the read path used by Get() MUST
// share the same underlying store, so that Get observes exactly what onboarding
// wrote.
type Harness interface {
	// Onboarder returns the onboarder under test. It is expected to be safe for
	// concurrent use and to return the same instance across calls.
	Onboarder() backend.Onboarder
	// Get resolves a ref exactly as returned by OnboardResponse.SecretRefs() and
	// yields the retrieved value. It returns an error if the secret does not
	// exist (or cannot be resolved).
	Get(ctx context.Context, ref string) (value string, err error)
}

// Factory builds a fresh Harness (fresh store) for a single spec.
type Factory func() Harness

type valueKind int

const (
	// nonEmpty asserts the retrieved value is present and not empty (used for
	// generated secrets whose exact value is not predictable).
	nonEmpty valueKind = iota
	// exact asserts the retrieved value equals a fixed string.
	exact
)

type expect struct {
	kind valueKind
	val  string
}

type scenario struct {
	name string
	do   func(ctx context.Context, ob backend.Onboarder) (backend.OnboardResponse, error)
	// wantErr, when non-empty, asserts the call fails with an error message
	// containing this substring (and skips all response/value checks).
	wantErr string
	// wantRefs is the exact expected set of SecretRefs() keys.
	wantRefs []string
	// want maps a ref key to the value expected when that ref is retrieved.
	want map[string]expect
}

func scenarios() []scenario {
	return []scenario{
		{
			name: "team / no options",
			do: func(ctx context.Context, ob backend.Onboarder) (backend.OnboardResponse, error) {
				return ob.OnboardTeam(ctx, Env, Team)
			},
			wantRefs: []string{"clientSecret", "teamToken"},
			want: map[string]expect{
				"clientSecret": {nonEmpty, ""},
				"teamToken":    {nonEmpty, ""},
			},
		},
		{
			name: "team / user-provided teamToken",
			do: func(ctx context.Context, ob backend.Onboarder) (backend.OnboardResponse, error) {
				return ob.OnboardTeam(ctx, Env, Team,
					backend.WithSecretValue("teamToken", backend.String("myteamtokenvalue")))
			},
			wantRefs: []string{"clientSecret", "teamToken"},
			want: map[string]expect{
				"clientSecret": {nonEmpty, ""},
				"teamToken":    {exact, "myteamtokenvalue"},
			},
		},
		{
			// Note: with only two always-present secrets, replace and merge are
			// indistinguishable here — this pins that an explicitly provided
			// value is stored while the other allowed secret is still generated.
			// The replace-vs-merge difference (dropping old keys) is exercised by
			// the multi-step "replace drops sub-keys" spec below.
			name: "team / explicit clientSecret is stored (replace)",
			do: func(ctx context.Context, ob backend.Onboarder) (backend.OnboardResponse, error) {
				return ob.OnboardTeam(ctx, Env, Team,
					backend.WithSecretValue("clientSecret", backend.String("my-secret")),
					backend.WithStrategy(backend.StrategyReplace))
			},
			wantRefs: []string{"clientSecret", "teamToken"},
			want: map[string]expect{
				"clientSecret": {exact, "my-secret"},
				"teamToken":    {nonEmpty, ""},
			},
		},
		{
			name: "env / no options (empty zones default yields no refs)",
			do: func(ctx context.Context, ob backend.Onboarder) (backend.OnboardResponse, error) {
				return ob.OnboardEnvironment(ctx, Env)
			},
			// zones defaults to "{}" (empty): like every empty default it is
			// neither stored nor advertised as a ref, so onboarding an
			// environment with no options returns no refs at all.
			wantRefs: []string{},
			want:     map[string]expect{},
		},
		{
			name: "app / no options",
			do: func(ctx context.Context, ob backend.Onboarder) (backend.OnboardResponse, error) {
				return ob.OnboardApplication(ctx, Env, Team, App)
			},
			// externalSecrets defaults to "{}" (empty): it is neither stored nor
			// advertised as a ref, so only the populated secrets appear.
			// rotatedClientSecret defaults to the literal placeholder "NOT_USED".
			wantRefs: []string{"clientSecret", "rotatedClientSecret"},
			want: map[string]expect{
				"clientSecret":        {nonEmpty, ""},
				"rotatedClientSecret": {exact, "NOT_USED"},
			},
		},
		{
			// Fresh store: merge and replace are indistinguishable here, so this
			// only pins that provided sub-secrets are stored and advertised as
			// refs. The merge-vs-replace difference (preserving vs dropping
			// previously-stored sub-keys) is exercised by the multi-step
			// "accumulates..." and "replace drops sub-keys" specs below.
			name: "app / additional sub-secrets stored",
			do: func(ctx context.Context, ob backend.Onboarder) (backend.OnboardResponse, error) {
				return ob.OnboardApplication(ctx, Env, Team, App,
					backend.WithStrategy(backend.StrategyMerge),
					backend.WithSecretValue("externalSecrets/key1", backend.String("value1")),
					backend.WithSecretValue("externalSecrets/key2", backend.String("value2")))
			},
			wantRefs: []string{
				"clientSecret", "rotatedClientSecret", "externalSecrets",
				"externalSecrets/key1", "externalSecrets/key2",
			},
			want: map[string]expect{
				"rotatedClientSecret":  {exact, "NOT_USED"},
				"externalSecrets/key1": {exact, "value1"},
				"externalSecrets/key2": {exact, "value2"},
			},
		},
		{
			name: "app / unknown secret is forbidden",
			do: func(ctx context.Context, ob backend.Onboarder) (backend.OnboardResponse, error) {
				return ob.OnboardApplication(ctx, Env, Team, App,
					backend.WithSecretValue("extraSecret1", backend.String("topsecret")))
			},
			wantErr: "Forbidden: secret extraSecret1 is not allowed",
		},
		{
			// Fresh store: merge and replace are indistinguishable here (nothing
			// pre-exists to preserve or drop). Pre-existing merge is covered by
			// the multi-step concurrent-zones spec below.
			name: "env / zones sub-secret stored",
			do: func(ctx context.Context, ob backend.Onboarder) (backend.OnboardResponse, error) {
				return ob.OnboardEnvironment(ctx, Env,
					backend.WithStrategy(backend.StrategyMerge),
					backend.WithSecretValue("zones/dataplane1/clientSecret", backend.String("dp1-secret")))
			},
			wantRefs: []string{"zones", "zones/dataplane1/clientSecret"},
			want: map[string]expect{
				"zones/dataplane1/clientSecret": {exact, "dp1-secret"},
			},
		},
	}
}

// RunContractSpecs registers the shared behavior specs against a backend,
// building a fresh Harness (fresh store) for each spec.
func RunContractSpecs(newHarness Factory) {
	ctx := context.Background()

	Context("Contract (input -> response -> retrieved)", func() {
		for _, sc := range scenarios() {
			sc := sc
			It(sc.name, func() {
				h := newHarness()
				res, err := sc.do(ctx, h.Onboarder())

				if sc.wantErr != "" {
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring(sc.wantErr))
					return
				}

				Expect(err).ToNot(HaveOccurred())
				Expect(res).ToNot(BeNil())

				refs := res.SecretRefs()
				gotKeys := slices.Collect(maps.Keys(refs))
				Expect(gotKeys).To(ConsistOf(sc.wantRefs), "response ref keys")

				for key, want := range sc.want {
					ref, ok := refs[key]
					Expect(ok).To(BeTrue(), "expected ref %q in response", key)

					got, err := h.Get(ctx, ref.String())
					Expect(err).ToNot(HaveOccurred(), "retrieving ref %q", key)
					switch want.kind {
					case nonEmpty:
						Expect(got).ToNot(BeEmpty(), "retrieved value for %q", key)
					case exact:
						Expect(got).To(Equal(want.val), "retrieved value for %q", key)
					}
				}
			})
		}
	})

	Context("Contract (multi-step)", func() {
		It("keeps clientSecret immutable across re-onboard, regardless of strategy", func() {
			h := newHarness()
			ob := h.Onboarder()

			res1, err := ob.OnboardTeam(ctx, Env, Team)
			Expect(err).ToNot(HaveOccurred())
			first, err := h.Get(ctx, res1.SecretRefs()["clientSecret"].String())
			Expect(err).ToNot(HaveOccurred())
			Expect(first).ToNot(BeEmpty())

			// Immutability, not strategy, is what preserves clientSecret here: a
			// generated initial value is never regenerated, so it survives both
			// merge and replace unchanged. (An explicitly provided clientSecret
			// is a deliberate override and is a separate concern, not covered
			// here.) We run both strategies only to prove neither breaks it.
			res2, err := ob.OnboardTeam(ctx, Env, Team, backend.WithStrategy(backend.StrategyMerge))
			Expect(err).ToNot(HaveOccurred())
			second, err := h.Get(ctx, res2.SecretRefs()["clientSecret"].String())
			Expect(err).ToNot(HaveOccurred())
			Expect(second).To(Equal(first))

			// Replace starts from an empty set but still preserves immutable
			// initial values that already exist, so clientSecret survives.
			res3, err := ob.OnboardTeam(ctx, Env, Team, backend.WithStrategy(backend.StrategyReplace))
			Expect(err).ToNot(HaveOccurred())
			third, err := h.Get(ctx, res3.SecretRefs()["clientSecret"].String())
			Expect(err).ToNot(HaveOccurred())
			Expect(third).To(Equal(first), "clientSecret must survive replace unchanged")
		})

		It("replace drops sub-keys that merge would keep", func() {
			h := newHarness()
			ob := h.Onboarder()

			// Seed externalSecrets with key1.
			_, err := ob.OnboardApplication(ctx, Env, Team, App,
				backend.WithStrategy(backend.StrategyMerge),
				backend.WithSecretValue("externalSecrets/key1", backend.String("value1")))
			Expect(err).ToNot(HaveOccurred())

			// Re-onboard with REPLACE and only key2: key1 must be gone.
			res, err := ob.OnboardApplication(ctx, Env, Team, App,
				backend.WithStrategy(backend.StrategyReplace),
				backend.WithSecretValue("externalSecrets/key2", backend.String("value2")))
			Expect(err).ToNot(HaveOccurred())

			blob, err := h.Get(ctx, res.SecretRefs()["externalSecrets"].String())
			Expect(err).ToNot(HaveOccurred())
			var m map[string]string
			Expect(json.Unmarshal([]byte(blob), &m)).To(Succeed())
			Expect(m).To(HaveKeyWithValue("key2", "value2"))
			Expect(m).ToNot(HaveKey("key1"), "replace must not preserve previously-set sub-keys")
		})

		It("accumulates externalSecrets sub-keys across onboards (merge)", func() {
			h := newHarness()
			ob := h.Onboarder()

			res1, err := ob.OnboardApplication(ctx, Env, Team, App,
				backend.WithStrategy(backend.StrategyMerge),
				backend.WithSecretValue("externalSecrets/key1", backend.String("value1")))
			Expect(err).ToNot(HaveOccurred())

			// key1 must actually be persisted before the second merge, otherwise
			// this could pass against a backend that only ever keeps the last
			// write. This pins that step 2 truly merges into stored state.
			seed, err := h.Get(ctx, res1.SecretRefs()["externalSecrets"].String())
			Expect(err).ToNot(HaveOccurred())
			var seedMap map[string]string
			Expect(json.Unmarshal([]byte(seed), &seedMap)).To(Succeed())
			Expect(seedMap).To(HaveKeyWithValue("key1", "value1"))

			res, err := ob.OnboardApplication(ctx, Env, Team, App,
				backend.WithStrategy(backend.StrategyMerge),
				backend.WithSecretValue("externalSecrets/key2", backend.String("value2")))
			Expect(err).ToNot(HaveOccurred())

			blob, err := h.Get(ctx, res.SecretRefs()["externalSecrets"].String())
			Expect(err).ToNot(HaveOccurred())
			var m map[string]string
			Expect(json.Unmarshal([]byte(blob), &m)).To(Succeed())
			Expect(m).To(HaveKeyWithValue("key1", "value1"))
			Expect(m).To(HaveKeyWithValue("key2", "value2"))
		})

		It("preserves all zones sub-keys under concurrent env onboard (merge)", func() {
			const concurrency = 10
			h := newHarness()
			ob := h.Onboarder()

			var wg sync.WaitGroup
			errs := make(chan error, concurrency)
			refs := make(chan backend.SecretRef, concurrency)
			wg.Add(concurrency)

			for i := 0; i < concurrency; i++ {
				go func(idx int) {
					defer wg.Done()
					defer GinkgoRecover()
					// Concurrent onboards may hit transient write conflicts: the
					// Kubernetes backend uses bounded optimistic-locking retries
					// that can be exhausted, and the Conjur backend may fail to
					// acquire its lock. Both are retryable, and a real caller
					// (a reconciler) requeues, so we retry until it succeeds.
					var res backend.OnboardResponse
					var err error
					for attempt := 0; attempt < 50; attempt++ {
						res, err = ob.OnboardEnvironment(ctx, Env,
							backend.WithStrategy(backend.StrategyMerge),
							backend.WithSecretValue(fmt.Sprintf("zones/foo%d", idx), backend.String(fmt.Sprintf("bar%d", idx))))
						if err == nil {
							break
						}
						time.Sleep(5 * time.Millisecond)
					}
					if err == nil {
						refs <- res.SecretRefs()["zones"]
					}
					errs <- err
				}(i)
			}

			wg.Wait()
			close(errs)
			close(refs)

			for err := range errs {
				Expect(err).ToNot(HaveOccurred())
			}

			ref := <-refs
			// Any goroutine's "zones" ref resolves to the same current value:
			// Get looks up the stored secret by its path, not by the checksum
			// captured in the ref, so all refs point at the final merged blob.
			blob, err := h.Get(ctx, ref.String())
			Expect(err).ToNot(HaveOccurred())
			var m map[string]string
			Expect(json.Unmarshal([]byte(blob), &m)).To(Succeed())
			Expect(m).To(HaveLen(concurrency))
			for i := 0; i < concurrency; i++ {
				Expect(m).To(HaveKeyWithValue(fmt.Sprintf("foo%d", i), fmt.Sprintf("bar%d", i)))
			}
		})

		It("removes the scope on delete: retrieval returns an error", func() {
			h := newHarness()
			ob := h.Onboarder()

			res, err := ob.OnboardApplication(ctx, Env, Team, App)
			Expect(err).ToNot(HaveOccurred())
			ref := res.SecretRefs()["clientSecret"].String()

			// Value is retrievable before delete.
			_, err = h.Get(ctx, ref)
			Expect(err).ToNot(HaveOccurred())

			Expect(ob.DeleteApplication(ctx, Env, Team, App)).To(Succeed())

			_, err = h.Get(ctx, ref)
			Expect(err).To(HaveOccurred())
		})
	})
}
