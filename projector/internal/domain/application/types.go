// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

// Package application implements the Application resource module for the projector
// operator. Application is a Level 2 entity with required FK dependencies
// on both Team and Zone.
package application

import "github.com/telekom/controlplane/projector/internal/domain/shared"

// ApplicationKey is the composite identity key for Application entities.
// Application names are only unique per team (composite unique index on
// name + owner_team), so both Name and TeamName are needed.
type ApplicationKey struct {
	Name     string
	TeamName string
}

// ApplicationData carries the transformed data for an Application entity.
type ApplicationData struct {
	Meta          shared.Metadata
	StatusPhase   string // "READY", "PENDING", "ERROR", "UNKNOWN"
	StatusMessage string
	Name          string
	ClientID      *string // optional/nillable — nil when Status.ClientId is empty
	ClientSecret  *string // optional/nillable — nil when Spec.Secret is empty
	IssuerURL     *string // optional/nillable — always nil (CR does not carry it)
	TeamName      string  // resolved to owner_team FK
	ZoneName      string  // resolved to zone FK
}
