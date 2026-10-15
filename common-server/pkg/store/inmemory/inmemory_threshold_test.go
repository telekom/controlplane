// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package inmemory

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/bytedance/sonic"
	"github.com/dgraph-io/badger/v4"
	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/telekom/controlplane/common-server/internal/informer"
	"github.com/telekom/controlplane/common-server/pkg/store"
)

var thresholdTestGVR = schema.GroupVersionResource{Group: "test.example", Version: "v1", Resource: "things"}

const thresholdDbName = "db-test.example-v1-things"

func thresholdObject(name, order string, payloadLen int) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "test.example/v1",
		"kind":       "Thing",
		"metadata":   map[string]any{"name": name, "namespace": "default"},
		"spec":       map[string]any{"order": order, "payload": strings.Repeat("a", payloadLen)},
	}}
}

// serializedSize mirrors the serialization done by OnUpdate.
func serializedSize(obj *unstructured.Unstructured) int {
	obj = obj.DeepCopy()
	informer.SanitizeObject(obj)
	data, err := sonic.Marshal(obj.Object)
	Expect(err).ToNot(HaveOccurred())
	return len(data)
}

// sizedObject returns an object whose stored representation is exactly size bytes.
func sizedObject(name, order string, size int) *unstructured.Unstructured {
	base := serializedSize(thresholdObject(name, order, 0))
	Expect(size).To(BeNumerically(">=", base))
	obj := thresholdObject(name, order, size-base)
	Expect(serializedSize(obj)).To(Equal(size))
	return obj
}

func newThresholdStore(dir string) *SortableStore[*unstructured.Unstructured] {
	opts := thresholdStoreOpts(dir)
	return wrapThresholdDB(newDbOrDie(opts, logr.Discard()), opts)
}

func thresholdStoreOpts(dir string) StoreOpts {
	return StoreOpts{
		GVR:          thresholdTestGVR,
		AllowedSorts: []string{"spec.order"},
		Database:     DatabaseOpts{Filepath: dir},
	}
}

func wrapThresholdDB(db *badger.DB, opts StoreOpts) *SortableStore[*unstructured.Unstructured] {
	ios := &InmemoryObjectStore[*unstructured.Unstructured]{
		ctx: context.Background(),
		log: logr.Discard(),
		db:  db,
	}
	return Sortable(ios, opts).(*SortableStore[*unstructured.Unstructured])
}

// rawReopenDiskStore reopens the existing on-disk database with the same
// Badger options, bypassing newDbOrDie and therefore its startup reset.
func rawReopenDiskStore(dir string) *SortableStore[*unstructured.Unstructured] {
	opts := thresholdStoreOpts(dir)
	bopts := newBadgerOpts(filepath.Join(dir, thresholdDbName))
	bopts.Logger = NewLoggerShim(logr.Discard())
	db, err := badger.Open(bopts)
	Expect(err).ToNot(HaveOccurred())
	s := wrapThresholdDB(db, opts)
	DeferCleanup(func() {
		if !s.db.IsClosed() {
			closeStore(s)
		}
	})
	return s
}

func closeStore(s *SortableStore[*unstructured.Unstructured]) {
	Expect(s.db.Close()).To(Succeed())
}

// openDiskStore opens a store and registers a checked close that is skipped
// if the test already closed it explicitly.
func openDiskStore(dir string) *SortableStore[*unstructured.Unstructured] {
	s := newThresholdStore(dir)
	DeferCleanup(func() {
		if !s.db.IsClosed() {
			closeStore(s)
		}
	})
	return s
}

func listOrders(ctx context.Context, s *SortableStore[*unstructured.Unstructured]) []string {
	res, err := s.List(ctx, store.ListOpts{
		Limit:   10,
		Sorters: []store.Sorter{{Path: "spec.order", Order: store.SortOrderAsc}},
	})
	Expect(err).ToNot(HaveOccurred())
	orders := make([]string, 0, len(res.Items))
	for _, item := range res.Items {
		order, _, _ := unstructured.NestedString(item.Object, "spec", "order")
		orders = append(orders, order)
	}
	return orders
}

var _ = Describe("Value thresholds", func() {
	ctx := context.Background()

	DescribeTable("profile selected by filepath",
		func(inMemory bool, memTable int64, numMemtables int, blockCache, vlog, threshold int64) {
			path := ""
			if !inMemory {
				path = GinkgoT().TempDir()
			}
			opts := newBadgerOpts(path)
			Expect(opts.InMemory).To(Equal(inMemory))
			Expect(opts.MemTableSize).To(Equal(memTable))
			Expect(opts.NumMemtables).To(Equal(numMemtables))
			Expect(opts.BlockCacheSize).To(Equal(blockCache))
			Expect(opts.BlockSize).To(Equal(4 << 10))
			Expect(opts.ValueLogFileSize).To(Equal(vlog))
			Expect(opts.ValueThreshold).To(Equal(threshold))
			Expect(valueThreshold(inMemory)).To(Equal(threshold))
		},
		Entry("empty filepath: in memory, default profile", true, int64(16<<20), 4, int64(64<<20), int64(512<<20), int64(1<<20)),
		Entry("filepath set: on disk, reduced-memory profile", false, int64(32<<20), 2, int64(32<<20), int64(64<<20), int64(4<<10)),
	)

	It("rejects in-memory objects at or above 1 MiB", func() {
		limit := int(valueThreshold(true))
		Expect(limit).To(Equal(1 << 20))
		s := newThresholdStore("")
		DeferCleanup(closeStore, s)

		for _, size := range []int{4097, limit - 1} {
			obj := sizedObject("ok", "b", size)
			Expect(s.OnCreate(ctx, obj)).To(Succeed())
			got, err := s.Get(ctx, "default", "ok")
			Expect(err).ToNot(HaveOccurred())
			Expect(serializedSize(got)).To(Equal(size))
		}

		for _, size := range []int{limit, limit + 1} {
			err := s.OnCreate(ctx, sizedObject("new", "a", size))
			Expect(err).To(MatchError(And(
				ContainSubstring("default/new/"),
				ContainSubstring("limit is less than"),
			)))
			_, err = s.Get(ctx, "default", "new")
			Expect(err).To(HaveOccurred())

			Expect(s.OnUpdate(ctx, sizedObject("ok", "z", size))).ToNot(Succeed())
			got, err := s.Get(ctx, "default", "ok")
			Expect(err).ToNot(HaveOccurred())
			Expect(serializedSize(got)).To(Equal(limit - 1))
			Expect(got.Object["spec"].(map[string]any)["order"]).To(Equal("b"))
		}

		Expect(s.OnCreate(ctx, thresholdObject("other", "c", 0))).To(Succeed())
		Expect(listOrders(ctx, s)).To(Equal([]string{"b", "c"}))
	})

	It("persists large values on disk across replacement, deletion and raw Badger reopen", func() {
		dir := GinkgoT().TempDir()
		sizes := []int{4095, 4096, 4097, 512 << 10, 1 << 20, (1 << 20) + 1}

		s := openDiskStore(dir)
		for i, size := range sizes {
			obj := sizedObject("obj-"+string(rune('a'+i)), string(rune('a'+i)), size)
			Expect(s.OnCreate(ctx, obj)).To(Succeed())
		}

		// Replace and delete values stored in the value log.
		Expect(s.OnCreate(ctx, sizedObject("replaced", "x", 8<<10))).To(Succeed())
		Expect(s.OnCreate(ctx, sizedObject("deleted", "y", 8<<10))).To(Succeed())
		Expect(s.OnUpdate(ctx, sizedObject("replaced", "r", 16<<10))).To(Succeed())
		Expect(s.OnDelete(ctx, thresholdObject("deleted", "y", 0))).To(Succeed())
		Expect(listOrders(ctx, s)).To(Equal([]string{"a", "b", "c", "d", "e", "f", "r"}))
		Expect(s.getSortValue("spec.order", "default/deleted/")).To(BeNil())
		closeStore(s)

		// Raw reopen (not store startup): newDbOrDie would reset the directory.
		s = rawReopenDiskStore(dir)
		for i, size := range sizes {
			got, err := s.Get(ctx, "default", "obj-"+string(rune('a'+i)))
			Expect(err).ToNot(HaveOccurred())
			Expect(serializedSize(got)).To(Equal(size))
		}
		got, err := s.Get(ctx, "default", "replaced")
		Expect(err).ToNot(HaveOccurred())
		Expect(serializedSize(got)).To(Equal(16 << 10))
		Expect(got.Object["spec"].(map[string]any)["order"]).To(Equal("r"))
		_, err = s.Get(ctx, "default", "deleted")
		Expect(err).To(HaveOccurred())
	})
})
