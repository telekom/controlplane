// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package common

import (
	"regexp"
	"strings"

	"github.com/telekom/controlplane/rover-ctl/pkg/types"
)

var nameRegex = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`)

// ValidateObjectName checks if the name follows the naming convention of Kubernetes resources, see https://kubernetes.io/docs/concepts/overview/working-with-objects/names/#dns-subdomain-names
func ValidateObjectName(obj types.Object) error {
	name := obj.GetName()

	fields := []FieldError{}
	if len(name) > 63 {
		fields = append(fields, FieldError{
			Field:  "name",
			Detail: "name must be no more than 63 characters",
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
