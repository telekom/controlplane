// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package decoder

import (
	"fmt"

	"github.com/telekom/controlplane/tools/snapshotter/pkg/snapshot"
)

type DecoderTarget struct {
	Type    string `yaml:"type" json:"type" validate:"required,oneof=base64"`
	Pattern string `yaml:"pattern" json:"pattern" validate:"required"`
}

func Decode(s *snapshot.State, targets []DecoderTarget) error {
	for _, target := range targets {
		switch target.Type {
		case "base64":
			if err := DecodeBase64Content(s, target.Pattern); err != nil {
				return err
			}
		default:
			return fmt.Errorf("unsupported decoder type: %s", target.Type)
		}
	}
	return nil
}
