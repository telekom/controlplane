// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package resolvers

import (
	"github.com/telekom/controlplane/controlplane-api/ent"
	"github.com/telekom/controlplane/controlplane-api/ent/zone"
	"github.com/telekom/controlplane/controlplane-api/pkg/model"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("mapApplicationInfo", func() {
	It("should map an application together with zone and owning team", func() {
		ids := []model.ExternalID{{ID: "z-123", Scheme: "inventory"}, {ID: "a-456", Scheme: "catalog"}}
		app := &ent.Application{ID: 3, Name: "app-alpha", ExternalIDs: ids}
		z := &ent.Zone{ID: 7, Name: "zone-eu", Visibility: zone.VisibilityEnterprise}
		team := &ent.Team{ID: 11, Name: "team-alpha", Email: "alpha@test.dev"}
		group := &ent.Group{ID: 13, Name: "group-a"}

		info := mapApplicationInfo(app, z, team, group)
		Expect(info).NotTo(BeNil())
		Expect(info.ID).To(Equal(3))
		Expect(info.Name).To(Equal("app-alpha"))
		Expect(info.ExternalIDs).To(Equal(ids))
		Expect(info.Zone).To(BeIdenticalTo(z))
		Expect(info.OwnerTeam).NotTo(BeNil())
		Expect(info.OwnerTeam.Name).To(Equal("team-alpha"))
		Expect(info.OwnerTeam.GroupName).To(Equal("group-a"))
	})

	It("should return nil for a nil application", func() {
		Expect(mapApplicationInfo(nil, nil, nil, nil)).To(BeNil())
	})
})
