// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package state

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"

	"github.com/goccy/go-yaml"
	"github.com/pkg/errors"
	kong "github.com/telekom/controlplane/gateway/pkg/kong/api"
	"github.com/telekom/controlplane/tools/snapshotter/util"
	"github.com/tidwall/sjson"
)

type RouteState struct {
	Environment  string `yaml:"environment" json:"environment"`
	Zone         string `yaml:"zone" json:"zone"`
	RouteName    string `yaml:"route_name,omitempty" json:"route_name,omitempty"`
	ConsumerName string `yaml:"consumer_name,omitempty" json:"consumer_name,omitempty"`

	Service  *kong.Service  `yaml:"service" json:"service"`
	Route    *kong.Route    `yaml:"route" json:"route"`
	Plugins  []kong.Plugin  `yaml:"plugins" json:"plugins"`
	Consumer *kong.Consumer `yaml:"consumer,omitempty" json:"consumer,omitempty"`
}

type ObfuscationTarget struct {
	Path    string
	Pattern string
	Replace string
}

func (s *RouteState) Print(w io.Writer) error {
	err := yaml.NewEncoder(w).Encode(s)
	if err != nil {
		return errors.Wrap(err, "failed to encode route state to YAML")
	}
	return nil
}

func (s *RouteState) String() string {
	util.DeepSort(s)
	data, err := yaml.Marshal(s)
	if err != nil {
		panic(fmt.Sprintf("Failed to marshal route state: %v", err))
	}
	return string(data)
}

func (s *RouteState) Write(filename string) error {
	file, err := os.OpenFile(filename, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return errors.Wrapf(err, "failed to open file %s for writing", filename)
	}
	defer file.Close() //nolint:errcheck

	_, err = io.WriteString(file, s.String())
	if err != nil {
		return errors.Wrapf(err, "failed to write route state to file %s", filename)
	}
	return nil
}

func Obfuscate(s *RouteState, targets ...ObfuscationTarget) error {
	b, err := json.Marshal(s)
	if err != nil {
		return errors.Wrap(err, "failed to marshal route state to JSON")
	}
	b, err = ObfuscateBytes(b, targets...)
	if err != nil {
		return errors.Wrap(err, "failed to obfuscate route state")
	}
	err = json.Unmarshal(b, &s)
	if err != nil {
		return errors.Wrap(err, "failed to unmarshal obfuscated route state")
	}
	return nil
}

func ObfuscateBytes(b []byte, targets ...ObfuscationTarget) ([]byte, error) {
	var err error

	for _, target := range targets {
		if len(target.Path) > 0 {
			// Use sjson to set the value at the specified path
			b, err = sjson.SetRawBytes(b, target.Path, []byte(target.Replace))
			if err != nil {
				return nil, errors.Wrapf(err, "failed to set value at path %s", target.Path)
			}
		}
		if len(target.Pattern) > 0 {
			// Use regex to replace the pattern in the JSON string
			re := regexp.MustCompile(target.Pattern)
			b = re.ReplaceAll(b, []byte(target.Replace))
		}
	}
	return b, nil
}

func DecodeBase64Content(s *RouteState, patterns ...string) error {
	for _, plugin := range s.Plugins {
		b, err := json.Marshal(plugin)
		if err != nil {
			return errors.Wrap(err, "failed to marshal plugin to JSON")
		}

		b, err = DecodeBase64ContentBytes(b, patterns...)
		if err != nil {
			return errors.Wrapf(err, "failed to decode base64 content for plugin %s", *plugin.Name)
		}
		err = json.Unmarshal(b, &plugin)
		if err != nil {
			return errors.Wrap(err, "failed to unmarshal plugin with decoded base64 content")
		}
	}
	return nil
}

// DecodeBase64ContentBytes decodes base64-encoded content in the plugin's JSON representation
// that matches the provided patterns. It modifies the plugin in place.
// The patterns must be regular expressions that capture the base64 content in a group.
func DecodeBase64ContentBytes(b []byte, patterns ...string) ([]byte, error) {
	for _, pattern := range patterns {
		re := regexp.MustCompile(pattern)
		matches := re.FindAllStringSubmatch(string(b), -1)

		for _, match := range matches {
			if len(match) < 2 {
				continue
			}
			encodedContent := match[1]
			decodedContent, err := base64.StdEncoding.DecodeString(encodedContent)
			if err != nil {
				return nil, errors.Wrapf(err, "failed to decode base64 content for pattern %s", pattern)
			}

			// Escape quotes in the decoded content
			decodedContent = bytes.ReplaceAll(decodedContent, []byte(`"`), []byte(`\"`))
			// Merge the decoded content back into the original JSON string.
			before, _, _ := strings.Cut(pattern, "(")
			newValue := before + string(decodedContent)
			b = re.ReplaceAll(b, []byte(newValue))
		}
	}
	return b, nil
}
