// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package tree

import (
	"encoding/json"
	"maps"
	"slices"

	"gopkg.in/yaml.v3"
)

var (
	_ json.Unmarshaler = &Set{}
	_ yaml.Unmarshaler = &Set{}

	_ json.Marshaler = &Set{}
	_ yaml.Marshaler = &Set{}
)

type Set map[TreeResourceInfo]bool

// UnmarshalJSON implements [json.Unmarshaler].
func (s *Set) UnmarshalJSON(data []byte) error {
	l := []TreeResourceInfo{}
	*s = map[TreeResourceInfo]bool{}

	err := json.Unmarshal(data, &l)
	if err != nil {
		return err
	}

	for _, i := range l {
		(*s)[i] = true
	}

	return nil
}

// UnmarshalYAML implements [yaml.Unmarshaler].
func (s *Set) UnmarshalYAML(value *yaml.Node) error {
	var l []TreeResourceInfo
	*s = map[TreeResourceInfo]bool{}

	if err := value.Decode(&l); err != nil {
		return err
	}

	for _, i := range l {
		(*s)[i] = true
	}

	return nil
}

// MarshalJSON implements [json.Marshaler].
func (s Set) MarshalJSON() ([]byte, error) {
	l := slices.Collect(maps.Keys(s))
	return json.Marshal(l)
}

// MarshalYAML implements [yaml.Marshaler].
func (s Set) MarshalYAML() (any, error) {
	l := slices.Collect(maps.Keys(s))
	return l, nil
}
