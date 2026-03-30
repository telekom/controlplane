// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package testutil

import (
	"github.com/telekom/controlplane/controlplane-api/ent"
	"github.com/telekom/controlplane/controlplane-api/internal/resolvers/model"
)

// SeedData holds references to all entities created by SeedStandard.
type SeedData struct {
	ZoneEU *ent.Zone
	EnvDev *ent.Environment

	GroupA *ent.Group
	GroupB *ent.Group

	TeamAlpha *ent.Team
	TeamBeta  *ent.Team

	AppAlpha *ent.Application
	AppBeta  *ent.Application

	ExposureAlpha *ent.ApiExposure
	ExposureBeta  *ent.ApiExposure

	Subscription *ent.ApiSubscription

	Approval        *ent.Approval
	ApprovalRequest *ent.ApprovalRequest

	MemberAlpha *ent.Member
	MemberBeta  *ent.Member

	TeamEnvAlpha *ent.TeamEnvironment
	TeamEnvBeta  *ent.TeamEnvironment
}

// SeedStandard creates a standard set of test data covering all entity types.
// It uses AllowContext to bypass privacy rules during seeding.
func SeedStandard(client *ent.Client) *SeedData {
	ctx := AllowContext()
	s := &SeedData{}

	// Public reference data
	s.ZoneEU = must(client.Zone.Create().SetName("zone-eu").Save(ctx))
	s.EnvDev = must(client.Environment.Create().SetName("env-dev").Save(ctx))
	s.GroupA = must(client.Group.Create().SetName("group-a").SetDisplayName("Group A").Save(ctx))
	s.GroupB = must(client.Group.Create().SetName("group-b").SetDisplayName("Group B").Save(ctx))

	// Teams
	s.TeamAlpha = must(client.Team.Create().
		SetName("team-alpha").SetEmail("alpha@test.dev").SetGroup(s.GroupA).Save(ctx))
	s.TeamBeta = must(client.Team.Create().
		SetName("team-beta").SetEmail("beta@test.dev").SetGroup(s.GroupB).Save(ctx))

	// Members
	s.MemberAlpha = must(client.Member.Create().
		SetName("Alice Alpha").SetEmail("alice@test.dev").SetTeam(s.TeamAlpha).Save(ctx))
	s.MemberBeta = must(client.Member.Create().
		SetName("Bob Beta").SetEmail("bob@test.dev").SetTeam(s.TeamBeta).Save(ctx))

	// TeamEnvironments
	s.TeamEnvAlpha = must(client.TeamEnvironment.Create().
		SetTeam(s.TeamAlpha).SetEnvironment(s.EnvDev).Save(ctx))
	s.TeamEnvBeta = must(client.TeamEnvironment.Create().
		SetTeam(s.TeamBeta).SetEnvironment(s.EnvDev).Save(ctx))

	// Applications
	s.AppAlpha = must(client.Application.Create().
		SetName("app-alpha").SetClientID("client-alpha").
		SetOwnerTeam(s.TeamAlpha).SetZone(s.ZoneEU).Save(ctx))
	s.AppBeta = must(client.Application.Create().
		SetName("app-beta").SetClientID("client-beta").
		SetOwnerTeam(s.TeamBeta).SetZone(s.ZoneEU).Save(ctx))

	// API Exposures
	s.ExposureAlpha = must(client.ApiExposure.Create().
		SetBasePath("/alpha").SetOwner(s.AppAlpha).Save(ctx))
	s.ExposureBeta = must(client.ApiExposure.Create().
		SetBasePath("/beta").SetOwner(s.AppBeta).Save(ctx))

	// Subscription: app-beta subscribes to exposure-alpha (cross-team)
	s.Subscription = must(client.ApiSubscription.Create().
		SetBasePath("/alpha").
		SetOwner(s.AppBeta).
		SetTarget(s.ExposureAlpha).
		Save(ctx))

	// Approval + ApprovalRequest on that subscription
	s.Approval = must(client.Approval.Create().
		SetAction("ALLOW").
		SetRequester(model.RequesterInfo{TeamName: "team-beta"}).
		SetDecider(model.DeciderInfo{TeamName: "team-alpha"}).
		SetAPISubscription(s.Subscription).
		Save(ctx))
	s.ApprovalRequest = must(client.ApprovalRequest.Create().
		SetAction("ALLOW").
		SetRequester(model.RequesterInfo{TeamName: "team-beta"}).
		SetDecider(model.DeciderInfo{TeamName: "team-alpha"}).
		SetAPISubscription(s.Subscription).
		Save(ctx))

	return s
}

func must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}
