// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package validator

import (
	"github.com/telekom/controlplane/common/pkg/controller"
	organizationv1 "github.com/telekom/controlplane/organization/api/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const separator = "--"

func ValidateTeamName(teamObj *organizationv1.Team) error {
	if teamObj.GetName() != teamObj.Spec.Group+separator+teamObj.Spec.Name {
		return errors.NewInvalid(
			schema.GroupKind{
				Group: organizationv1.GroupVersion.Group,
				Kind:  "Team",
			},
			teamObj.GetName(),
			field.ErrorList{
				field.Invalid(field.NewPath("metadata").Child("name"), teamObj.GetName(), "must be equal to 'spec.group"+separator+"spec.name'"),
			},
		)
	}
	return nil
}

func ValidateAndGetEnv(obj client.Object) (string, error) {
	env, ok := controller.GetEnvironment(obj)
	if !ok {
		return env, errors.NewInvalid(
			schema.GroupKind{
				Group: organizationv1.GroupVersion.Group,
				Kind:  "Team",
			},
			obj.GetName(),
			field.ErrorList{
				field.Invalid(field.NewPath("metadata").Child("labels"), obj.GetLabels(), "must contain an environment label"),
			},
		)
	}

	return env, nil
}
