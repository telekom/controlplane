// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package orchestrator_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/telekom/controlplane/tools/snapshotter/pkg/decoder"
	"github.com/telekom/controlplane/tools/snapshotter/pkg/diffmatcher"
	"github.com/telekom/controlplane/tools/snapshotter/pkg/obfuscator"
	"github.com/telekom/controlplane/tools/snapshotter/pkg/orchestrator"
	"github.com/telekom/controlplane/tools/snapshotter/pkg/snapshot"
	"github.com/telekom/controlplane/tools/snapshotter/pkg/store"
)

const routeStateJSON = `{
  "route": {"id": "11111111-2222-3333-4444-555555555555", "name": "my-route", "paths": ["/b", "/a"]},
  "service": {"id": "66666666-2222-3333-4444-555555555555", "name": "my-route"},
  "plugins": [{"id": "77777777-2222-3333-4444-555555555555", "name": "jwt", "config": {"data": "b64:aGVsbG8=", "list": ["z", "a"]}}]
}`

type fakeSource struct {
	stateJSON string
	globalErr error
}

func (f *fakeSource) TakeSnapshot(_ context.Context, _, resourceId string) (*snapshot.Snapshot, error) {
	state := &snapshot.State{}
	Expect(json.Unmarshal([]byte(f.stateJSON), state)).To(Succeed())
	return &snapshot.Snapshot{Id: resourceId, State: state}, nil
}

func (f *fakeSource) TakeGlobalSnapshot(context.Context, string, int, chan<- *snapshot.Snapshot) error {
	return f.globalErr
}

var _ = Describe("Orchestrator raw mode", func() {
	var (
		dir    string
		fs     *store.FileStore[*snapshot.Snapshot]
		src    *fakeSource
		newOrc func(raw bool) *orchestrator.Orchestrator
	)

	BeforeEach(func() {
		dir = GinkgoT().TempDir()
		fs = store.NewFileStore[*snapshot.Snapshot](dir)
		src = &fakeSource{stateJSON: routeStateJSON}
		newOrc = func(raw bool) *orchestrator.Orchestrator {
			o := orchestrator.NewOrchestrator(src, fs)
			o.Environment, o.Zone = "prod", "aws"
			o.Raw = raw
			o.SetObfuscationTargets(obfuscator.ObfuscationTarget{
				Pattern: "[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}",
				Replace: "00000000-0000-0000-0000-000000000000",
			})
			o.SetDecoderTargets(decoder.DecoderTarget{Type: "base64", Pattern: "b64:([A-Za-z0-9=]+)"})
			return o
		}
	})

	stored := func() *snapshot.Snapshot {
		snap := &snapshot.Snapshot{}
		Expect(fs.GetLatest(context.Background(), "prod-aws-my-route", snap)).To(Succeed())
		return snap
	}

	It("skips obfuscators and decoders and stores unsorted data", func() {
		snap, err := newOrc(true).Do(context.Background(), "route", "my-route")
		Expect(err).NotTo(HaveOccurred())
		Expect(snap.Raw).To(BeTrue())
		Expect(*snap.State.Route.Paths).To(Equal([]string{"/b", "/a"}), "returned snapshot must not be sorted")

		s := stored()
		Expect(s.Raw).To(BeTrue())
		Expect(*s.State.Route.Id).To(Equal("11111111-2222-3333-4444-555555555555"))
		Expect(*s.State.Route.Paths).To(Equal([]string{"/b", "/a"}))
		Expect((*s.State.Plugins[0].Config)["data"]).To(Equal("b64:aGVsbG8="))
		Expect((*s.State.Plugins[0].Config)["list"]).To(Equal([]any{"z", "a"}))

		info, err := os.Stat(filepath.Join(dir, "prod-aws-my-route", "1.snap.yaml"))
		Expect(err).NotTo(HaveOccurred())
		Expect(info.Mode().Perm()).To(Equal(os.FileMode(0o600)))
	})

	It("still obfuscates and decodes in normal mode", func() {
		_, err := newOrc(false).Do(context.Background(), "route", "my-route")
		Expect(err).NotTo(HaveOccurred())
		s := stored()
		Expect(s.Raw).To(BeFalse())
		Expect(*s.State.Route.Id).To(Equal("00000000-0000-0000-0000-000000000000"))
		Expect((*s.State.Plugins[0].Config)["data"]).To(Equal("b64:hello"))
	})

	It("re-stores identical content when the previous snapshot was not raw", func() {
		src.stateJSON = `{"route": {"id": "r", "name": "my-route", "paths": ["/a"]}}`
		_, err := newOrc(false).Do(context.Background(), "route", "my-route")
		Expect(err).NotTo(HaveOccurred())
		Expect(stored().Raw).To(BeFalse())

		var result diffmatcher.Result
		o := newOrc(true)
		o.ReportResult = func(r diffmatcher.Result, _ string) { result = r }
		_, err = o.Do(context.Background(), "route", "my-route")
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Changed).To(BeFalse())
		Expect(stored().Raw).To(BeTrue())
	})

	It("does not re-store an unchanged raw snapshot", func() {
		_, err := newOrc(true).Do(context.Background(), "route", "my-route")
		Expect(err).NotTo(HaveOccurred())
		_, err = newOrc(true).Do(context.Background(), "route", "my-route")
		Expect(err).NotTo(HaveOccurred())
		_, err = os.Stat(filepath.Join(dir, "prod-aws-my-route", "1.snap.yaml"))
		Expect(err).NotTo(HaveOccurred(), "version 1 must still be the latest")
	})
})

var _ = Describe("Orchestrator Run", func() {
	It("returns the error from TakeGlobalSnapshot", func() {
		src := &fakeSource{globalErr: errors.New("listing failed")}
		o := orchestrator.NewOrchestrator(src, store.NewFileStore[*snapshot.Snapshot](GinkgoT().TempDir()))
		snaps, err := o.Run(context.Background(), orchestrator.RunOptions{ResourceType: "route", Limit: 10})
		Expect(err).To(MatchError("listing failed"))
		Expect(snaps).To(BeEmpty())
	})
})
