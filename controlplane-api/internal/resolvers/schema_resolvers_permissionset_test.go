// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package resolvers_test

import (
	"context"
	"os"
	"path/filepath"
	"runtime"

	"github.com/telekom/controlplane/controlplane-api/ent"
	"github.com/telekom/controlplane/controlplane-api/internal/resolvers"
	"github.com/telekom/controlplane/controlplane-api/internal/service"
	"github.com/telekom/controlplane/controlplane-api/internal/testutil"
	"github.com/telekom/controlplane/controlplane-api/pkg/model"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("PermissionSet resolver", func() {
	var (
		client *ent.Client
		r      *resolvers.Resolver
		s      *testutil.SeedData
	)

	BeforeEach(func() {
		client = testutil.NewTestClient(GinkgoT())
		s = testutil.SeedStandard(client)
		r = resolvers.NewResolver(client, service.Services{}, nil, "")
	})

	AfterEach(func() {
		client.Close()
	})

	It("should be queryable via the top-level permissionSets query resolver", func() {
		ctx := testutil.AllowContext()

		conn, err := r.Query().PermissionSets(ctx, nil, nil, nil, nil, nil, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(conn.Edges).To(HaveLen(1))
		Expect(conn.Edges[0].Node.ID).To(Equal(s.PermissionSetAlpha.ID))
	})

	It("should be reachable from Application.permissionSet", func() {
		ctx := testutil.AllowContext()

		reloaded, err := client.Application.Get(ctx, s.AppAlpha.ID)
		Expect(err).NotTo(HaveOccurred())

		ps, err := reloaded.QueryPermissionSet().Only(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(ps.ID).To(Equal(s.PermissionSetAlpha.ID))
		Expect(ps.Permissions).To(HaveLen(1))
		Expect(ps.Permissions[0]).To(Equal(model.Permission{
			Role:     "admin",
			Resource: "orders",
			Actions:  []string{"read", "write"},
		}))
	})

	It("should not expose any create/update/delete mutations for PermissionSet in the GraphQL schema", func() {
		_, thisFile, _, ok := runtime.Caller(0)
		Expect(ok).To(BeTrue())
		mutationSchemaPath := filepath.Join(filepath.Dir(thisFile), "..", "..", "mutation.graphql")

		data, err := os.ReadFile(mutationSchemaPath)
		Expect(err).NotTo(HaveOccurred())
		schema := string(data)

		Expect(schema).NotTo(ContainSubstring("createPermissionSet"))
		Expect(schema).NotTo(ContainSubstring("updatePermissionSet"))
		Expect(schema).NotTo(ContainSubstring("deletePermissionSet"))
	})

	It("should deny access to PermissionSet queries when no viewer is present in context", func() {
		// No viewer + no privacy bypass: PrivacyMixin's DenyIfNoViewer rule must trigger.
		_, err := client.PermissionSet.Query().All(context.Background())
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("viewer-context is missing"))
	})
})
