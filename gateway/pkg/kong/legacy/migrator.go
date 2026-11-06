// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

// Package legacy knows how earlier operators named Kong entities. The legacy base name
// is the Route's metadata.name; Routes that must adopt legacy entities carry the legacy-compatible base name.
// It has no dependency on the Kong client; the naming package consumes it.
package legacy

import (
	"fmt"
	"os"

	"sigs.k8s.io/yaml"
)

// RealmTagPrefix is the prefix of the realm tag the legacy operator stamped on its entities.
const RealmTagPrefix = "ei__telekom__de--realm---"

// Migrator returns the legacy Kong names under which a route may exist.
type Migrator interface {
	// Candidates returns the legacy Kong names under which the route may exist, in lookup order.
	Candidates(route Route) []Candidate
}

// Route is the part of a Route the Migrator needs.
type Route struct {
	Name       string // metadata.name, also the legacy-compatible base name
	NamePrefix string // spec.namePrefix
}

// Candidate is a legacy Kong name and the tag that proves ownership of it.
type Candidate struct {
	Name     string // Kong name of the route and service
	OwnerTag string // tag that proves ownership, e.g. "ei__telekom__de--realm---bond"
}

// Config is the legacy naming configuration.
type Config struct {
	// DefaultRealm is the legacy realm whose names carry no realm prefix.
	DefaultRealm string `json:"defaultRealm"`
	// Separator joins the legacy realm and the base name.
	Separator string `json:"separator"`
	// RealmSuffixes are appended to the name prefix to build the legacy realms that are tried.
	RealmSuffixes []string `json:"realmSuffixes"`
}

type configMigrator struct {
	defaultRealm  string
	separator     string
	realmSuffixes []string
}

type noMigrator struct{}

func (noMigrator) Candidates(Route) []Candidate { return nil }

// None returns a Migrator without legacy candidates.
func None() Migrator { return noMigrator{} }

// LoadMigrator reads and validates the config file at path. An empty path gives a Migrator
// without legacy candidates.
func LoadMigrator(path string) (Migrator, error) {
	if path == "" {
		return None(), nil
	}
	data, err := os.ReadFile(path) //nolint:gosec // path is an operator flag
	if err != nil {
		return nil, fmt.Errorf("reading legacy naming config: %w", err)
	}
	cfg := Config{}
	if err := yaml.UnmarshalStrict(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing legacy naming config: %w", err)
	}
	return NewMigrator(&cfg)
}

// NewMigrator builds a Migrator from a config.
func NewMigrator(cfg *Config) (Migrator, error) {
	return &configMigrator{
		defaultRealm:  cfg.DefaultRealm,
		separator:     cfg.Separator,
		realmSuffixes: cfg.RealmSuffixes,
	}, nil
}

// Candidates returns one candidate per realm suffix, without duplicates. A Route without
// name prefix has no candidates.
func (m *configMigrator) Candidates(route Route) []Candidate {
	if route.NamePrefix == "" {
		return nil
	}
	baseName := route.Name
	var candidates []Candidate
	seen := map[string]bool{}
	for _, suffix := range m.realmSuffixes {
		javaRealm := route.NamePrefix + suffix
		name := javaRealm + m.separator + baseName
		if javaRealm == m.defaultRealm {
			name = baseName
		}
		if seen[name] {
			continue
		}
		seen[name] = true
		candidates = append(candidates, Candidate{Name: name, OwnerTag: RealmTagPrefix + javaRealm})
	}
	return candidates
}
