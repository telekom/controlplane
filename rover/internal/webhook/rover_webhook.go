// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package webhook

import (
	"context"
	"fmt"
	apiapi "github.com/telekom/controlplane/api/api/v1"
	cclient "github.com/telekom/controlplane/common/pkg/client"
	"reflect"
	"strings"

	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	"github.com/telekom/controlplane/common/pkg/controller"
	roverv1 "github.com/telekom/controlplane/rover/api/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// log is for logging in this package.
var roverlog = logf.Log.WithName("rover-webhook")

// SetupWebhookWithManager will setup the manager to manage the webhooks
func SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(&roverv1.Rover{}).
		WithDefaulter(&RoverDefaulter{mgr.GetClient()}).
		WithValidator(&RoverValidator{mgr.GetClient()}).
		Complete()
}

// +kubebuilder:webhook:path=/mutate-rover-cp-ei-telekom-de-v1-rover,mutating=true,failurePolicy=fail,sideEffects=None,groups=rover.cp.ei.telekom.de,resources=rovers,verbs=create;update,versions=v1,name=mrover.kb.io,admissionReviewVersions=v1

type RoverDefaulter struct {
	client client.Client
}

var _ webhook.CustomDefaulter = &RoverDefaulter{}

func (r *RoverDefaulter) Default(ctx context.Context, obj runtime.Object) error {
	roverlog.Info("default")

	return nil
}

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
// NOTE: The 'path' attribute must follow a specific pattern and should not be modified directly here.
// Modifying the path for an invalid path can cause API server errors; failing to locate the webhook.
// +kubebuilder:webhook:path=/validate-rover-cp-ei-telekom-de-v1-rover,mutating=false,failurePolicy=fail,sideEffects=None,groups=rover.cp.ei.telekom.de,resources=rovers,verbs=create;update,versions=v1,name=vrover.kb.io,admissionReviewVersions=v1

// +kubebuilder:rbac:groups=admin.cp.ei.telekom.de,resources=zones,verbs=get;list;watch

type RoverValidator struct {
	client client.Client
}

var _ webhook.CustomValidator = &RoverValidator{}

func (r *RoverValidator) ValidateCreate(ctx context.Context, obj runtime.Object) (warnings admission.Warnings, err error) {
	roverlog.Info("validate create")

	return r.ValidateCreateOrUpdate(ctx, obj)
}

func (r *RoverValidator) ValidateUpdate(ctx context.Context, oldObj, obj runtime.Object) (warnings admission.Warnings, err error) {
	roverlog.Info("validate update")

	return r.ValidateCreateOrUpdate(ctx, obj)
}

func (r *RoverValidator) ValidateDelete(ctx context.Context, obj runtime.Object) (warnings admission.Warnings, err error) {
	roverlog.Info("validate delete")

	return
}

func (r *RoverValidator) ValidateCreateOrUpdate(ctx context.Context, obj runtime.Object) (warnings admission.Warnings, err error) {
	rover, ok := obj.(*roverv1.Rover)
	if !ok {
		return nil, apierrors.NewBadRequest("not a rover")
	}

	roverlog.Info("validate create or update", "name", rover.GetName())

	environment, ok := controller.GetEnvironment(rover)
	if !ok {
		return nil, apierrors.NewBadRequest("environment not found")
	}

	zoneRef := client.ObjectKey{
		Name:      rover.Spec.Zone,
		Namespace: environment,
	}
	if err = r.ResourceMustExist(ctx, zoneRef, &adminv1.Zone{}); err != nil {
		return nil, err
	}

	if err := MustNotHaveDuplicates(rover.Spec.Subscriptions, rover.Spec.Exposures); err != nil {
		return nil, err
	}

	for _, sub := range rover.Spec.Subscriptions {
		if _, err = r.ValidateSubscription(ctx, environment, sub); err != nil {
			return nil, err
		}
		if _, err = r.ValidateSubscriptionVisibility(ctx, sub, rover); err != nil {
			return nil, err
		}
	}

	for _, exposure := range rover.Spec.Exposures {
		if _, err = r.ValidateExposure(ctx, environment, exposure); err != nil {
			return nil, err
		}
	}

	return nil, nil
}

func (r *RoverValidator) ResourceMustExist(ctx context.Context, objRef client.ObjectKey, obj client.Object) error {
	err := r.client.Get(ctx, objRef, obj)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return apierrors.NewBadRequest(fmt.Sprintf("%s not found", reflect.TypeOf(obj).Elem().Name()))
		}
		return apierrors.NewInternalError(err)
	}
	return nil
}

func (r *RoverValidator) ValidateSubscription(ctx context.Context, environment string, sub roverv1.Subscription) (warnings admission.Warnings, err error) {
	roverlog.Info("validate subscription")

	if sub.Api != nil && sub.Api.Organization != "" {
		remoteOrgRef := client.ObjectKey{
			Name:      sub.Api.Organization,
			Namespace: environment,
		}
		if err = r.ResourceMustExist(ctx, remoteOrgRef, &adminv1.RemoteOrganization{}); err != nil {
			return nil, err
		}
	}

	return
}

func (r *RoverValidator) ValidateExposure(ctx context.Context, environment string, exposure roverv1.Exposure) (warnings admission.Warnings, err error) {
	if exposure.Api != nil {
		if strings.Contains(exposure.Api.Upstream, "localhost") {
			return nil, apierrors.NewBadRequest("upstream must not contain localhost")
		}
	}

	return
}

func (r *RoverValidator) ValidateSubscriptionVisibility(ctx context.Context, sub roverv1.Subscription, rover *roverv1.Rover) (warnings admission.Warnings, err error) {
	scopedClient := cclient.ClientFromContextOrDie(ctx)

	apiExposureList := &apiapi.ApiExposureList{}
	err = scopedClient.List(ctx, apiExposureList,
		client.MatchingLabels{apiapi.BasePathLabelKey: sub.Api.BasePath},
		client.MatchingFields{"status.active": "true"})

	if err != nil {
		return nil, err
	}

	if len(apiExposureList.Items) == 0 {
		return nil, apierrors.NewBadRequest("Active Api Exposure for this subscription not found")
	}

	exposure := &apiExposureList.Items[0]
	exposureVisibility := exposure.Spec.Visibility

	// any subscription is valid for a WORLD exposure
	if exposureVisibility == apiapi.VisibilityWorld {
		return nil, nil
	}

	// get the subscription zone
	subZone := &adminv1.Zone{}
	err = scopedClient.Get(ctx, client.ObjectKey{
		Namespace: "default",
		Name:      rover.Spec.Zone,
	}, subZone)
	if err != nil {
		return nil, apierrors.NewBadRequest("Zone '" + rover.Spec.Zone + "' not found")
	}

	// only same zone
	if exposureVisibility == apiapi.VisibilityZone {
		if exposure.Spec.Zone.GetName() != subZone.GetName() {
			return nil, apierrors.NewBadRequest("Exposure visibility is ZONE and it doesnt match the subscription zone '" + subZone.GetName() + "'")
		}
	}

	// only enterprise zones
	if exposureVisibility == apiapi.VisibilityEnterprise {
		if subZone.Spec.Visibility != adminv1.ZoneVisibilityEnterprise {
			return nil, apierrors.NewBadRequest(fmt.Sprintf("Api is exposed with visibility '%s', but subscriptions is from zone with visibility '%s'", apiapi.VisibilityEnterprise, subZone.Spec.Visibility))
		}
	}
	return nil, nil
}

// MustNotHaveDuplicates checks if there are no duplicates in the subscriptions and exposures
func MustNotHaveDuplicates(subs []roverv1.Subscription, exps []roverv1.Exposure) error {
	exisitingSubs := make(map[string]bool)
	for _, sub := range subs {
		if sub.Api != nil {
			if _, exists := exisitingSubs[sub.Api.BasePath]; exists {
				return apierrors.NewBadRequest(fmt.Sprintf("duplicate subscription for base path %s", sub.Api.BasePath))
			}
			exisitingSubs[sub.Api.BasePath] = true
		}

		if sub.Event != nil {
			if _, exists := exisitingSubs[sub.Event.EventType]; exists {
				return apierrors.NewBadRequest(fmt.Sprintf("duplicate subscription for event-type %s", sub.Event.EventType))
			}
			exisitingSubs[sub.Event.EventType] = true
		}
	}

	exisitingExps := make(map[string]bool)
	for _, exposure := range exps {
		if exposure.Api != nil {
			if _, exists := exisitingExps[exposure.Api.BasePath]; exists {
				return apierrors.NewBadRequest(fmt.Sprintf("duplicate exposure for base path %s", exposure.Api.BasePath))
			}
			exisitingExps[exposure.Api.BasePath] = true
		}

		if exposure.Event != nil {
			if _, exists := exisitingExps[exposure.Event.EventType]; exists {
				return apierrors.NewBadRequest(fmt.Sprintf("duplicate exposure for event-type %s", exposure.Event.EventType))
			}
			exisitingExps[exposure.Event.EventType] = true
		}
	}

	return nil
}
