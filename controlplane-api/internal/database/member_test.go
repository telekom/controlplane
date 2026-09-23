// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package database_test

import (
	"context"
	"testing"

	"github.com/telekom/controlplane/controlplane-api/ent"
	"github.com/telekom/controlplane/controlplane-api/ent/member"
	"github.com/telekom/controlplane/controlplane-api/internal/testutil"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestMembers(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Member Persistence Suite")
}

var _ = Describe("Member persistence", func() {
	var (
		client *ent.Client
		ctx    context.Context
		team   *ent.Team
	)

	BeforeEach(func() {
		client = testutil.NewTestClient(GinkgoT())
		DeferCleanup(func() { Expect(client.Close()).To(Succeed()) })
		ctx = testutil.AllowContext()
		var err error
		team, err = client.Team.Create().SetName("team-alpha").SetNamespace("default").
			SetEmail("Contact@Example.com").Save(ctx)
		Expect(err).NotTo(HaveOccurred())
	})

	It("stores supplied casing on direct writes; normalization belongs to admission", func() {
		m, err := client.Member.Create().SetName("Alice").SetEmail("Alice@Example.COM").SetTeam(team).Save(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(m.Email).To(Equal("Alice@Example.COM"))
		Expect(client.Member.UpdateOneID(m.ID).SetEmail("Alice.New@Example.COM").Exec(ctx)).To(Succeed())
		stored, err := client.Member.Get(ctx, m.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(stored.Email).To(Equal("Alice.New@Example.COM"))
		Expect(client.Member.Update().Where(member.IDEQ(m.ID)).SetEmail("Alice.Bulk@Example.COM").Exec(ctx)).To(Succeed())
		Expect(client.Member.UpdateOneID(m.ID).SetName("Alice Renamed").Exec(ctx)).To(Succeed())
		stored, err = client.Member.Get(ctx, m.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(stored.Email).To(Equal("Alice.Bulk@Example.COM"))
		storedTeam, err := client.Team.Get(ctx, team.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(storedTeam.Email).To(Equal("Contact@Example.com"))
	})

	It("enforces email uniqueness within a team", func() {
		_, err := client.Member.CreateBulk(
			client.Member.Create().SetName("Alice").SetEmail("alice@example.com").SetTeam(team),
			client.Member.Create().SetName("Bob").SetEmail("bob@example.com").SetTeam(team),
		).Save(ctx)
		Expect(err).NotTo(HaveOccurred())
		emails, err := client.Member.Query().Select(member.FieldEmail).Strings(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(emails).To(ConsistOf("alice@example.com", "bob@example.com"))
		err = client.Member.Update().Where(member.EmailEQ("bob@example.com")).SetEmail("alice@example.com").Exec(ctx)
		Expect(ent.IsConstraintError(err)).To(BeTrue())
		err = client.Member.Create().SetName("Duplicate").SetEmail("alice@example.com").SetTeam(team).Exec(ctx)
		Expect(ent.IsConstraintError(err)).To(BeTrue())
	})

	DescribeTable("updates the member in place on an email and team conflict",
		func(mode string) {
			upsert := func(email, name string) int {
				builder := client.Member.Create().SetName(name).SetEmail(email).SetTeam(team).
					OnConflictColumns(member.FieldEmail, member.TeamColumn)
				switch mode {
				case "projector":
					builder.Update(func(u *ent.MemberUpsert) { u.SetName(name) })
				case "new values":
					builder.UpdateNewValues()
				case "excluded email":
					builder.Update(func(u *ent.MemberUpsert) { u.UpdateEmail().UpdateName() })
				}
				id, err := builder.ID(ctx)
				Expect(err).NotTo(HaveOccurred())
				return id
			}
			id := upsert("alice@example.com", "Alice")
			Expect(upsert("alice@example.com", "Alice Updated")).To(Equal(id))
			stored, err := client.Member.Query().Only(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(stored.Email).To(Equal("alice@example.com"))
			Expect(stored.Name).To(Equal("Alice Updated"))
		},
		Entry("projector name-only conflict update", "projector"),
		Entry("UpdateNewValues", "new values"),
		Entry("UpdateEmail from excluded insert values", "excluded email"),
	)

	It("requires nonempty emails on create and update", func() {
		Expect(client.Member.Create().SetName("Invalid").SetEmail("").SetTeam(team).Exec(ctx)).NotTo(Succeed())
		m, err := client.Member.Create().SetName("Valid").SetEmail("valid@example.com").SetTeam(team).Save(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(client.Member.UpdateOne(m).SetEmail("").Exec(ctx)).NotTo(Succeed())
		stored, err := client.Member.Get(ctx, m.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(stored.Email).To(Equal("valid@example.com"))
	})
})
