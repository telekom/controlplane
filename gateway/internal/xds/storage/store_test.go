// SPDX-FileCopyrightText: 2026 Deutsche Telekom AG
// SPDX-License-Identifier: Apache-2.0

package storage

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"

	xdsapi "github.com/telekom/controlplane/gateway/internal/xds/api/v1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func testBundle(publisherGeneration, compilerVersion string) *xdsapi.Bundle {
	bundle := &xdsapi.Bundle{
		TargetId: "target-a", PublisherGeneration: publisherGeneration,
		SchemaVersion: xdsapi.SchemaVersion, CompilerVersion: compilerVersion, EnvoyVersion: "1.37",
		Sources: []*xdsapi.SourceReference{{Kind: "Gateway", Name: "gateway-a"}},
	}
	Expect(xdsapi.SetDigest(bundle)).To(Succeed())
	return bundle
}

var _ = Describe("SQLite store", func() {
	var (
		ctx   context.Context
		path  string
		store *Store
	)

	BeforeEach(func() {
		ctx = context.Background()
		path = filepath.Join(GinkgoT().TempDir(), "xds.db")
		var err error
		store, err = Open(ctx, path, 2)
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		if store != nil {
			Expect(store.Close()).To(Succeed())
		}
	})

	It("uses WAL and private database permissions", func() {
		mode, err := store.JournalMode(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(mode).To(Equal("wal"))
		info, err := os.Stat(path)
		Expect(err).NotTo(HaveOccurred())
		Expect(info.Mode().Perm()).To(Equal(os.FileMode(0o600)))
	})

	It("rejects legacy schemas that cannot preserve pruned replay history", func() {
		Expect(store.Close()).To(Succeed())
		store = nil
		database, err := sql.Open("sqlite", path)
		Expect(err).NotTo(HaveOccurred())
		_, err = database.ExecContext(ctx, "DELETE FROM schema_migrations")
		Expect(err).NotTo(HaveOccurred())
		_, err = database.ExecContext(ctx, "INSERT INTO schema_migrations(version) VALUES (1)")
		Expect(err).NotTo(HaveOccurred())
		Expect(database.Close()).To(Succeed())

		_, err = Open(ctx, path, 2)
		Expect(err).To(MatchError("unsupported database schema version 1"))
	})

	It("is idempotent by target and digest and rejects generation conflicts", func() {
		bundle := testBundle("source-1", "compiler-a")
		first, err := store.Activate(ctx, bundle)
		Expect(err).NotTo(HaveOccurred())
		Expect(first.Generation).To(Equal(uint64(1)))
		Expect(first.Idempotent).To(BeFalse())

		second, err := store.Activate(ctx, bundle)
		Expect(err).NotTo(HaveOccurred())
		Expect(second).To(Equal(Activation{Generation: 1, Idempotent: true, Active: true}))

		conflict := testBundle("source-1", "compiler-b")
		_, err = store.Activate(ctx, conflict)
		Expect(err).To(MatchError(ErrGenerationConflict))
	})

	It("does not reactivate an older digest replay", func() {
		older := testBundle("source-1", "compiler-a")
		newer := testBundle("source-2", "compiler-b")
		_, err := store.Activate(ctx, older)
		Expect(err).NotTo(HaveOccurred())
		_, err = store.Activate(ctx, newer)
		Expect(err).NotTo(HaveOccurred())

		replay, err := store.Activate(ctx, older)
		Expect(err).NotTo(HaveOccurred())
		Expect(replay).To(Equal(Activation{Generation: 1, Idempotent: true, Active: false}))
		active, exists, err := store.Active(ctx, "target-a")
		Expect(err).NotTo(HaveOccurred())
		Expect(exists).To(BeTrue())
		Expect(active.Generation).To(Equal(uint64(2)))
	})

	It("reopens with the active canonical bundle and bounded history", func() {
		for generation := 1; generation <= 3; generation++ {
			bundle := testBundle(string(rune('0'+generation)), string(rune('a'+generation)))
			_, err := store.Activate(ctx, bundle)
			Expect(err).NotTo(HaveOccurred())
		}
		var count int
		Expect(store.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM bundles WHERE target_id='target-a'").Scan(&count)).To(Succeed())
		Expect(count).To(Equal(2))
		Expect(store.Close()).To(Succeed())
		store = nil

		reopened, err := Open(ctx, path, 2)
		Expect(err).NotTo(HaveOccurred())
		store = reopened
		active, err := store.LoadActive(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(active["target-a"].Generation).To(Equal(uint64(3)))
		Expect(active["target-a"].Bundle.PublisherGeneration).To(Equal("3"))

		pruned := testBundle("1", "b")
		replay, err := store.Activate(ctx, pruned)
		Expect(err).NotTo(HaveOccurred())
		Expect(replay).To(Equal(Activation{Generation: 1, Idempotent: true, Active: false}))

		conflict := testBundle("1", "different")
		_, err = store.Activate(ctx, conflict)
		Expect(err).To(MatchError(ErrGenerationConflict))
	})

	It("retains delivery observations for separate generations", func() {
		for generation := uint64(1); generation <= 2; generation++ {
			observation := &xdsapi.DeliveryObservation{
				NodeId: "node-a", TypeUrl: "type-a", Generation: generation,
				State: xdsapi.DeliveryState_DELIVERY_STATE_ACK, ObservedAt: timestampFromUnixNano(int64(generation)),
			}
			Expect(store.RecordObservation(ctx, "target-a", observation)).To(Succeed())
		}
		observations, err := store.Observations(ctx, "target-a")
		Expect(err).NotTo(HaveOccurred())
		Expect(observations).To(HaveLen(2))
		Expect(observations[0].Generation).To(Equal(uint64(1)))
		Expect(observations[1].Generation).To(Equal(uint64(2)))
	})

	It("fails loading a corrupt active envelope", func() {
		_, err := store.Activate(ctx, testBundle("1", "compiler-a"))
		Expect(err).NotTo(HaveOccurred())
		_, err = store.db.ExecContext(ctx, "UPDATE bundles SET envelope = x'00' WHERE target_id='target-a'")
		Expect(err).NotTo(HaveOccurred())

		_, err = store.LoadActive(ctx)
		Expect(err).To(MatchError(ContainSubstring("decoding active bundle")))
	})
})
