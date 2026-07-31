// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package parser

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/goccy/go-yaml/lexer"
	"github.com/goccy/go-yaml/token"
)

// placeholderRegex matches ${VAR_NAME} where VAR_NAME follows POSIX environment variable naming rules.
var placeholderRegex = regexp.MustCompile(`\$\{([a-zA-Z_][a-zA-Z0-9_]*)\}`)

// SubstitutePlaceholders replaces all ${VAR} occurrences in content with
// their corresponding environment variable values.
// Placeholders inside YAML comments are ignored (left untouched).
// Returns an error listing all unresolved variables if any are not set.
func SubstitutePlaceholders(content string) (string, error) {
	seen := make(map[string]bool)
	var unresolved []string

	replace := func(segment string) string {
		return placeholderRegex.ReplaceAllStringFunc(segment, func(match string) string {
			varName := placeholderRegex.FindStringSubmatch(match)[1]
			value, exists := os.LookupEnv(varName)
			if !exists {
				if !seen[varName] {
					seen[varName] = true
					unresolved = append(unresolved, varName)
				}
				return match
			}
			return value
		})
	}

	var result strings.Builder
	for _, tk := range lexer.Tokenize(content) {
		origin := tk.Origin
		if tk.Type != token.CommentType {
			origin = replace(origin)
		}
		result.WriteString(origin)
	}

	if len(unresolved) > 0 {
		return "", fmt.Errorf("unresolved environment variable(s): %s", strings.Join(unresolved, ", "))
	}

	return result.String(), nil
}
