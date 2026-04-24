// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package validator

import (
	applicationv1 "github.com/telekom/controlplane/application/api/v1"
	"github.com/telekom/controlplane/common/pkg/controller"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ValidateAndGetEnv verifies that the object carries an environment label
// and returns its value. Returns a well-formed Invalid error when absent.
func ValidateAndGetEnv(obj client.Object) (string, error) {
	env, ok := controller.GetEnvironment(obj)
	if !ok {
		return env, errors.NewInvalid(
			schema.GroupKind{
				Group: applicationv1.GroupVersion.Group,
				Kind:  "Application",
			},
			obj.GetName(),
			field.ErrorList{
				field.Invalid(field.NewPath("metadata").Child("labels"), obj.GetLabels(), "must contain an environment label"),
			},
		)
	}
	return env, nil
}
