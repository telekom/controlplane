// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package decoder

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"regexp"
	"strings"

	"github.com/pkg/errors"
	"github.com/telekom/controlplane/tools/snapshotter/pkg/snapshot"
)

func DecodeBase64Content(s *snapshot.State, patterns ...string) error {
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
