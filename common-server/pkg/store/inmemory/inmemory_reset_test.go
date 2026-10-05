// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package inmemory

import (
	"context"
	"os"
	"path/filepath"

	"github.com/dgraph-io/badger/v4"
	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// capturePanic runs fn and returns the recovered panic value, or nil.
func capturePanic(fn func()) (recovered any) {
	defer func() { recovered = recover() }()
	fn()
	return nil
}

// closeIfOpened registers a checked close for a DB that unexpectedly opened.
func closeIfOpened(db *badger.DB) {
	DeferCleanup(func() { Expect(db.Close()).To(Succeed()) })
}

var _ = Describe("Store startup reset", func() {
	ctx := context.Background()

	It("starts empty after a previous store left data on disk", func() {
		root := GinkgoT().TempDir()

		s := openDiskStore(root)
		Expect(s.OnCreate(ctx, thresholdObject("old", "a", 0))).To(Succeed())
		_, err := s.Get(ctx, "default", "old")
		Expect(err).ToNot(HaveOccurred())
		closeStore(s)

		s = openDiskStore(root)
		_, err = s.Get(ctx, "default", "old")
		Expect(err).To(HaveOccurred())
		Expect(listOrders(ctx, s)).To(BeEmpty())
	})

	It("removes only the per-store directory, leaving the root and siblings untouched", func() {
		root := GinkgoT().TempDir()
		siblingDir := filepath.Join(root, "db-other-v1-things")
		Expect(os.Mkdir(siblingDir, 0o755)).To(Succeed())
		siblingFile := filepath.Join(siblingDir, "keep")
		Expect(os.WriteFile(siblingFile, []byte("keep"), 0o600)).To(Succeed())
		rootFile := filepath.Join(root, "keep")
		Expect(os.WriteFile(rootFile, []byte("keep"), 0o600)).To(Succeed())

		storeDir := filepath.Join(root, thresholdDbName)
		Expect(os.Mkdir(storeDir, 0o755)).To(Succeed())
		staleFile := filepath.Join(storeDir, "stale")
		Expect(os.WriteFile(staleFile, []byte("stale"), 0o600)).To(Succeed())

		openDiskStore(root)

		Expect(staleFile).ToNot(BeAnExistingFile())
		Expect(storeDir).To(BeADirectory())
		Expect(siblingFile).To(BeAnExistingFile())
		Expect(rootFile).To(BeAnExistingFile())
	})

	It("removes a symlinked store path without touching its target", func() {
		root := GinkgoT().TempDir()
		target := GinkgoT().TempDir()
		targetFile := filepath.Join(target, "keep")
		Expect(os.WriteFile(targetFile, []byte("keep"), 0o600)).To(Succeed())
		Expect(os.Symlink(target, filepath.Join(root, thresholdDbName))).To(Succeed())

		openDiskStore(root)

		Expect(targetFile).To(BeAnExistingFile())
	})

	It("starts empty in memory without touching the filesystem", func() {
		cwd, err := os.Getwd()
		Expect(err).ToNot(HaveOccurred())
		s := newThresholdStore("")
		DeferCleanup(closeStore, s)

		Expect(s.db.Opts().InMemory).To(BeTrue())
		Expect(s.db.Opts().Dir).To(BeEmpty())
		Expect(listOrders(ctx, s)).To(BeEmpty())
		Expect(filepath.Join(cwd, thresholdDbName)).ToNot(BeAnExistingFile())
	})

	It("panics with context when the store directory cannot be removed", func() {
		root := GinkgoT().TempDir()
		// A regular file as parent component makes RemoveAll fail with ENOTDIR.
		fileRoot := filepath.Join(root, "not-a-dir")
		Expect(os.WriteFile(fileRoot, []byte("x"), 0o600)).To(Succeed())

		r := capturePanic(func() {
			closeIfOpened(newDbOrDie(thresholdStoreOpts(fileRoot), logr.Discard()))
		})

		err, ok := r.(error)
		Expect(ok).To(BeTrue(), "expected panic with error, got %v", r)
		Expect(err).To(MatchError(ContainSubstring("failed to reset badger DB directory")))
		Expect(err).To(MatchError(ContainSubstring(filepath.Join(fileRoot, thresholdDbName))))
		Expect(fileRoot).To(BeAnExistingFile())
	})

	DescribeTable("rejects GVRs that would escape the root",
		func(gvr schema.GroupVersionResource) {
			root := GinkgoT().TempDir()
			opts := thresholdStoreOpts(root)
			opts.GVR = gvr

			r := capturePanic(func() {
				closeIfOpened(newDbOrDie(opts, logr.Discard()))
			})

			err, ok := r.(error)
			Expect(ok).To(BeTrue(), "expected panic with error, got %v", r)
			Expect(err).To(MatchError(ContainSubstring("invalid badger DB directory name")))
			Expect(root).To(BeADirectory())
		},
		Entry("slash in resource", schema.GroupVersionResource{Group: "g", Version: "v1", Resource: "../../x"}),
		Entry("slash in group", schema.GroupVersionResource{Group: "a/b", Version: "v1", Resource: "x"}),
	)

	DescribeTable("isSafeDbName",
		func(name string, safe bool) {
			Expect(isSafeDbName(name)).To(Equal(safe))
		},
		Entry("normal name", "db-test.example-v1-things", true),
		Entry("core group", "db--v1-pods", true),
		Entry("parent", "..", false),
		Entry("slash", "db-a/b", false),
		Entry("absolute", "/db", false),
		Entry("empty", "", false),
	)
})
