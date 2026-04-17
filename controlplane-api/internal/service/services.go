// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package service

// Services groups all mutation services.
type Services struct {
	Team        TeamService
	Application ApplicationService
	Approval    ApprovalService
}
