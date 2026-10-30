// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

// Package legacy knows how earlier operators named Kong entities.
// It has no dependency on the Kong client; the naming package consumes it.
package legacy

import (
	"errors"
	"fmt"
	"os"
	"regexp"

	"sigs.k8s.io/yaml"
)

// RealmTagPrefix is the prefix of the realm tag the Java operator stamped on its entities.
const RealmTagPrefix = "ei__telekom__de--realm---"

// Migrator returns the legacy Kong names under which a route may exist.
type Migrator interface {
	// Candidates returns the legacy Kong names under which the route may exist, in lookup order.
	Candidates(route Route) []Candidate
}

// Route is the part of a Route the Migrator needs.
type Route struct {
	Name       string            // metadata.name
	NamePrefix string            // spec.namePrefix
	Labels     map[string]string // metadata.labels
}

// Candidate is a legacy Kong name and the tag that proves ownership of it.
type Candidate struct {
	Name     string // Kong name of the route and service
	OwnerTag string // tag that proves ownership, e.g. "ei__telekom__de--realm---bond"
}

// Config is the Java operator naming configuration.
type Config struct {
	// DefaultRealm is the Java realm whose names carry no realm prefix.
	DefaultRealm string `json:"defaultRealm"`
	// Separator joins the Java realm and the base name.
	Separator string `json:"separator"`
	// RealmSuffixes are appended to the name prefix to build the Java realms that are tried.
	RealmSuffixes []string `json:"realmSuffixes"`
	// Names map metadata.name to the Java base name; the first matching rule wins.
	Names []NameRule `json:"names"`
}

// NameRule maps metadata.name of Routes whose labels match Selector to a Java base name.
type NameRule struct {
	Selector map[string]string `json:"selector"`
	Match    string            `json:"match"`
	Replace  string            `json:"replace"`
}

type compiledRule struct {
	selector map[string]string
	match    *regexp.Regexp
	replace  string
}

type configMigrator struct {
	defaultRealm  string
	separator     string
	realmSuffixes []string
	rules         []compiledRule
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

// NewMigrator builds a Migrator from a config. Rules with an invalid or empty regular
// expression are rejected.
func NewMigrator(cfg *Config) (Migrator, error) {
	m := &configMigrator{
		defaultRealm:  cfg.DefaultRealm,
		separator:     cfg.Separator,
		realmSuffixes: cfg.RealmSuffixes,
	}
	for i, rule := range cfg.Names {
		if rule.Match == "" {
			return nil, fmt.Errorf("legacy naming rule %d: %w", i, errors.New("match is empty"))
		}
		re, err := regexp.Compile(rule.Match)
		if err != nil {
			return nil, fmt.Errorf("legacy naming rule %d: compiling match: %w", i, err)
		}
		m.rules = append(m.rules, compiledRule{selector: rule.Selector, match: re, replace: rule.Replace})
	}
	return m, nil
}

// Candidates returns one candidate per realm suffix, without duplicates. A Route without
// name prefix has no candidates.
func (m *configMigrator) Candidates(route Route) []Candidate {
	if route.NamePrefix == "" {
		return nil
	}
	baseName := m.baseName(route)
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

func (m *configMigrator) baseName(route Route) string {
	for _, rule := range m.rules {
		if !selects(rule.selector, route.Labels) || !rule.match.MatchString(route.Name) {
			continue
		}
		return rule.match.ReplaceAllString(route.Name, rule.replace)
	}
	return route.Name
}

// selects reports whether every selector key is present in the labels with the same value.
func selects(selector, labels map[string]string) bool {
	for key, value := range selector {
		if actual, ok := labels[key]; !ok || actual != value {
			return false
		}
	}
	return true
}
