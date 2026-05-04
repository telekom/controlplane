// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	applicationv1 "github.com/telekom/controlplane/application/api/v1"
	"github.com/telekom/controlplane/application/internal/webhook/v1/mutator"
	"github.com/telekom/controlplane/application/internal/webhook/v1/validator"
	"github.com/telekom/controlplane/common/pkg/controller"
	cerrors "github.com/telekom/controlplane/common/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/validation/field"
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
		WithValidator(&ApplicationCustomValidator{client: mgr.GetClient()}).
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
// +kubebuilder:rbac:groups=admin.cp.ei.telekom.de,resources=zones,verbs=get;list;watch

var _ admission.Validator[*applicationv1.Application] = &ApplicationCustomValidator{}

type ApplicationCustomValidator struct {
	client client.Client
}

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
	ctx, log := setupLog(ctx, app)
	log.Info("validating application")

	env, err := validator.ValidateAndGetEnv(app)
	if err != nil {
		return nil, err
	}

	valErr := cerrors.NewValidationError(applicationv1.GroupVersion.WithKind("Application").GroupKind(), app)

	zone, err := v.getZone(ctx, app, env)
	if err != nil {
		return nil, err
	}
	if zone != nil {
		validateExternalIds(valErr, app.Spec.ExternalIds, zone, field.NewPath("spec").Child("externalIds"))
	}

	return valErr.BuildWarnings(), valErr.BuildError()
}

// getZone fetches the Zone referenced by the Application. It returns (nil, nil)
// when the Application does not reference a Zone name (defensive — the CRD
// marks Zone as required, but the validator avoids crashing on malformed input
// and lets the required-field rejection happen elsewhere). A missing Zone is
// reported as a validation failure.
func (v *ApplicationCustomValidator) getZone(ctx context.Context, app *applicationv1.Application, env string) (*adminv1.Zone, error) {
	if app.Spec.Zone.Name == "" {
		return nil, nil
	}

	// Zones live in the environment namespace, matching the Rover webhook.
	zoneRef := client.ObjectKey{
		Name:      app.Spec.Zone.Name,
		Namespace: env,
	}
	zone := &adminv1.Zone{}
	if err := v.client.Get(ctx, zoneRef, zone); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, apierrors.NewBadRequest(fmt.Sprintf("zone '%s' not found", app.Spec.Zone.Name))
		}
		return nil, apierrors.NewInternalError(err)
	}
	return zone, nil
}
