// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package common

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/telekom/controlplane/rover-ctl/pkg/types"
)

// The main validation is done on server-side, but we can already do some basic checks here
// to provide faster feedback to the user and provide better error messages.

const (
	// Kubernetes allows up to 253 characters, but we limit it for our use case, ideal would be < 63 (labels)
	MaxLength = 90
	// to avoid names like "a" which are valid but not very useful
	MinLength = 2
)

var (
	nameRegex = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`)
)

// ValidateObjectName checks if the name follows the naming convention of Kubernetes resources, see https://kubernetes.io/docs/concepts/overview/working-with-objects/names/#dns-subdomain-names
// This is the generic version that can be used by all handlers
func ValidateObjectName(obj types.Object) error {
	name := obj.GetName()

	fields := []FieldError{}
	if len(name) > MaxLength || len(name) < MinLength {
		fields = append(fields, FieldError{
			Field:  "name",
			Detail: fmt.Sprintf("name must be between %d and %d characters", MinLength, MaxLength),
		})
	}
	if !nameRegex.MatchString(name) {
		fields = append(fields, FieldError{
			Field:  "name",
			Detail: "name must consist of lower case alphanumeric characters or '-', start and end with an alphanumeric character",
		})
	}

	if strings.Contains(name, "--") {
		fields = append(fields, FieldError{
			Field:  "name",
			Detail: "name must not contain consecutive '-' characters",
		})
	}

	if len(fields) > 0 {
		return ValidationError(obj, fields...)
	}

	return nil
}
