// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package validator

import (
	"fmt"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/telekom/controlplane/common/pkg/controller"
	organizationv1 "github.com/telekom/controlplane/organization/api/v1"
)

const separator = "--"

func ValidateTeamName(teamObj *organizationv1.Team) error {
	if teamObj.Spec.Group == "" || teamObj.Spec.Name == "" || teamObj.GetName() != organizationv1.TeamResourceName(teamObj.Spec.Group, teamObj.Spec.Name) {
		return errors.NewInvalid(
			schema.GroupKind{
				Group: organizationv1.GroupVersion.Group,
				Kind:  "Team",
			},
			teamObj.GetName(),
			field.ErrorList{
				field.Invalid(field.NewPath("metadata").Child("name"), teamObj.GetName(), "must be equal to the normalized 'spec.group"+separator+"spec.name'"),
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

// ValidateTeamNamespace bounds the namespace built from the original spec fields.
func ValidateTeamNamespace(team *organizationv1.Team, environment string) error {
	budget := validation.DNS1123LabelMaxLength - len(environment) - 2*len(separator)
	if len(team.Spec.Group)+len(team.Spec.Name) <= budget {
		return nil
	}

	return errors.NewInvalid(organizationv1.GroupVersion.WithKind("Team").GroupKind(), team.Name, field.ErrorList{
		field.Invalid(field.NewPath("spec", "name"), team.Spec.Name,
			fmt.Sprintf("generated namespace must not exceed %d characters; spec.group and spec.name together may use at most %d characters in environment %q after reserving space for both '--' separators",
				validation.DNS1123LabelMaxLength, max(0, budget), environment)),
	})
}
