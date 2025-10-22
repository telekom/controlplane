// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package labelutil

import (
	"strings"

	"github.com/telekom/controlplane/common/pkg/util/hash"
)

const (
	MaxLabelLength  = 63  // see https://kubernetes.io/docs/concepts/overview/working-with-objects/names/#dns-label-names
	MaxNameLength   = 253 // see https://kubernetes.io/docs/concepts/overview/working-with-objects/names/#dns-subdomain-names
	StartCharacters = 16
	EndCharacters   = 16
)

var (
	// " " -> "-", "_" -> "-", "/" -> "-", "\" -> "-"
	unwantedChars = []string{" ", "-", "_", "-", "/", "-", "\\", "-"}
	replacer      = strings.NewReplacer(unwantedChars...)
)

// NormalizeValue normalizes the given value by replacing unwanted characters with hyphens,
// converting to lowercase, and trimming leading/trailing hyphens.
func NormalizeValue(value string) string {
	value = strings.ToLower(replacer.Replace(value))
	return strings.Trim(value, "-")
}

// NormalizeLabelValue normalizes and shortens the given label value to fit within MaxLabelLength.
func NormalizeLabelValue(value string) string {
	normalizedValue := NormalizeValue(value)
	_, normalizedValue = ShortenValue(normalizedValue, MaxLabelLength)
	return normalizedValue
}

// NormalizeNameValue normalizes and shortens the given name value to fit within MaxNameLength.
func NormalizeNameValue(value string) string {
	normalizedValue := NormalizeValue(value)
	_, normalizedValue = ShortenValue(normalizedValue, MaxNameLength)
	return normalizedValue
}

// ShortenValue shortens the given value to fit within maxLen by keeping the first and last parts
// and replacing the middle part with a hash if necessary.
// Returns true if the value was shortened, false otherwise.
func ShortenValue(value string, maxLen int) (bool, string) {
	if maxLen < StartCharacters+EndCharacters {
		panic("maxLen is too small to shorten the value")
	}

	if len(value) <= maxLen {
		return false, value
	}
	first := value[:StartCharacters]
	last := value[len(value)-EndCharacters:]
	middle := hash.ComputeHash(value, nil)
	allowedMiddleLen := min(maxLen-StartCharacters-EndCharacters, len(middle))
	return true, first + middle[:allowedMiddleLen] + last
}
