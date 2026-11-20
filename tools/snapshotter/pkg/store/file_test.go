// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package store_test

import (
	"context"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/telekom/controlplane/tools/snapshotter/pkg/snapshot"
	"github.com/telekom/controlplane/tools/snapshotter/pkg/store"
)

var _ = Describe("FileStore file modes", func() {
	var dir string
	var fs *store.FileStore[*snapshot.Snapshot]

	BeforeEach(func() {
		dir = GinkgoT().TempDir()
		fs = store.NewFileStore[*snapshot.Snapshot](dir)
	})

	mode := func(path string) os.FileMode {
		info, err := os.Stat(path)
		Expect(err).NotTo(HaveOccurred())
		return info.Mode().Perm()
	}

	It("writes raw snapshots with 0600 in a 0700 directory", func() {
		Expect(fs.Set(context.Background(), &snapshot.Snapshot{Environment: "e", Zone: "z", Id: "r", Raw: true})).To(Succeed())
		Expect(mode(filepath.Join(dir, "e-z-r", "1.snap.yaml"))).To(Equal(os.FileMode(0o600)))
		Expect(mode(filepath.Join(dir, "e-z-r"))).To(Equal(os.FileMode(0o700)))
	})

	It("keeps 0644 for non-raw snapshots", func() {
		Expect(fs.Set(context.Background(), &snapshot.Snapshot{Environment: "e", Zone: "z", Id: "r"})).To(Succeed())
		Expect(mode(filepath.Join(dir, "e-z-r", "1.snap.yaml"))).To(Equal(os.FileMode(0o644) &^ umask()))
	})

	It("ignores the .backups directory when reading snapshots", func() {
		Expect(os.MkdirAll(filepath.Join(dir, ".backups", "e-z-r"), 0o700)).To(Succeed())
		err := fs.GetLatest(context.Background(), "e-z-r", &snapshot.Snapshot{})
		Expect(err).To(MatchError(store.ErrNotFound))
	})
})
