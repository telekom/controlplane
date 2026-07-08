// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package handler

import (
	"context"
	"strconv"

	"github.com/gofiber/fiber/v2"

	"github.com/telekom/controlplane/organization-server/internal/api"
	"github.com/telekom/controlplane/organization-server/internal/client"
	gql "github.com/telekom/controlplane/organization-server/internal/graphql"
	mw "github.com/telekom/controlplane/organization-server/internal/middleware"
)

// contextWithIdentity builds a context.Context with the consumer's identity
// for forwarding to CP API via genqlient.
func (h *Handler) contextWithIdentity(c *fiber.Ctx) context.Context {
	id := mw.ConsumerIdentityFromContext(c)
	ctx := c.UserContext()
	if id != nil {
		ctx = client.WithIdentity(ctx, &client.ConsumerIdentity{
			Environment: h.environment,
			Group:       id.Group,
			Team:        id.Team,
		})
	}
	return ctx
}

// MutationError is a common representation for mapping genqlient mutation errors.
type MutationError struct {
	Code    string
	Message string
}

// mutationErrorLike is implemented by all genqlient mutation error types.
type mutationErrorLike interface {
	GetCode() gql.ErrorCode
	GetMessage() string
}

// toMutationErrors converts any slice of genqlient error structs to common MutationErrors.
func toMutationErrors[T any, PT interface {
	*T
	mutationErrorLike
}](errors []T) []MutationError {
	result := make([]MutationError, len(errors))
	for i := range errors {
		p := PT(&errors[i])
		result[i] = MutationError{
			Code:    string(p.GetCode()),
			Message: p.GetMessage(),
		}
	}
	return result
}

// mapMutationErrors translates mutation errors to an HTTP error response.
func (h *Handler) mapMutationErrors(c *fiber.Ctx, errors []MutationError) error {
	if len(errors) == 0 {
		return nil
	}

	e := errors[0]
	status := fiber.StatusInternalServerError
	switch e.Code {
	case "FORBIDDEN":
		status = fiber.StatusForbidden
	case "NOT_FOUND":
		status = fiber.StatusNotFound
	case "CONFLICT", "ALREADY_EXISTS":
		status = fiber.StatusConflict
	case "PRECONDITION_FAILED":
		status = fiber.StatusPreconditionFailed
	case "BAD_REQUEST", "VALIDATION_FAILED":
		status = fiber.StatusBadRequest
	}

	return c.Status(status).JSON(api.Error{
		Type:   "about:blank",
		Title:  e.Code,
		Status: float32(status),
		Detail: e.Message,
	})
}

func ptr[T any](v T) *T {
	return &v
}

func ptrOr[T any](p *T, def T) T {
	if p != nil {
		return *p
	}
	return def
}

func intToStr(i int) string {
	return strconv.Itoa(i)
}

// resolveGroupID looks up a group by name and returns its ent ID.
func (h *Handler) resolveGroupID(ctx context.Context, name string) (string, error) {
	resp, err := gql.GetGroup(ctx, h.cpapi, &gql.GroupWhereInput{
		Name: &name,
	})
	if err != nil {
		return "", err
	}
	if len(resp.Groups) == 0 {
		return "", nil
	}
	return resp.Groups[0].Id, nil
}

// resolveTeamID looks up a team by hub name + team name and returns its ent ID.
func (h *Handler) resolveTeamID(ctx context.Context, hubName, teamName string) (string, error) {
	fullTeamName := hubName + "--" + teamName
	resp, err := gql.GetTeam(ctx, h.cpapi, &gql.TeamWhereInput{
		Name:         &fullTeamName,
		HasGroupWith: []gql.GroupWhereInput{{Name: &hubName}},
	})
	if err != nil {
		return "", err
	}
	if len(resp.Teams.Edges) == 0 {
		return "", nil
	}
	return resp.Teams.Edges[0].Node.Id, nil
}
