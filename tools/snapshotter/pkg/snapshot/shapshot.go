// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package snapshot

import (
	"cmp"
	"fmt"
	"path/filepath"
	"slices"

	"github.com/goccy/go-yaml"
	kong "github.com/telekom/controlplane/gateway/pkg/kong/api"
	"github.com/telekom/controlplane/tools/snapshotter/pkg/util"
)

type State struct {
	RouteName    string `yaml:"route_name,omitempty" json:"route_name,omitempty"`
	ConsumerName string `yaml:"consumer_name,omitempty" json:"consumer_name,omitempty"`

	Service  *kong.Service  `yaml:"service,omitempty" json:"service,omitempty"`
	Route    *kong.Route    `yaml:"route,omitempty" json:"route,omitempty"`
	Plugins  []kong.Plugin  `yaml:"plugins,omitempty" json:"plugins,omitempty"`
	Consumer *kong.Consumer `yaml:"consumer,omitempty" json:"consumer,omitempty"`
	Upstream *kong.Upstream `yaml:"upstream,omitempty" json:"upstream,omitempty"`
	Targets  []kong.Target  `yaml:"targets,omitempty" json:"targets,omitempty"`
}

func (s *State) SortPlugins() {
	slices.SortFunc(s.Plugins, func(a, b kong.Plugin) int {
		return cmp.Compare(*a.Name, *b.Name)
	})
}

type Snapshot struct {
	Environment string `yaml:"environment" json:"environment"`
	Zone        string `yaml:"zone" json:"zone"`
	ID          string `yaml:"id" json:"id"`
	State       *State `yaml:"state" json:"state"`
}

func (s *Snapshot) String() string {
	util.DeepSort(s.State)
	s.State.SortPlugins()
	data, err := yaml.Marshal(s.State)
	if err != nil {
		panic(fmt.Sprintf("Failed to marshal route state: %v", err))
	}
	return string(data)
}

func Unmarshal(data []byte) (*Snapshot, error) {
	var snap Snapshot
	if err := yaml.Unmarshal(data, &snap); err != nil {
		return nil, err
	}
	util.DeepSort(snap.State)
	snap.State.SortPlugins()
	return &snap, nil
}

func (s *Snapshot) Path() string {
	return MakePath(s.Environment, s.Zone, s.ID)
}

func MakePath(env, zone, id string) string {
	return filepath.Join(env, zone, id)
}
