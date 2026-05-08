// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package viewer

import (
	"context"

	"entgo.io/ent/privacy"
)

type viewerKey struct{}

type forwardedUserKey struct{}

// ForwardedUser holds the user identity extracted from X-Forwarded-User-* headers.
type ForwardedUser struct {
	Name  string
	Email string
}

// NewForwardedUserContext stores forwarded user identity in the context.
func NewForwardedUserContext(ctx context.Context, u ForwardedUser) context.Context {
	return context.WithValue(ctx, forwardedUserKey{}, u)
}

// ForwardedUserFromContext returns the forwarded user identity, if present.
func ForwardedUserFromContext(ctx context.Context) (ForwardedUser, bool) {
	u, ok := ctx.Value(forwardedUserKey{}).(ForwardedUser)
	return u, ok
}

// Viewer represents the authenticated user and their accessible teams.
type Viewer struct {
	Teams     []string
	Group     string // Group name from BusinessContext (set for group-level viewers)
	Admin     bool
	UserName  string // Display name from X-Forwarded-User-Name header
	UserEmail string // Email from X-Forwarded-User-Email header
}

// NewContext returns a new context with the viewer attached.
func NewContext(ctx context.Context, v *Viewer) context.Context {
	return context.WithValue(ctx, viewerKey{}, v)
}

// FromContext returns the Viewer from the context, or nil if not present.
func FromContext(ctx context.Context) *Viewer {
	v, _ := ctx.Value(viewerKey{}).(*Viewer)
	return v
}

// HasTeam checks if the viewer belongs to the specified team.
func (v *Viewer) HasTeam(teamName string) bool {
	if v == nil {
		return false
	}
	for _, t := range v.Teams {
		if t == teamName {
			return true
		}
	}
	return false
}

// SystemContext returns a context that bypasses all privacy rules.
func SystemContext(ctx context.Context) context.Context {
	return privacy.DecisionContext(ctx, privacy.Allow)
}
