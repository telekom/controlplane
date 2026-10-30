// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package commands

import (
	"bytes"
	"context"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	kong "github.com/telekom/controlplane/gateway/pkg/kong/api"
	"github.com/telekom/controlplane/tools/snapshotter/pkg/config"
	"github.com/telekom/controlplane/tools/snapshotter/pkg/rollback"
	"github.com/telekom/controlplane/tools/snapshotter/pkg/rollback/rollbacktest"
	"github.com/telekom/controlplane/tools/snapshotter/pkg/snapshot"
	"github.com/telekom/controlplane/tools/snapshotter/pkg/store"
)

const applySnapshotYAML = `environment: prod
zone: aws
id: my-route
raw: true
state:
  service:
    id: svc-1
    name: my-route
    host: my-route
  route:
    id: route-1
    name: my-route
    service:
      id: svc-1
  plugins:
    - id: plugin-1
      name: acl
      route:
        id: route-1
      config:
        allow: [b, a]
`

var _ = Describe("apply command", func() {
	var (
		fake      *rollbacktest.FakeKong
		cfg       config.Config
		storeDir  string
		out       *bytes.Buffer
		newClient clientFactory
	)

	writeSnapshotFile := func(content string) string {
		path := filepath.Join(GinkgoT().TempDir(), "1.snap.yaml")
		Expect(os.WriteFile(path, []byte(content), 0o600)).To(Succeed())
		return path
	}

	BeforeEach(func() {
		outputFormat = OutputFormatText
		fake = rollbacktest.New()
		DeferCleanup(fake.Close)
		storeDir = GinkgoT().TempDir()
		cfg = config.Config{Sources: map[string]config.SourceConfig{
			"prod": {Environment: "prod", Zone: "aws"},
		}}
		out = &bytes.Buffer{}
		newClient = func(context.Context, config.SourceConfig) (kong.ClientWithResponsesInterface, func(), error) {
			c, err := kong.NewClientWithResponses(fake.Server.URL)
			return c, nil, err
		}
	})

	run := func(opts applyOptions) error {
		return runApply(context.Background(), cfg, opts, newClient, out)
	}

	It("requires exactly one of --route and --file", func() {
		Expect(run(applyOptions{})).To(MatchError("exactly one of --route or --file must be provided"))
		Expect(run(applyOptions{RouteId: "r", File: "f"})).To(MatchError("exactly one of --route or --file must be provided"))
	})

	It("requires --source when multiple sources are configured", func() {
		cfg.Sources["qa"] = config.SourceConfig{Environment: "qa", Zone: "aws"}
		Expect(run(applyOptions{File: "f"})).To(MatchError(ContainSubstring("no source key provided")))
		Expect(run(applyOptions{File: "f", SourceKey: "nope"})).To(MatchError(ContainSubstring("no source found")))
	})

	It("refuses non-raw snapshots", func() {
		path := writeSnapshotFile("environment: prod\nzone: aws\nid: my-route\nstate:\n  route:\n    id: r\n")
		Expect(run(applyOptions{File: path})).To(MatchError(ContainSubstring("not raw")))
		Expect(fake.Writes).To(BeEmpty())
	})

	It("refuses snapshots for another environment or zone", func() {
		cfg.Sources["prod"] = config.SourceConfig{Environment: "prod", Zone: "azure"}
		Expect(run(applyOptions{File: writeSnapshotFile(applySnapshotYAML), Apply: true, StorePath: storeDir})).
			To(MatchError(ContainSubstring("selected source")))
		Expect(fake.Writes).To(BeEmpty())
	})

	It("does a dry run by default", func() {
		Expect(run(applyOptions{File: writeSnapshotFile(applySnapshotYAML)})).To(Succeed())
		Expect(fake.Writes).To(BeEmpty())
		Expect(out.String()).To(ContainSubstring("Planned operations (dry run):"))
		Expect(out.String()).To(ContainSubstring("upsert service svc-1"))
		Expect(out.String()).To(ContainSubstring("upsert plugin acl plugin-1"))
	})

	It("requires a store path for --apply", func() {
		Expect(run(applyOptions{File: writeSnapshotFile(applySnapshotYAML), Apply: true})).
			To(MatchError(ContainSubstring("no store path configured")))
		Expect(fake.Writes).To(BeEmpty())
	})

	It("loads the latest snapshot by route from the store and applies it", func() {
		snap, err := rollback.LoadFile(writeSnapshotFile(applySnapshotYAML))
		Expect(err).NotTo(HaveOccurred())
		Expect(store.NewFileStore[*snapshot.Snapshot](storeDir).Set(context.Background(), snap)).To(Succeed())

		Expect(run(applyOptions{RouteId: "my-route", StorePath: storeDir, Apply: true})).To(Succeed())
		Expect(fake.WritePaths()).To(Equal([]string{"PUT /services/svc-1", "PUT /routes/route-1", "PUT /plugins/plugin-1"}))
		backups, err := filepath.Glob(filepath.Join(storeDir, ".backups", "prod-aws-my-route", "*.snap.yaml"))
		Expect(err).NotTo(HaveOccurred())
		Expect(backups).To(HaveLen(1))

		// second run is a no-op
		fake.Writes = nil
		Expect(run(applyOptions{RouteId: "my-route", StorePath: storeDir, Apply: true})).To(Succeed())
		Expect(fake.Writes).To(BeEmpty())
	})

	It("fails when the route snapshot is not in the store", func() {
		Expect(run(applyOptions{RouteId: "missing", StorePath: storeDir})).To(MatchError(ContainSubstring(`no snapshot "prod-aws-missing" found`)))
	})
})

var _ = Describe("validateSnapFlags", func() {
	It("allows --raw only with --route", func() {
		Expect(validateSnapFlags("r", true)).To(Succeed())
		Expect(validateSnapFlags("", false)).To(Succeed())
		Expect(validateSnapFlags("", true)).To(MatchError("--raw is only supported together with --route"))
	})
})
