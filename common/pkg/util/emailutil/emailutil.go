// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

// Package emailutil defines the shared email syntax and member identity policy.
package emailutil

import (
	"fmt"
	"net/mail"
	"strings"
	"unicode"
)

const unwantedBareAddressCharacters = "()<>"

// Canonicalize applies Unicode lowercase, not full case folding. It does not trim,
// normalize Unicode, apply IDNA, or rewrite mailbox dots, tags, or quoting.
func Canonicalize(email string) string {
	return strings.ToLower(email)
}

// Validate accepts a bare addr-spec, including quoted mailboxes and Unicode.
// It checks syntax, not deliverability or the receiving system's SMTPUTF8 support.
func Validate(email string) error {
	// net/mail accepts display names and comments. Exclude those outside quoted
	// mailboxes, while leaving punctuation and quoted spaces to its syntax parser.
	quoted, escaped := false, false
	for _, c := range email {
		// net/mail permits quoted tabs and non-ASCII control characters.
		if unicode.IsControl(c) {
			return fmt.Errorf("email address must not contain control characters")
		}
		if escaped {
			escaped = false
			continue
		}
		if quoted && c == '\\' {
			escaped = true
			continue
		}
		if c == '"' {
			quoted = !quoted
			continue
		}
		if !quoted && (unicode.IsSpace(c) || strings.ContainsRune(unwantedBareAddressCharacters, c)) {
			return fmt.Errorf("must be a bare email address without display names, comments, or outer whitespace")
		}
	}
	// Wrapping forces addr-spec parsing, excluding address lists and groups.
	if _, err := mail.ParseAddress("<" + email + ">"); err != nil {
		return fmt.Errorf("invalid bare email address: %w", err)
	}
	return nil
}
