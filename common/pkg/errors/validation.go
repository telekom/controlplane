// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package errors

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
	Errors   field.ErrorList
	Warnings admission.Warnings
	gk       schema.GroupKind
	ref      types.NamedObject
}

func NewValidationError(gk schema.GroupKind, ref types.NamedObject) *ValidationError {
	return &ValidationError{
		gk:       gk,
		ref:      ref,
		Errors:   field.ErrorList{},
		Warnings: admission.Warnings{},
	}
}

func (e *ValidationError) AddError(err *field.Error) {
	e.Errors = append(e.Errors, err)
}

func (e *ValidationError) AddInvalidError(path *field.Path, value any, message string) {
	e.AddError(field.Invalid(path, value, message))
}

func (e *ValidationError) AddRequiredError(path *field.Path, message string) {
	e.AddError(field.Required(path, message))
}

func (e *ValidationError) BuildError() error {
	if len(e.Errors) == 0 {
		return nil
	}

	return apierrors.NewInvalid(
		e.gk,
		e.ref.GetName(),
		e.Errors,
	)
}

func (e *ValidationError) HasErrors() bool {
	return len(e.Errors) > 0
}

func (e *ValidationError) BuildWarnings() admission.Warnings {
	if len(e.Warnings) == 0 {
		return nil
	}
	return e.Warnings
}

func (e *ValidationError) AddWarning(message string) {
	e.Warnings = append(e.Warnings, message)
}

func (e *ValidationError) AddWarningf(path *field.Path, value any, format string, args ...any) {
	e.Warnings = append(e.Warnings, fmt.Sprintf("%s: %s", path.String(), fmt.Sprintf(format, args...)))
}
