// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package parser

import (
	"fmt"
	"os"
	"regexp"
	"strings"
)

// placeholderRegex matches ${VAR_NAME} where VAR_NAME follows POSIX environment variable naming rules.
var placeholderRegex = regexp.MustCompile(`\$\{([a-zA-Z_][a-zA-Z0-9_]*)\}`)

// SubstitutePlaceholders replaces all ${VAR} occurrences in content with
// their corresponding environment variable values.
// Returns an error listing all unresolved variables if any are not set.
func SubstitutePlaceholders(content []byte) ([]byte, error) {
	seen := make(map[string]bool)
	var unresolved []string

	result := placeholderRegex.ReplaceAllFunc(content, func(match []byte) []byte {
		varName := string(placeholderRegex.FindSubmatch(match)[1])
		value, exists := os.LookupEnv(varName)
		if !exists {
			if !seen[varName] {
				seen[varName] = true
				unresolved = append(unresolved, varName)
			}
			return match
		}
		return []byte(value)
	})

	if len(unresolved) > 0 {
		return nil, fmt.Errorf("unresolved environment variable(s): %s", strings.Join(unresolved, ", "))
	}

	return result, nil
}
