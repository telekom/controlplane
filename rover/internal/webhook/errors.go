// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package webhook

import (
	"fmt"

	"github.com/telekom/controlplane/common/pkg/config"
	"github.com/telekom/controlplane/common/pkg/types"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

var (
	MetadataEnvPath = field.NewPath("metadata").Child("labels").Child(config.EnvironmentLabelKey)
)

type ValidationError struct {
	errors   field.ErrorList
	warnings admission.Warnings
	gk       schema.GroupKind
	ref      types.NamedObject
}

func NewValidationError(gk schema.GroupKind, ref types.NamedObject) *ValidationError {
	return &ValidationError{
		gk:       gk,
		ref:      ref,
		errors:   field.ErrorList{},
		warnings: admission.Warnings{},
	}
}

func (e *ValidationError) AddError(err *field.Error) {
	e.errors = append(e.errors, err)
}

func (e *ValidationError) AddInvalidError(path *field.Path, value any, message string) {
	e.AddError(field.Invalid(path, value, message))
}

func (e *ValidationError) AddRequiredError(path *field.Path, message string) {
	e.AddError(field.Required(path, message))
}

func (e *ValidationError) BuildError() error {
	if len(e.errors) == 0 {
		return nil
	}

	return apierrors.NewInvalid(
		e.gk,
		e.ref.GetName(),
		e.errors,
	)
}

func (e *ValidationError) HasErrors() bool {
	return len(e.errors) > 0
}

func (e *ValidationError) BuildWarnings() admission.Warnings {
	if len(e.warnings) == 0 {
		return nil
	}
	return e.warnings
}

func (e *ValidationError) AddWarning(message string) {
	e.warnings = append(e.warnings, message)
}

func (e *ValidationError) AddWarningf(path *field.Path, value any, format string, args ...any) {
	e.warnings = append(e.warnings, fmt.Sprintf("%s: %s", path.String(), fmt.Sprintf(format, args...)))
}
