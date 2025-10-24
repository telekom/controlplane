// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package obfuscator

import (
	"encoding/json"
	"regexp"

	"github.com/pkg/errors"
	"github.com/tidwall/sjson"
)

type ObfuscationTarget struct {
	Path    string `yaml:"path,omitempty" json:"path,omitempty"`
	Pattern string `yaml:"pattern,omitempty" json:"pattern,omitempty"`
	Replace string `yaml:"replace,omitempty" json:"replace,omitempty"`
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

func Obfuscate(o any, targets ...ObfuscationTarget) error {
	if len(targets) == 0 {
		return nil
	}

	b, err := json.Marshal(o)
	if err != nil {
		return errors.Wrap(err, "failed to marshal object to JSON")
	}
	b, err = ObfuscateBytes(b, targets...)
	if err != nil {
		return errors.Wrap(err, "failed to obfuscate object")
	}
	err = json.Unmarshal(b, &o)
	if err != nil {
		return errors.Wrap(err, "failed to unmarshal obfuscated object")
	}
	return nil
}
