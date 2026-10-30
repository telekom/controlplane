// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package rollback_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	kong "github.com/telekom/controlplane/gateway/pkg/kong/api"
	"github.com/telekom/controlplane/tools/snapshotter/pkg/rollback"
	"github.com/telekom/controlplane/tools/snapshotter/pkg/rollback/rollbacktest"
	"github.com/telekom/controlplane/tools/snapshotter/pkg/snapshot"
)

// seed stores the entities of the snapshot in the fake gateway as-is.
func seed(fake *rollbacktest.FakeKong, snap *snapshot.Snapshot) {
	toMap := func(v any) map[string]any {
		data, err := json.Marshal(v)
		Expect(err).NotTo(HaveOccurred())
		m := map[string]any{}
		Expect(json.Unmarshal(data, &m)).To(Succeed())
		return m
	}
	s := snap.State
	fake.Services[*s.Service.Id] = toMap(s.Service)
	fake.Routes[*s.Route.Id] = toMap(s.Route)
	for _, p := range s.Plugins {
		fake.Plugins[*p.Id] = toMap(p)
	}
	if s.Upstream != nil {
		fake.Upstreams[*s.Upstream.Id] = toMap(s.Upstream)
		fake.Targets[*s.Upstream.Id] = map[string]map[string]any{}
		for _, t := range s.Targets {
			fake.Targets[*s.Upstream.Id][*t.Id] = toMap(t)
		}
	}
}

var _ = Describe("Validate", func() {
	It("accepts a complete raw snapshot", func() {
		Expect(rollback.Validate(desiredSnapshot(), "prod", "aws")).To(Succeed())
	})

	It("refuses non-raw snapshots", func() {
		snap := desiredSnapshot()
		snap.Raw = false
		Expect(rollback.Validate(snap, "prod", "aws")).To(MatchError(ContainSubstring("not raw")))
	})

	It("refuses environment or zone mismatches", func() {
		Expect(rollback.Validate(desiredSnapshot(), "qa", "aws")).To(MatchError(ContainSubstring(`environment "prod" zone "aws"`)))
		Expect(rollback.Validate(desiredSnapshot(), "prod", "azure")).To(MatchError(ContainSubstring("selected source")))
	})

	It("requires a route and a service with IDs", func() {
		snap := desiredSnapshot()
		snap.State.Route.Id = nil
		Expect(rollback.Validate(snap, "prod", "aws")).To(MatchError(ContainSubstring("route with an ID")))
		snap = desiredSnapshot()
		snap.State.Service = nil
		Expect(rollback.Validate(snap, "prod", "aws")).To(MatchError(ContainSubstring("service with an ID")))
		snap = desiredSnapshot()
		snap.State = nil
		Expect(rollback.Validate(snap, "prod", "aws")).To(HaveOccurred())
	})

	It("requires IDs on plugins, targets and upstream", func() {
		snap := desiredSnapshot()
		snap.State.Plugins[0].Id = nil
		Expect(rollback.Validate(snap, "prod", "aws")).To(MatchError(ContainSubstring("plugin")))
		snap = desiredSnapshot()
		snap.State.Targets[0].Id = nil
		Expect(rollback.Validate(snap, "prod", "aws")).To(MatchError(ContainSubstring("target")))
		snap = desiredSnapshot()
		snap.State.Upstream.Id = nil
		Expect(rollback.Validate(snap, "prod", "aws")).To(MatchError(ContainSubstring("upstream")))
	})
})

var _ = Describe("LoadFile", func() {
	It("reads a snapshot YAML file", func() {
		path := filepath.Join(GinkgoT().TempDir(), "1.snap.yaml")
		Expect(os.WriteFile(path, []byte("environment: prod\nzone: aws\nid: r\nraw: true\nstate: {}\n"), 0o600)).To(Succeed())
		snap, err := rollback.LoadFile(path)
		Expect(err).NotTo(HaveOccurred())
		Expect(snap.Raw).To(BeTrue())
		Expect(snap.ID()).To(Equal("prod-aws-r"))
	})

	It("fails for a missing file", func() {
		_, err := rollback.LoadFile(filepath.Join(GinkgoT().TempDir(), "missing.yaml"))
		Expect(err).To(HaveOccurred())
	})
})

var _ = Describe("Rollback against a fake Kong", func() {
	var (
		fake      *rollbacktest.FakeKong
		client    kong.ClientWithResponsesInterface
		ctx       context.Context
		storePath string
		rb        *rollback.Rollback
	)

	BeforeEach(func() {
		fake = rollbacktest.New()
		DeferCleanup(fake.Close)
		var err error
		client, err = kong.NewClientWithResponses(fake.Server.URL)
		Expect(err).NotTo(HaveOccurred())
		ctx = context.Background()
		storePath = GinkgoT().TempDir()
		rb = &rollback.Rollback{Client: client, Now: func() time.Time { return time.Unix(1234, 0) }}
	})

	Describe("ReadState", func() {
		It("treats a missing route as absent", func() {
			state, err := rollback.ReadState(ctx, client, desiredSnapshot().State)
			Expect(err).NotTo(HaveOccurred())
			Expect(state.Route).To(BeNil())
			Expect(state.Service).To(BeNil())
			Expect(state.Upstream).To(BeNil())
			Expect(state.Plugins).To(BeEmpty())
		})

		It("reads all pages of plugins and targets", func() {
			snap := desiredSnapshot()
			snap.State.Targets = append(snap.State.Targets, kong.Target{Id: ptr("target-2"), Target: ptr("10.0.0.2:80")})
			seed(fake, snap)
			state, err := rollback.ReadState(ctx, client, snap.State)
			Expect(err).NotTo(HaveOccurred())
			Expect(state.Plugins).To(HaveLen(2))
			Expect(state.Targets).To(HaveLen(2))
			Expect(*state.Route.Id).To(Equal("route-1"))
		})

		It("finds the upstream by route name when the snapshot has none", func() {
			snap := desiredSnapshot()
			seed(fake, snap)
			snap.State.Upstream, snap.State.Targets = nil, nil
			state, err := rollback.ReadState(ctx, client, snap.State)
			Expect(err).NotTo(HaveOccurred())
			Expect(*state.Upstream.Id).To(Equal("up-1"))
			Expect(state.Targets).To(HaveLen(1))
		})

		It("returns an error for unexpected statuses", func() {
			fake.Fail["GET /routes/route-1"] = 500
			_, err := rollback.ReadState(ctx, client, desiredSnapshot().State)
			var statusErr *rollback.StatusError
			Expect(errors.As(err, &statusErr)).To(BeTrue())
			Expect(statusErr.StatusCode).To(Equal(500))
		})
	})

	Describe("Apply", func() {
		It("creates the route from scratch in order with clean payloads", func() {
			desired := desiredSnapshot()
			prepared, err := rb.Prepare(ctx, desired)
			Expect(err).NotTo(HaveOccurred())
			Expect(prepared.Diff.Changed).To(BeTrue())

			backup, err := rb.Apply(ctx, prepared, storePath)
			Expect(err).NotTo(HaveOccurred())

			Expect(fake.WritePaths()).To(Equal([]string{
				"PUT /upstreams/up-1",
				"PUT /upstreams/up-1/targets/target-1",
				"PUT /services/svc-1",
				"PUT /routes/route-1",
				"PUT /plugins/plugin-2",
				"PUT /plugins/plugin-1",
			}))
			for _, w := range fake.Writes {
				Expect(w.Body).NotTo(HaveKey("created_at"))
				Expect(w.Body).NotTo(HaveKey("updated_at"))
				Expect(w.Body).To(HaveKey("id"))
			}
			Expect(fake.Writes[0].Body["id"]).To(Equal("up-1"))
			Expect(fake.Writes[3].Body["paths"]).To(Equal([]any{"/b", "/a"}), "order must not be changed")
			Expect(fake.Writes[4].Body["config"]).To(Equal(map[string]any{
				"minute": 10.0, "policy": nil, "unknown_field": map[string]any{"x": []any{3.0, 1.0}},
			}))
			Expect(fake.Writes[4].Body["route"]).To(Equal(map[string]any{"id": "route-1"}))

			Expect(backup).To(Equal(filepath.Join(storePath, ".backups", "prod-aws-my-route", "1234.snap.yaml")))
			info, err := os.Stat(backup)
			Expect(err).NotTo(HaveOccurred())
			Expect(info.Mode().Perm()).To(Equal(os.FileMode(0o600)))
			dirInfo, err := os.Stat(filepath.Dir(backup))
			Expect(err).NotTo(HaveOccurred())
			Expect(dirInfo.Mode().Perm()).To(Equal(os.FileMode(0o700)))

			saved, err := rollback.LoadFile(backup)
			Expect(err).NotTo(HaveOccurred())
			Expect(saved.Raw).To(BeTrue())
			Expect(saved.ID()).To(Equal("prod-aws-my-route"))
			Expect(saved.State.Route).To(BeNil())
		})

		It("is idempotent", func() {
			desired := desiredSnapshot()
			seed(fake, desired)
			prepared, err := rb.Prepare(ctx, desired)
			Expect(err).NotTo(HaveOccurred())
			Expect(prepared.Diff.Changed).To(BeFalse())
			Expect(prepared.HasWrites()).To(BeFalse())
		})

		It("restores a changed plugin and prunes extras", func() {
			desired := desiredSnapshot()
			seed(fake, desired)
			fake.Plugins["plugin-2"]["config"] = map[string]any{"minute": 99}
			fake.Plugins["plugin-x"] = map[string]any{"id": "plugin-x", "name": "cors", "route": map[string]any{"id": "route-1"}}
			fake.Targets["up-1"]["target-x"] = map[string]any{"id": "target-x", "target": "1.1.1.1:80"}
			rb.Prune = true

			prepared, err := rb.Prepare(ctx, desired)
			Expect(err).NotTo(HaveOccurred())
			_, err = rb.Apply(ctx, prepared, storePath)
			Expect(err).NotTo(HaveOccurred())
			Expect(fake.WritePaths()).To(Equal([]string{
				"PUT /plugins/plugin-2",
				"DELETE /plugins/plugin-x",
				"DELETE /upstreams/up-1/targets/target-x",
			}))
			Expect(fake.Plugins).NotTo(HaveKey("plugin-x"))
		})

		It("keeps extras without prune and still verifies", func() {
			desired := desiredSnapshot()
			seed(fake, desired)
			fake.Plugins["plugin-2"]["enabled"] = false
			fake.Plugins["plugin-x"] = map[string]any{"id": "plugin-x", "name": "cors", "route": map[string]any{"id": "route-1"}}

			prepared, err := rb.Prepare(ctx, desired)
			Expect(err).NotTo(HaveOccurred())
			_, err = rb.Apply(ctx, prepared, storePath)
			Expect(err).NotTo(HaveOccurred())
			Expect(fake.WritePaths()).To(Equal([]string{"PUT /plugins/plugin-2"}))
			Expect(fake.Plugins).To(HaveKey("plugin-x"))
		})

		It("stops at the first failing operation and reports succeeded ones", func() {
			fake.Fail["PUT /services/svc-1"] = 409
			prepared, err := rb.Prepare(ctx, desiredSnapshot())
			Expect(err).NotTo(HaveOccurred())
			backup, err := rb.Apply(ctx, prepared, storePath)
			Expect(backup).NotTo(BeEmpty())

			var execErr *rollback.ExecutionError
			Expect(errors.As(err, &execErr)).To(BeTrue())
			Expect(execErr.Failed.String()).To(Equal("upsert service svc-1"))
			Expect(execErr.Succeeded).To(HaveLen(2))
			Expect(err.Error()).To(ContainSubstring("409"))
			Expect(err.Error()).To(ContainSubstring("injected failure"))
			Expect(err.Error()).To(ContainSubstring("already succeeded: upsert upstream up-1; upsert target target-1"))
			Expect(fake.WritePaths()).To(HaveLen(3))
		})

		It("fails verification when the gateway does not match afterwards", func() {
			fake.OnWrite = func(path string, entity map[string]any) {
				if path == "/routes/route-1" {
					entity["paths"] = []any{"/other"}
				}
			}
			prepared, err := rb.Prepare(ctx, desiredSnapshot())
			Expect(err).NotTo(HaveOccurred())
			_, err = rb.Apply(ctx, prepared, storePath)
			Expect(err).To(MatchError(ContainSubstring("gateway state does not match the snapshot")))
		})

		It("requires a store path", func() {
			prepared, err := rb.Prepare(ctx, desiredSnapshot())
			Expect(err).NotTo(HaveOccurred())
			_, err = rb.Apply(ctx, prepared, "")
			Expect(err).To(MatchError(ContainSubstring("store path is required")))
			Expect(fake.Writes).To(BeEmpty())
		})
	})
})
