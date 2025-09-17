// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"context"
	"fmt"

	"github.com/telekom/controlplane/organization/internal/secret"
	"github.com/telekom/controlplane/organization/internal/webhook/v1/mutator"
	"github.com/telekom/controlplane/organization/internal/webhook/v1/validator"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	organizationv1 "github.com/telekom/controlplane/organization/api/v1"
)

// nolint:unused
// log is for logging in this package.
var teamlog = logf.Log.WithName("team-resource")

// SetupTeamWebhookWithManager registers the webhook for Team in the manager.
func SetupTeamWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).For(&organizationv1.Team{}).
		WithValidator(&TeamCustomValidator{}).
		WithDefaulter(&TeamCustomDefaulter{mgr.GetClient()}).
		Complete()
}

// NOTE: The 'path' attribute must follow a specific pattern and should not be modified directly here.
// Modifying the path for an invalid path can cause API server errors; failing to locate the webhook.
// +kubebuilder:webhook:path=/validate-organization-cp-ei-telekom-de-v1-team,mutating=false,failurePolicy=fail,sideEffects=None,groups=organization.cp.ei.telekom.de,resources=teams,verbs=create;update;delete,versions=v1,name=vteam-v1.kb.io,admissionReviewVersions=v1

// TeamCustomValidator struct is responsible for validating the Team resource
// when it is created, updated, or deleted.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as this struct is used only for temporary operations and does not need to be deeply copied.
type TeamCustomValidator struct{}

var _ webhook.CustomValidator = &TeamCustomValidator{}

// +kubebuilder:webhook:path=/mutate-organization-cp-ei-telekom-de-v1-team,mutating=true,failurePolicy=fail,sideEffects=None,groups=organization.cp.ei.telekom.de,resources=teams,verbs=create;update;delete,versions=v1,name=mteam-v1.kb.io,admissionReviewVersions=v1

type TeamCustomDefaulter struct {
	client client.Client
}

func (t TeamCustomDefaulter) Default(ctx context.Context, obj runtime.Object) error {
	teamlog.V(1).Info("mutating webhook called")
	teamObj, ok := obj.(*organizationv1.Team)
	if !ok {
		return errors.NewInternalError(fmt.Errorf("unable to convert object to team object"))
	}

	env, err := validator.ValidateAndGetEnv(teamObj)
	if err != nil {
		return err
	}

	teamlog.V(1).Info("mutating secret")
	err = mutator.MutateSecret(ctx, t.client, env, teamObj)
	if err != nil {
		return err
	}
	return nil
}

var _ webhook.CustomDefaulter = &TeamCustomDefaulter{}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type Team.
func (v *TeamCustomValidator) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	teamlog.Info("validate create")
	return v.validateCreateOrUpdate(ctx, obj)
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type Team.
func (v *TeamCustomValidator) ValidateUpdate(ctx context.Context, _, newObj runtime.Object) (admission.Warnings, error) {
	teamlog.Info("validate update")
	return v.validateCreateOrUpdate(ctx, newObj)
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type Team.
func (v *TeamCustomValidator) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	teamlog.Info("validate delete")

	teamObj, ok := obj.(*organizationv1.Team)
	if !ok {
		return nil, errors.NewInternalError(fmt.Errorf("unable to convert object to team object"))
	}

	env, err := validator.ValidateAndGetEnv(teamObj)
	if err != nil {
		return nil, err
	}

	err = secret.GetSecretManager().DeleteTeam(ctx, env, teamObj.GetName())
	if err != nil {
		return nil, errors.NewInternalError(err)
	}

	return nil, nil
}

func (v *TeamCustomValidator) validateCreateOrUpdate(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	teamObj, ok := obj.(*organizationv1.Team)
	if !ok {
		return nil, fmt.Errorf("unable to convert object to team object")
	}

	err := validator.ValidateTeamName(teamObj)
	if err != nil {
		return nil, err
	}

	_, err = validator.ValidateAndGetEnv(teamObj)
	if err != nil {
		return nil, err
	}

	return nil, nil
}
