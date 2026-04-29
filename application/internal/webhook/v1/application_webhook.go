// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	applicationv1 "github.com/telekom/controlplane/application/api/v1"
	"github.com/telekom/controlplane/application/internal/webhook/v1/mutator"
	"github.com/telekom/controlplane/common/pkg/controller"
	"github.com/telekom/controlplane/common/pkg/util/contextutil"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/tools/events"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

var applicationLog = logf.Log.WithName("application-resource").WithValues("apiVersion", "application.cp.ei.telekom.de/v1", "kind", "Application")

// SetupApplicationWebhookWithManager registers the webhook for Application in the manager.
func SetupApplicationWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, &applicationv1.Application{}).
		WithDefaulter(&ApplicationCustomDefaulter{
			Reader:   mgr.GetClient(),
			Recorder: mgr.GetEventRecorder("application-webhook"),
		}).
		Complete()
}

func setupLog(ctx context.Context, obj client.Object) (context.Context, logr.Logger) {
	log := applicationLog.WithValues("name", obj.GetName(), "namespace", obj.GetNamespace())
	return logr.NewContext(ctx, log), log
}

// +kubebuilder:webhook:path=/mutate-application-cp-ei-telekom-de-v1-application,mutating=true,failurePolicy=fail,sideEffects=None,groups=application.cp.ei.telekom.de,resources=applications,verbs=create;update,versions=v1,name=mapplication-v1.kb.io,admissionReviewVersions=v1
// +kubebuilder:rbac:groups=core,resources=events,verbs=create;patch

var _ admission.Defaulter[*applicationv1.Application] = &ApplicationCustomDefaulter{}

type ApplicationCustomDefaulter struct {
	Reader   client.Reader
	Recorder events.EventRecorder
}

func (d *ApplicationCustomDefaulter) Default(ctx context.Context, app *applicationv1.Application) error {
	ctx, log := setupLog(ctx, app)
	log.Info("defaulting application")

	ctx = contextutil.WithEventRecorder(ctx, d.Recorder)

	env, ok := controller.GetEnvironment(app)
	if !ok {
		return fmt.Errorf("application %s does not have an environment label", app.GetName())
	}

	if err := mutator.MutateSecret(ctx, env, app, d.Reader); err != nil {
		if errors.IsInternalError(err) {
			log.Error(err, "failed to default application")
			return fmt.Errorf("failed to default application")
		}
		return err
	}
	return nil
}
