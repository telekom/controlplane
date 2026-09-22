// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

// Package emailutil defines the shared email syntax and member identity policy.
package emailutil

import (
	"fmt"
	"net/mail"
	"unicode"
)

// Canonicalize folds only ASCII A-Z, matching CEL lowerAscii. It does not trim,
// normalize Unicode, or rewrite mailbox dots, tags, or quoting.
func Canonicalize(email string) string {
	b := []byte(email)
	for i, c := range b {
		if c >= 'A' && c <= 'Z' {
			b[i] = c + ('a' - 'A')
		}
	}
	return string(b)
}

// Validate accepts a bare addr-spec, including quoted mailboxes and Unicode.
// It checks syntax, not deliverability or the receiving system's SMTPUTF8 support.
func Validate(email string) error {
	// net/mail accepts display names and comments. Exclude those outside quoted
	// mailboxes, while leaving punctuation and quoted spaces to its syntax parser.
	quoted, escaped := false, false
	for _, c := range email {
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
		if !quoted && (unicode.IsSpace(c) || c == '(' || c == ')' || c == '<' || c == '>') {
			return fmt.Errorf("must be a bare email address without display names, comments, or outer whitespace")
		}
	}
	// Wrapping forces addr-spec parsing, excluding address lists and groups.
	if _, err := mail.ParseAddress("<" + email + ">"); err != nil {
		return fmt.Errorf("invalid bare email address: %w", err)
	}
	return nil
}
