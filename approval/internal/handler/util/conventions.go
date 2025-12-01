// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package util

import (
	"fmt"
	"github.com/pkg/errors"
	"strings"
)

const DELIMITER = "--"

// splitTeamName according to convention team name is <group>--<team>
func splitTeamName(teamName string) (string, string, error) {
	parts := strings.Split(teamName, DELIMITER)
	if len(parts) != 2 {
		return "", "", errors.New(fmt.Sprintf("TeamName is not according to convention <group>--<team>. %q", teamName))
	}
	return parts[0], parts[1], nil
}
