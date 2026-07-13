// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package mapper

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	gql "github.com/telekom/controlplane/organization-server/internal/graphql"
)

func ptr[T any](v T) *T { return &v }

func TestGroupToHubResponse(t *testing.T) {
	g := &gql.ListGroupsGroupsGroup{
		Name:        "eni",
		DisplayName: "Eni Group",
		Description: "The ENI group",
	}

	resp := GroupToHubResponse(g)

	if resp.Name != "eni" {
		t.Errorf("expected name=eni, got %s", resp.Name)
	}
	if resp.DisplayName != "Eni Group" {
		t.Errorf("expected displayName=Eni Group, got %s", resp.DisplayName)
	}
	if resp.Description != "The ENI group" {
		t.Errorf("expected description, got %s", resp.Description)
	}
}

func TestGroupDetailToHubResponse(t *testing.T) {
	g := &gql.GetGroupGroupsGroup{
		Name:        "cit",
		DisplayName: "CIT",
		Description: "CIT group",
	}

	resp := GroupDetailToHubResponse(g)

	if resp.Name != "cit" {
		t.Errorf("expected name=cit, got %s", resp.Name)
	}
	if resp.DisplayName != "CIT" {
		t.Errorf("expected displayName=CIT, got %s", resp.DisplayName)
	}
}

func TestTeamToTeamResponse(t *testing.T) {
	phase := gql.TeamStatusPhaseReady
	token := "my-token"
	now := time.Now()

	team := &gql.ListTeamsTeamsTeamConnectionEdgesTeamEdgeNodeTeam{
		Id:             "10",
		Name:           "eni--hyperion",
		Email:          "hyperion@test.de",
		CreatedAt:      now,
		LastModifiedAt: now,
		StatusPhase:    &phase,
		TeamToken:      &token,
		Members: []gql.ListTeamsTeamsTeamConnectionEdgesTeamEdgeNodeTeamMembersMember{
			{Name: "Alice", Email: "alice@test.de"},
			{Name: "Bob", Email: "bob@test.de"},
		},
	}

	resp := TeamToTeamResponse(team)

	if resp.Name != "eni--hyperion" {
		t.Errorf("expected name=eni--hyperion, got %s", resp.Name)
	}
	if resp.Email != "hyperion@test.de" {
		t.Errorf("expected email, got %s", resp.Email)
	}
	if resp.ClientId != "eni--hyperion--team-user" {
		t.Errorf("expected clientId=eni--hyperion--team-user, got %s", resp.ClientId)
	}
	if resp.TeamToken != "my-token" {
		t.Errorf("expected teamToken=my-token, got %s", resp.TeamToken)
	}
	if len(resp.Members) != 2 {
		t.Fatalf("expected 2 members, got %d", len(resp.Members))
	}
	if resp.Members[0].Name != "Alice" {
		t.Errorf("expected member Alice, got %s", resp.Members[0].Name)
	}
	if resp.Status.ProcessingState != "done" {
		t.Errorf("expected processingState=done for READY, got %s", resp.Status.ProcessingState)
	}
}

func TestTeamToTeamResponse_NilToken(t *testing.T) {
	team := &gql.ListTeamsTeamsTeamConnectionEdgesTeamEdgeNodeTeam{
		Name:  "eni--team",
		Email: "t@test.de",
	}

	resp := TeamToTeamResponse(team)

	if resp.TeamToken != "" {
		t.Errorf("expected empty teamToken, got %s", resp.TeamToken)
	}
}

func TestGetTeamToTeamResponse(t *testing.T) {
	phase := gql.TeamStatusPhasePending
	token := "get-token"

	team := &gql.GetTeamTeamsTeamConnectionEdgesTeamEdgeNodeTeam{
		Name:        "cit--sigma",
		Email:       "sigma@test.de",
		StatusPhase: &phase,
		TeamToken:   &token,
		Members: []gql.GetTeamTeamsTeamConnectionEdgesTeamEdgeNodeTeamMembersMember{
			{Name: "Charlie", Email: "charlie@test.de"},
		},
	}

	resp := GetTeamToTeamResponse(team)

	if resp.Name != "cit--sigma" {
		t.Errorf("expected name, got %s", resp.Name)
	}
	if resp.ClientId != "cit--sigma--team-user" {
		t.Errorf("expected clientId, got %s", resp.ClientId)
	}
	if resp.Status.ProcessingState != "pending" {
		t.Errorf("expected pending, got %s", resp.Status.ProcessingState)
	}
	if resp.TeamToken != "get-token" {
		t.Errorf("expected get-token, got %s", resp.TeamToken)
	}
}

func TestGetTeamToTeamResponse_NilToken(t *testing.T) {
	team := &gql.GetTeamTeamsTeamConnectionEdgesTeamEdgeNodeTeam{
		Name:  "x--y",
		Email: "y@test.de",
	}

	resp := GetTeamToTeamResponse(team)

	if resp.TeamToken != "" {
		t.Errorf("expected empty teamToken, got %s", resp.TeamToken)
	}
}

func TestMapStatusPhase(t *testing.T) {
	tests := []struct {
		name         string
		phase        *gql.TeamStatusPhase
		message      *string
		wantProc     string
		wantState    string
		wantErrCount int
	}{
		{
			name:     "nil phase",
			phase:    nil,
			wantProc: "none", wantState: "none",
		},
		{
			name:     "READY",
			phase:    ptr(gql.TeamStatusPhaseReady),
			wantProc: "done", wantState: "complete",
		},
		{
			name:     "PENDING",
			phase:    ptr(gql.TeamStatusPhasePending),
			wantProc: "pending", wantState: "none",
		},
		{
			name:         "ERROR with message",
			phase:        ptr(gql.TeamStatusPhaseError),
			message:      ptr("something broke"),
			wantProc:     "failed",
			wantState:    "blocked",
			wantErrCount: 1,
		},
		{
			name:     "ERROR without message",
			phase:    ptr(gql.TeamStatusPhaseError),
			wantProc: "failed", wantState: "blocked",
		},
		{
			name:     "ERROR with empty message",
			phase:    ptr(gql.TeamStatusPhaseError),
			message:  ptr(""),
			wantProc: "failed", wantState: "blocked",
		},
		{
			name:     "UNKNOWN",
			phase:    ptr(gql.TeamStatusPhaseUnknown),
			wantProc: "none", wantState: "none",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := mapStatusPhase(tt.phase, tt.message)
			if s.ProcessingState != tt.wantProc {
				t.Errorf("processingState: want %s, got %s", tt.wantProc, s.ProcessingState)
			}
			if s.State != tt.wantState {
				t.Errorf("state: want %s, got %s", tt.wantState, s.State)
			}
			if len(s.Errors) != tt.wantErrCount {
				t.Errorf("errors: want %d, got %d", tt.wantErrCount, len(s.Errors))
			}
		})
	}
}

func TestTeamStatusToResourceStatus(t *testing.T) {
	now := time.Now().UTC()
	modified := now.Add(5 * time.Minute)

	tests := []struct {
		name      string
		phase     *gql.TeamStatusPhase
		message   *string
		wantProc  string
		wantState string
		wantErr   int
	}{
		{
			name: "nil phase", phase: nil,
			wantProc: "none", wantState: "none",
		},
		{
			name: "READY", phase: ptr(gql.TeamStatusPhaseReady),
			wantProc: "done", wantState: "complete",
		},
		{
			name: "PENDING", phase: ptr(gql.TeamStatusPhasePending),
			wantProc: "pending", wantState: "none",
		},
		{
			name: "ERROR with message", phase: ptr(gql.TeamStatusPhaseError),
			message: ptr("failed hard"), wantProc: "failed", wantState: "blocked", wantErr: 1,
		},
		{
			name: "ERROR empty message", phase: ptr(gql.TeamStatusPhaseError),
			message: ptr(""), wantProc: "failed", wantState: "blocked",
		},
		{
			name: "ERROR nil message", phase: ptr(gql.TeamStatusPhaseError),
			wantProc: "failed", wantState: "blocked",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := TeamStatusToResourceStatus(tt.phase, tt.message, now, modified)
			if string(r.ProcessingState) != tt.wantProc {
				t.Errorf("processingState: want %s, got %s", tt.wantProc, r.ProcessingState)
			}
			if string(r.State) != tt.wantState {
				t.Errorf("state: want %s, got %s", tt.wantState, r.State)
			}
			if len(r.Errors) != tt.wantErr {
				t.Errorf("errors: want %d, got %d", tt.wantErr, len(r.Errors))
			}
			if !r.CreatedAt.Equal(now) {
				t.Errorf("createdAt: want %v, got %v", now, r.CreatedAt)
			}
			if !r.ProcessedAt.Equal(modified) {
				t.Errorf("processedAt: want %v, got %v", modified, r.ProcessedAt)
			}
		})
	}
}

func TestParsePagination(t *testing.T) {
	tests := []struct {
		name       string
		query      string
		wantOffset int
		wantLimit  int
	}{
		{"defaults", "", 0, 20},
		{"custom", "?offset=10&limit=5", 10, 5},
		{"negative offset clamped", "?offset=-5&limit=10", 0, 10},
		{"zero limit clamped to 1", "?offset=0&limit=0", 0, 1},
		{"limit above 20 clamped", "?offset=0&limit=50", 0, 20},
		{"negative limit clamped", "?offset=0&limit=-1", 0, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := fiber.New()
			var captured PaginationParams
			app.Get("/test", func(c *fiber.Ctx) error {
				captured = ParsePagination(c)
				return c.SendStatus(200)
			})

			req := httptest.NewRequest("GET", "/test"+tt.query, http.NoBody)
			_, _ = app.Test(req, -1)

			if captured.Offset != tt.wantOffset {
				t.Errorf("offset: want %d, got %d", tt.wantOffset, captured.Offset)
			}
			if captured.Limit != tt.wantLimit {
				t.Errorf("limit: want %d, got %d", tt.wantLimit, captured.Limit)
			}
		})
	}
}

func TestBuildPaginatedResponse(t *testing.T) {
	app := fiber.New()

	app.Get("/hubs", func(c *fiber.Ctx) error {
		items := []string{"a", "b", "c"}
		p := PaginationParams{Offset: 0, Limit: 2}
		result := BuildPaginatedResponse(c, items, 5, p)

		links := result["_links"].(fiber.Map)
		if links["self"] != "/hubs?offset=0&limit=2" {
			t.Errorf("self link: got %v", links["self"])
		}
		if links["first"] != "/hubs?offset=0&limit=2" {
			t.Errorf("first link: got %v", links["first"])
		}
		if links["last"] != "/hubs?offset=4&limit=2" {
			t.Errorf("last link: got %v", links["last"])
		}

		paging := result["paging"].(fiber.Map)
		if paging["total"] != 5 {
			t.Errorf("total: got %v", paging["total"])
		}
		if paging["page"] != 1 {
			t.Errorf("page: got %v", paging["page"])
		}
		if paging["last_page"] != 3 {
			t.Errorf("last_page: got %v", paging["last_page"])
		}
		return c.SendStatus(200)
	})

	req := httptest.NewRequest("GET", "/hubs", http.NoBody)
	_, _ = app.Test(req, -1)
}

func TestBuildPaginatedResponse_EmptyItems(t *testing.T) {
	app := fiber.New()

	app.Get("/empty", func(c *fiber.Ctx) error {
		p := PaginationParams{Offset: 0, Limit: 10}
		result := BuildPaginatedResponse(c, []string{}, 0, p)

		paging := result["paging"].(fiber.Map)
		if paging["last_page"] != 1 {
			t.Errorf("last_page for empty: expected 1, got %v", paging["last_page"])
		}
		return c.SendStatus(200)
	})

	req := httptest.NewRequest("GET", "/empty", http.NoBody)
	_, _ = app.Test(req, -1)
}
