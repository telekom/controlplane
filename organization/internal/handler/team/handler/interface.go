// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package handler

import (
	"context"

	organizationv1 "github.com/telekom/controlplane/organization/api/v1"
)

const Separator = "--"

type ObjectHandler interface {
	CreateOrUpdate(ctx context.Context, teamObj *organizationv1.Team) (err error)
	Delete(ctx context.Context, teamObj *organizationv1.Team) (err error)
	Identifier() string
}
