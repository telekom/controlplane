// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

// Package naming works out the Kong names of a Route: the desired name and the
// ordered names under which it may still exist. It does no I/O.
package naming

import (
	"slices"

	"github.com/telekom/controlplane/gateway/pkg/kong/legacy"
)

// Separator joins the name prefix and metadata.name.
const Separator = "--"

// Names are the Kong names of a Route in lookup order.
type Names struct {
	// Desired is the name the Kong entities get: "<namePrefix>--<metadata.name>".
	Desired string
	// PreMigration is the name before migration: metadata.name.
	PreMigration string
	// PreMigrationOwnerTags are the owner tags of legacy candidates named like PreMigration.
	// They prove ownership of an entity found under PreMigration in addition to the CP tags.
	PreMigrationOwnerTags []string
	// Candidates are the legacy names, without Desired, PreMigration and duplicates.
	Candidates []legacy.Candidate
}

// NameScheme derives the Kong names of a Route from a legacy Migrator.
type NameScheme struct {
	migrator legacy.Migrator
}

// NewNameScheme returns a NameScheme. A nil Migrator yields no legacy candidates.
func NewNameScheme(migrator legacy.Migrator) NameScheme {
	if migrator == nil {
		migrator = legacy.None()
	}
	return NameScheme{migrator: migrator}
}

// DesiredName returns "<namePrefix>--<name>", or name if the prefix is empty.
func DesiredName(namePrefix, name string) string {
	if namePrefix == "" {
		return name
	}
	return namePrefix + Separator + name
}

// Names returns the desired name and the ordered candidate names of the route.
func (s NameScheme) Names(route legacy.Route) Names {
	names := Names{
		Desired:      DesiredName(route.NamePrefix, route.Name),
		PreMigration: route.Name,
	}
	seen := map[string]bool{names.Desired: true, names.PreMigration: true}
	for _, candidate := range s.migrator.Candidates(route) {
		if candidate.Name == names.PreMigration && !slices.Contains(names.PreMigrationOwnerTags, candidate.OwnerTag) {
			names.PreMigrationOwnerTags = append(names.PreMigrationOwnerTags, candidate.OwnerTag)
		}
		if seen[candidate.Name] {
			continue
		}
		seen[candidate.Name] = true
		names.Candidates = append(names.Candidates, candidate)
	}
	return names
}
