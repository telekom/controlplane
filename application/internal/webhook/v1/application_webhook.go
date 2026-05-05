// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	applicationv1 "github.com/telekom/controlplane/application/api/v1"
	"github.com/telekom/controlplane/application/internal/webhook/v1/mutator"
	"github.com/telekom/controlplane/application/internal/webhook/v1/validator"
	"github.com/telekom/controlplane/common/pkg/controller"
)

var applicationLog = logf.Log.WithName("application-resource").WithValues("apiVersion", "application.cp.ei.telekom.de/v1", "kind", "Application")

// SetupApplicationWebhookWithManager registers the webhook for Application in the manager.
func SetupApplicationWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, &applicationv1.Application{}).
		WithDefaulter(&ApplicationCustomDefaulter{}).
		WithValidator(&ApplicationCustomValidator{}).
		Complete()
}

func setupLog(ctx context.Context, obj client.Object) (context.Context, logr.Logger) {
	log := applicationLog.WithValues("name", obj.GetName(), "namespace", obj.GetNamespace())
	return logr.NewContext(ctx, log), log
}

// +kubebuilder:webhook:path=/mutate-application-cp-ei-telekom-de-v1-application,mutating=true,failurePolicy=fail,sideEffects=None,groups=application.cp.ei.telekom.de,resources=applications,verbs=create;update,versions=v1,name=mapplication-v1.kb.io,admissionReviewVersions=v1

var _ admission.Defaulter[*applicationv1.Application] = &ApplicationCustomDefaulter{}

type ApplicationCustomDefaulter struct{}

func (d *ApplicationCustomDefaulter) Default(ctx context.Context, app *applicationv1.Application) error {
	ctx, log := setupLog(ctx, app)
	log.Info("defaulting application")

	env, ok := controller.GetEnvironment(app)
	if !ok {
		return fmt.Errorf("application %s does not have an environment label", app.GetName())
	}

	return mutator.MutateSecret(ctx, env, app)
}

// +kubebuilder:webhook:path=/validate-application-cp-ei-telekom-de-v1-application,mutating=false,failurePolicy=fail,sideEffects=None,groups=application.cp.ei.telekom.de,resources=applications,verbs=create;update,versions=v1,name=vapplication-v1.kb.io,admissionReviewVersions=v1

var _ admission.Validator[*applicationv1.Application] = &ApplicationCustomValidator{}

type ApplicationCustomValidator struct{}

func (v *ApplicationCustomValidator) ValidateCreate(ctx context.Context, app *applicationv1.Application) (admission.Warnings, error) {
	return v.validateCreateOrUpdate(ctx, app)
}

func (v *ApplicationCustomValidator) ValidateUpdate(ctx context.Context, _, app *applicationv1.Application) (admission.Warnings, error) {
	return v.validateCreateOrUpdate(ctx, app)
}

func (v *ApplicationCustomValidator) ValidateDelete(_ context.Context, _ *applicationv1.Application) (admission.Warnings, error) {
	return nil, nil
}

func (v *ApplicationCustomValidator) validateCreateOrUpdate(ctx context.Context, app *applicationv1.Application) (admission.Warnings, error) {
	_, log := setupLog(ctx, app)
	log.Info("validating application")

	if _, err := validator.ValidateAndGetEnv(app); err != nil {
		return nil, err
	}

	return nil, nil
}
