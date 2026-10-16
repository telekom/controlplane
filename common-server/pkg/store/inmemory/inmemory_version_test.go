// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package inmemory

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sync"

	"github.com/dgraph-io/badger/v4"
	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/tidwall/gjson"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
)

const versionSortPath = "spec.order"

// versionObject builds an object; empty uid or rv leaves the field unset.
func versionObject(name string, uid types.UID, rv string, spec map[string]any) *unstructured.Unstructured {
	meta := map[string]any{"name": name, "namespace": "default"}
	if uid != "" {
		meta["uid"] = string(uid)
	}
	if rv != "" {
		meta["resourceVersion"] = rv
	}
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "test.example/v1",
		"kind":       "Thing",
		"metadata":   meta,
		"spec":       spec,
	}}
}

func newVersionStore(db *badger.DB) *InmemoryObjectStore[*unstructured.Unstructured] {
	opts := StoreOpts{GVR: thresholdTestGVR, AllowedSorts: []string{versionSortPath}}
	ios := &InmemoryObjectStore[*unstructured.Unstructured]{
		ctx: context.Background(),
		log: logr.Discard(),
		db:  db,
	}
	Sortable(ios, opts)
	return ios
}

func openRawDB(dir string, readOnly bool) *badger.DB {
	db, err := badger.Open(newBadgerOpts(dir).WithReadOnly(readOnly).WithLogger(nil))
	Expect(err).ToNot(HaveOccurred())
	// Registered per opened DB so every handle is closed even if the test fails.
	DeferCleanup(closeDB, db)
	return db
}

func closeDB(db *badger.DB) {
	if !db.IsClosed() {
		Expect(db.Close()).To(Succeed())
	}
}

func readRaw(db *badger.DB, key string) ([]byte, bool) {
	var data []byte
	err := db.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(key))
		if err != nil {
			return err
		}
		data, err = item.ValueCopy(nil)
		return err
	})
	if errors.Is(err, badger.ErrKeyNotFound) {
		return nil, false
	}
	Expect(err).ToNot(HaveOccurred())
	return data, true
}

func cachedSortValue(s *InmemoryObjectStore[*unstructured.Unstructured], key string) (any, bool) {
	m, ok := s.sortValueCache.Load(versionSortPath)
	Expect(ok).To(BeTrue())
	return m.(*sync.Map).Load(key)
}

// storedRV returns the resourceVersion persisted for key.
func storedRV(db *badger.DB, key string) string {
	data, found := readRaw(db, key)
	Expect(found).To(BeTrue())
	return gjson.GetBytes(data, "metadata.resourceVersion").String()
}

// expectConsistent asserts that the DB, versionCache and sort cache agree for key.
func expectConsistent(s *InmemoryObjectStore[*unstructured.Unstructured], key string) {
	data, found := readRaw(s.db, key)
	version, versioned := s.versionCache[key]
	sortValue, sorted := cachedSortValue(s, key)
	if !found {
		Expect(versioned).To(BeFalse(), "version cached for deleted key %s", key)
		Expect(sorted).To(BeFalse(), "sort value cached for deleted key %s", key)
		return
	}
	rv := gjson.GetBytes(data, "metadata.resourceVersion").String()
	if rv == "" {
		Expect(versioned).To(BeFalse(), "version cached for versionless key %s", key)
	} else {
		Expect(versioned).To(BeTrue(), "version missing for key %s", key)
		Expect(version).To(Equal(cachedObjectVersion{
			uid:             types.UID(gjson.GetBytes(data, "metadata.uid").String()),
			resourceVersion: rv,
		}))
	}
	expected := gjson.GetBytes(data, versionSortPath)
	Expect(sorted).To(Equal(expected.Exists()))
	if expected.Exists() {
		Expect(sortValue).To(Equal(expected.Value()))
	}
}

var _ = Describe("InmemoryObjectStore version deduplication", func() {
	var (
		ctx context.Context
		dir string
	)

	BeforeEach(func() {
		ctx = context.Background()
		dir = GinkgoT().TempDir()
	})

	newStore := func() *InmemoryObjectStore[*unstructured.Unstructured] {
		return newVersionStore(openRawDB(dir, false))
	}

	It("does not allocate the version cache before a versioned write", func() {
		s := newStore()
		Expect(s.versionCache).To(BeNil())
		Expect(s.OnUpdate(ctx, versionObject("a", "", "", nil))).To(Succeed())
		Expect(s.versionCache).To(BeNil())
	})

	It("skips the write for the same UID and resourceVersion", func() {
		s := newStore()
		obj := versionObject("a", "u1", "100", map[string]any{"order": "1"})
		Expect(s.OnUpdate(ctx, obj)).To(Succeed())
		version := s.db.MaxVersion()

		Expect(s.OnUpdate(ctx, obj)).To(Succeed())
		Expect(s.db.MaxVersion()).To(Equal(version))
		expectConsistent(s, calculateKey(obj))
	})

	It("skips duplicates before serialization", func() {
		s := newStore()
		Expect(s.OnUpdate(ctx, versionObject("a", "u1", "100", map[string]any{"order": "1"}))).To(Succeed())
		version := s.db.MaxVersion()

		// NaN cannot be serialized; success proves the fast path ran first.
		Expect(s.OnUpdate(ctx, versionObject("a", "u1", "100", map[string]any{"bad": math.NaN()}))).To(Succeed())
		Expect(s.db.MaxVersion()).To(Equal(version))
		// The same value with a new version reaches serialization and fails.
		Expect(s.OnUpdate(ctx, versionObject("a", "u1", "101", map[string]any{"bad": math.NaN()}))).ToNot(Succeed())
	})

	It("ignores map ordering for the same version", func() {
		s := newStore()
		Expect(s.OnUpdate(ctx, versionObject("a", "u1", "100", map[string]any{"order": "1", "x": int64(1), "y": map[string]any{"p": int64(1), "q": int64(2)}}))).To(Succeed())
		version := s.db.MaxVersion()

		for range 20 {
			again := versionObject("a", "u1", "100", map[string]any{"y": map[string]any{"q": int64(2), "p": int64(1)}, "x": int64(1), "order": "1"})
			Expect(s.OnUpdate(ctx, again)).To(Succeed())
		}
		Expect(s.db.MaxVersion()).To(Equal(version))
	})

	It("treats the version as authoritative and skips a changed payload with the same version", func() {
		s := newStore()
		obj := versionObject("a", "u1", "100", map[string]any{"order": "1"})
		key := calculateKey(obj)
		Expect(s.OnUpdate(ctx, obj)).To(Succeed())
		version := s.db.MaxVersion()

		Expect(s.OnUpdate(ctx, versionObject("a", "u1", "100", map[string]any{"order": "2"}))).To(Succeed())
		Expect(s.db.MaxVersion()).To(Equal(version))
		v, ok := cachedSortValue(s, key)
		Expect(ok).To(BeTrue())
		Expect(v).To(Equal("1"))
		expectConsistent(s, key)
	})

	It("writes a changed resourceVersion with the same payload, comparing versions opaquely", func() {
		s := newStore()
		obj := versionObject("a", "u1", "100", map[string]any{"order": "1"})
		key := calculateKey(obj)
		Expect(s.OnUpdate(ctx, obj)).To(Succeed())
		version := s.db.MaxVersion()

		// "2" sorts before "100"; no ordering is applied.
		Expect(s.OnUpdate(ctx, versionObject("a", "u1", "2", map[string]any{"order": "1"}))).To(Succeed())
		Expect(s.db.MaxVersion()).To(BeNumerically(">", version))
		Expect(storedRV(s.db, key)).To(Equal("2"))
		expectConsistent(s, key)
	})

	It("writes a changed UID with the same resourceVersion", func() {
		s := newStore()
		obj := versionObject("a", "u1", "100", map[string]any{"order": "1"})
		key := calculateKey(obj)
		Expect(s.OnUpdate(ctx, obj)).To(Succeed())
		version := s.db.MaxVersion()

		Expect(s.OnUpdate(ctx, versionObject("a", "u2", "100", map[string]any{"order": "1"}))).To(Succeed())
		Expect(s.db.MaxVersion()).To(BeNumerically(">", version))
		expectConsistent(s, key)
		Expect(s.versionCache[key].uid).To(Equal(types.UID("u2")))
	})

	DescribeTable("always writes versionless objects",
		func(uid types.UID) {
			s := newStore()
			obj := versionObject("a", uid, "", map[string]any{"order": "1"})
			Expect(s.OnUpdate(ctx, obj)).To(Succeed())
			version := s.db.MaxVersion()

			Expect(s.OnUpdate(ctx, obj)).To(Succeed())
			Expect(s.db.MaxVersion()).To(Equal(version + 1))
			expectConsistent(s, calculateKey(obj))
		},
		Entry("without UID", types.UID("")),
		Entry("with UID", types.UID("u1")),
	)

	It("invalidates the cached version after a versionless write", func() {
		s := newStore()
		a := versionObject("a", "u1", "100", map[string]any{"order": "A"})
		key := calculateKey(a)
		Expect(s.OnUpdate(ctx, a)).To(Succeed())
		Expect(s.OnUpdate(ctx, versionObject("a", "u1", "", map[string]any{"order": "B"}))).To(Succeed())
		Expect(s.versionCache).ToNot(HaveKey(key))
		expectConsistent(s, key)

		Expect(s.OnUpdate(ctx, a)).To(Succeed())
		Expect(storedRV(s.db, key)).To(Equal("100"))
		v, _ := cachedSortValue(s, key)
		Expect(v).To(Equal("A"))
		expectConsistent(s, key)
	})

	It("preserves the cached version when a versionless write fails", func() {
		rw := openRawDB(dir, false)
		s := newVersionStore(rw)
		a := versionObject("a", "u1", "100", map[string]any{"order": "A"})
		key := calculateKey(a)
		Expect(s.OnUpdate(ctx, a)).To(Succeed())
		closeDB(rw)

		s.db = openRawDB(dir, true)
		Expect(s.OnUpdate(ctx, versionObject("a", "u1", "", map[string]any{"order": "B"}))).ToNot(Succeed())
		Expect(s.versionCache).To(HaveKeyWithValue(key, cachedObjectVersion{uid: "u1", resourceVersion: "100"}))
		v, _ := cachedSortValue(s, key)
		Expect(v).To(Equal("A"))
		expectConsistent(s, key)
	})

	It("preserves the cached version on a failed changed write and retries successfully", func() {
		rw := openRawDB(dir, false)
		s := newVersionStore(rw)
		a := versionObject("a", "u1", "100", map[string]any{"order": "A"})
		key := calculateKey(a)
		Expect(s.OnUpdate(ctx, a)).To(Succeed())
		closeDB(rw)

		ro := openRawDB(dir, true)
		s.db = ro
		b := versionObject("a", "u1", "2", map[string]any{"order": "B"})
		Expect(s.OnUpdate(ctx, b)).ToNot(Succeed())
		Expect(s.versionCache).To(HaveKeyWithValue(key, cachedObjectVersion{uid: "u1", resourceVersion: "100"}))
		v, _ := cachedSortValue(s, key)
		Expect(v).To(Equal("A"))
		expectConsistent(s, key)
		closeDB(ro)

		s.db = openRawDB(dir, false)
		Expect(s.OnUpdate(ctx, b)).To(Succeed())
		Expect(storedRV(s.db, key)).To(Equal("2"))
		expectConsistent(s, key)
	})

	It("does not cache the version of a failed first write", func() {
		closeDB(openRawDB(dir, false))
		s := newVersionStore(openRawDB(dir, true))
		obj := versionObject("a", "u1", "100", map[string]any{"order": "1"})
		key := calculateKey(obj)

		Expect(s.OnUpdate(ctx, obj)).ToNot(Succeed())
		Expect(s.versionCache).ToNot(HaveKey(key))
		_, ok := cachedSortValue(s, key)
		Expect(ok).To(BeFalse())
	})

	It("preserves the cached version when a delete fails", func() {
		rw := openRawDB(dir, false)
		s := newVersionStore(rw)
		obj := versionObject("a", "u1", "100", map[string]any{"order": "1"})
		key := calculateKey(obj)
		Expect(s.OnUpdate(ctx, obj)).To(Succeed())
		closeDB(rw)

		s.db = openRawDB(dir, true)
		Expect(s.OnDelete(ctx, obj)).ToNot(Succeed())
		Expect(s.versionCache).To(HaveKeyWithValue(key, cachedObjectVersion{uid: "u1", resourceVersion: "100"}))
		v, ok := cachedSortValue(s, key)
		Expect(ok).To(BeTrue())
		Expect(v).To(Equal("1"))
		// Still a duplicate; a write would fail on the read-only DB.
		Expect(s.OnUpdate(ctx, obj)).To(Succeed())
		expectConsistent(s, key)
	})

	It("writes again after deletion and recreation with a new UID", func() {
		s := newStore()
		obj := versionObject("a", "u1", "100", map[string]any{"order": "1"})
		key := calculateKey(obj)
		Expect(s.OnUpdate(ctx, obj)).To(Succeed())
		Expect(s.OnDelete(ctx, obj)).To(Succeed())
		expectConsistent(s, key)

		Expect(s.OnUpdate(ctx, versionObject("a", "u2", "100", map[string]any{"order": "1"}))).To(Succeed())
		_, found := readRaw(s.db, key)
		Expect(found).To(BeTrue())
		expectConsistent(s, key)
	})

	It("writes only once for concurrent duplicates", func() {
		s := newStore()
		before := s.db.MaxVersion()
		var wg sync.WaitGroup
		for range 32 {
			wg.Go(func() {
				defer GinkgoRecover()
				Expect(s.OnUpdate(ctx, versionObject("a", "u1", "100", map[string]any{"order": "1"}))).To(Succeed())
			})
		}
		wg.Wait()
		Expect(s.db.MaxVersion()).To(Equal(before + 1))
	})

	DescribeTable("keeps DB and caches in agreement under concurrent mutations",
		func(endWithDelete bool) {
			s := newStore()
			const keys, workers, iterations = 4, 8, 50
			var wg sync.WaitGroup
			for w := range workers {
				wg.Go(func() {
					defer GinkgoRecover()
					for i := range iterations {
						name := fmt.Sprintf("obj-%d", (w+i)%keys)
						rv := ""
						if i%4 != 0 {
							rv = fmt.Sprintf("%d", i%3)
						}
						obj := versionObject(name, "u1", rv, map[string]any{"order": fmt.Sprintf("%d", i%3)})
						if (w+i)%5 == 0 {
							Expect(s.OnDelete(ctx, obj)).To(Succeed())
							continue
						}
						Expect(s.OnUpdate(ctx, obj)).To(Succeed())
					}
				})
			}
			wg.Wait()

			for k := range keys {
				expectConsistent(s, calculateKey(versionObject(fmt.Sprintf("obj-%d", k), "", "", nil)))
			}

			for k := range keys {
				obj := versionObject(fmt.Sprintf("obj-%d", k), "u1", "final", map[string]any{"order": "final"})
				if endWithDelete {
					Expect(s.OnDelete(ctx, obj)).To(Succeed())
				} else {
					Expect(s.OnUpdate(ctx, obj)).To(Succeed())
				}
				expectConsistent(s, calculateKey(obj))
			}
		},
		Entry("ending with update", false),
		Entry("ending with delete", true),
	)
})
