// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package permissionset_test

import (
	"context"
	"errors"
	"fmt"

	"entgo.io/ent/privacy"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	_ "github.com/mattn/go-sqlite3"
	"github.com/telekom/controlplane/controlplane-api/ent"
	"github.com/telekom/controlplane/controlplane-api/ent/enttest"
	entpermissionset "github.com/telekom/controlplane/controlplane-api/ent/permissionset"
	_ "github.com/telekom/controlplane/controlplane-api/ent/runtime"
	"github.com/telekom/controlplane/controlplane-api/ent/zone"
	"github.com/telekom/controlplane/controlplane-api/pkg/model"

	"github.com/telekom/controlplane/projector/internal/domain/permissionset"
	"github.com/telekom/controlplane/projector/internal/domain/shared"
	"github.com/telekom/controlplane/projector/internal/infrastructure"
	"github.com/telekom/controlplane/projector/internal/runtime"
)

// mockPermissionSetDeps implements permissionset.PermissionSetDeps for testing.
type mockPermissionSetDeps struct {
	appIDs map[string]int // key: "appName:teamName"
	appErr error          // if non-nil, FindApplicationID always returns this error
}

func (m *mockPermissionSetDeps) FindApplicationID(_ context.Context, name, teamName string) (int, error) {
	if m.appErr != nil {
		return 0, m.appErr
	}
	key := name + ":" + teamName
	if id, ok := m.appIDs[key]; ok {
		return id, nil
	}
	return 0, fmt.Errorf("application %q (team %q): %w", name, teamName, infrastructure.ErrEntityNotFound)
}

var _ = Describe("PermissionSet Repository", func() {
	var (
		client *ent.Client
		cache  *infrastructure.EdgeCache
		deps   *mockPermissionSetDeps
		repo   *permissionset.Repository
		ctx    context.Context
		appID  int
	)

	BeforeEach(func() {
		ctx = privacy.DecisionContext(context.Background(), privacy.Allow)
		var err error
		cache, err = infrastructure.NewEdgeCache(100_000, 10<<20, 64)
		Expect(err).NotTo(HaveOccurred())
		client = enttest.Open(GinkgoT(), "sqlite3", "file:ent?mode=memory&_fk=1")

		// Seed Zone → Team → Application — required dependency chain.
		z, err := client.Zone.Create().
			SetName("caas").
			SetVisibility(zone.VisibilityEnterprise).
			Save(ctx)
		Expect(err).NotTo(HaveOccurred())

		t, err := client.Team.Create().
			SetName("platform--narvi").
			SetEmail("narvi@example.com").
			SetNamespace("platform--narvi").
			Save(ctx)
		Expect(err).NotTo(HaveOccurred())

		app, err := client.Application.Create().
			SetName("my-app").
			SetNamespace("platform--narvi").
			SetOwnerTeamID(t.ID).
			SetZoneID(z.ID).
			Save(ctx)
		Expect(err).NotTo(HaveOccurred())
		appID = app.ID

		deps = &mockPermissionSetDeps{
			appIDs: map[string]int{"my-app:platform--narvi": appID},
		}
		repo = permissionset.NewRepository(client, cache, deps)
	})

	AfterEach(func() {
		_ = client.Close()
		cache.Close()
	})

	Describe("Upsert", func() {
		It("should create a permission set with valid deps", func() {
			data := &permissionset.PermissionSetData{
				Meta:          shared.NewMetadata("prod--platform--narvi", "my-app", nil),
				StatusPhase:   "READY",
				StatusMessage: "ok",
				Permissions: []model.Permission{
					{Role: "admin", Resource: "orders", Actions: []string{"read", "write"}},
				},
				AppName:  "my-app",
				TeamName: "platform--narvi",
			}
			Expect(repo.Upsert(ctx, data)).To(Succeed())

			ps, err := client.PermissionSet.Query().Only(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(ps.Permissions).To(HaveLen(1))
			Expect(ps.Permissions[0].Role).To(Equal("admin"))
			Expect(ps.Permissions[0].Resource).To(Equal("orders"))
			Expect(ps.Permissions[0].Actions).To(Equal([]string{"read", "write"}))

			// Verify FK edge.
			owner, err := ps.QueryOwnerApplication().Only(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(owner.ID).To(Equal(appID))
		})

		It("should return ErrDependencyMissing when application is missing", func() {
			data := &permissionset.PermissionSetData{
				Meta:        shared.NewMetadata("prod--platform--narvi", "fail-app", nil),
				StatusPhase: "UNKNOWN",
				Permissions: []model.Permission{
					{Role: "admin", Resource: "orders", Actions: []string{"read"}},
				},
				AppName:  "missing-app",
				TeamName: "platform--narvi",
			}
			err := repo.Upsert(ctx, data)
			Expect(err).To(HaveOccurred())
			Expect(runtime.IsDependencyMissing(err)).To(BeTrue())
			Expect(err.Error()).To(ContainSubstring("application"))
		})

		It("should propagate non-ErrEntityNotFound errors from FindApplicationID", func() {
			dbErr := errors.New("connection refused")
			failDeps := &mockPermissionSetDeps{
				appIDs: map[string]int{},
				appErr: dbErr,
			}
			failRepo := permissionset.NewRepository(client, cache, failDeps)

			data := &permissionset.PermissionSetData{
				Meta:        shared.NewMetadata("prod--platform--narvi", "my-app", nil),
				StatusPhase: "UNKNOWN",
				Permissions: []model.Permission{
					{Role: "admin", Resource: "orders", Actions: []string{"read"}},
				},
				AppName:  "my-app",
				TeamName: "platform--narvi",
			}
			err := failRepo.Upsert(ctx, data)
			Expect(err).To(HaveOccurred())
			Expect(runtime.IsDependencyMissing(err)).To(BeFalse())
			Expect(errors.Is(err, dbErr)).To(BeTrue())
		})

		It("should update existing permission set on conflict with UpdateNewValues", func() {
			data := &permissionset.PermissionSetData{
				Meta:        shared.NewMetadata("prod--platform--narvi", "my-app", nil),
				StatusPhase: "PENDING",
				Permissions: []model.Permission{
					{Role: "viewer", Resource: "orders", Actions: []string{"read"}},
				},
				AppName:  "my-app",
				TeamName: "platform--narvi",
			}
			Expect(repo.Upsert(ctx, data)).To(Succeed())

			// Update with new values.
			data.StatusPhase = "READY"
			data.StatusMessage = "v2"
			data.Permissions = []model.Permission{
				{Role: "admin", Resource: "orders", Actions: []string{"read", "write"}},
				{Role: "viewer", Resource: "invoices", Actions: []string{"read"}},
			}
			Expect(repo.Upsert(ctx, data)).To(Succeed())

			count, err := client.PermissionSet.Query().Count(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(count).To(Equal(1))

			ps, err := client.PermissionSet.Query().Where(entpermissionset.StatusPhaseEQ(entpermissionset.StatusPhaseReady)).Only(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(*ps.StatusMessage).To(Equal("v2"))
			Expect(ps.Permissions).To(HaveLen(2))
			Expect(ps.Permissions[1].Resource).To(Equal("invoices"))
		})

		It("should populate the edge cache after upsert", func() {
			data := &permissionset.PermissionSetData{
				Meta:        shared.NewMetadata("prod--platform--narvi", "my-app", nil),
				StatusPhase: "READY",
				Permissions: []model.Permission{
					{Role: "admin", Resource: "orders", Actions: []string{"read"}},
				},
				AppName:  "my-app",
				TeamName: "platform--narvi",
			}
			Expect(repo.Upsert(ctx, data)).To(Succeed())
			cache.Wait()

			_, found := cache.Get("permissionset", "my-app:platform--narvi")
			Expect(found).To(BeTrue())
		})

		It("should propagate a validation error for an invalid StatusPhase enum", func() {
			data := &permissionset.PermissionSetData{
				Meta:        shared.NewMetadata("prod--platform--narvi", "my-app", nil),
				StatusPhase: "BOGUS",
				Permissions: []model.Permission{
					{Role: "admin", Resource: "orders", Actions: []string{"read"}},
				},
				AppName:  "my-app",
				TeamName: "platform--narvi",
			}
			err := repo.Upsert(ctx, data)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("status_phase"))
		})
	})

	Describe("Delete", func() {
		It("should delete an existing permission set", func() {
			data := &permissionset.PermissionSetData{
				Meta:        shared.NewMetadata("prod--platform--narvi", "my-app", nil),
				StatusPhase: "READY",
				Permissions: []model.Permission{
					{Role: "admin", Resource: "orders", Actions: []string{"read"}},
				},
				AppName:  "my-app",
				TeamName: "platform--narvi",
			}
			Expect(repo.Upsert(ctx, data)).To(Succeed())

			key := permissionset.PermissionSetKey{AppName: "my-app", TeamName: "platform--narvi"}
			Expect(repo.Delete(ctx, key)).To(Succeed())

			count, err := client.PermissionSet.Query().Count(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(count).To(Equal(0))
		})

		It("should be idempotent for non-existent permission set", func() {
			key := permissionset.PermissionSetKey{AppName: "my-app", TeamName: "platform--narvi"}
			Expect(repo.Delete(ctx, key)).To(Succeed())
		})

		It("should evict from edge cache after delete", func() {
			data := &permissionset.PermissionSetData{
				Meta:        shared.NewMetadata("prod--platform--narvi", "my-app", nil),
				StatusPhase: "READY",
				Permissions: []model.Permission{
					{Role: "admin", Resource: "orders", Actions: []string{"read"}},
				},
				AppName:  "my-app",
				TeamName: "platform--narvi",
			}
			Expect(repo.Upsert(ctx, data)).To(Succeed())
			cache.Wait()

			_, found := cache.Get("permissionset", "my-app:platform--narvi")
			Expect(found).To(BeTrue())

			key := permissionset.PermissionSetKey{AppName: "my-app", TeamName: "platform--narvi"}
			Expect(repo.Delete(ctx, key)).To(Succeed())
			cache.Wait()

			_, found = cache.Get("permissionset", "my-app:platform--narvi")
			Expect(found).To(BeFalse())
		})
	})
})
