// SPDX-FileCopyrightText: 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"context"
	"fmt"
	"github.com/pkg/errors"
	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	cclient "github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/controller"
	"github.com/telekom/controlplane/common/pkg/util/labelutil"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	apiv1 "github.com/telekom/controlplane/api/api/v1"
)

// nolint:unused
// log is for logging in this package.
var apisubscriptionlog = logf.Log.WithName("apisubscription-resource")

// SetupApiSubscriptionWebhookWithManager registers the webhook for ApiSubscription in the manager.
func SetupApiSubscriptionWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).For(&apiv1.ApiSubscription{}).
		WithValidator(&ApiSubscriptionCustomValidator{mgr.GetClient()}).
		WithDefaulter(&ApiSubscriptionCustomDefaulter{}).
		Complete()
}

// +kubebuilder:webhook:path=/mutate-api-cp-ei-telekom-de-v1-apisubscription,mutating=true,failurePolicy=fail,sideEffects=None,groups=api.cp.ei.telekom.de,resources=apisubscriptions,verbs=create;update,versions=v1,name=mapisubscription-v1.kb.io,admissionReviewVersions=v1

// ApiSubscriptionCustomDefaulter struct is responsible for setting default values on the custom resource of the
// Kind ApiSubscription when those are created or updated.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as it is used only for temporary operations and does not need to be deeply copied.
type ApiSubscriptionCustomDefaulter struct {
	// TODO(user): Add more fields as needed for defaulting
}

var _ webhook.CustomDefaulter = &ApiSubscriptionCustomDefaulter{}

// Default implements webhook.CustomDefaulter so a webhook will be registered for the Kind ApiSubscription.
func (d *ApiSubscriptionCustomDefaulter) Default(_ context.Context, obj runtime.Object) error {
	apisubscription, ok := obj.(*apiv1.ApiSubscription)

	if !ok {
		return fmt.Errorf("expected an ApiSubscription object but got %T", obj)
	}
	apisubscriptionlog.Info("Defaulting for ApiSubscription", "name", apisubscription.GetName())

	return nil
}

// +kubebuilder:webhook:path=/validate-api-cp-ei-telekom-de-v1-apisubscription,mutating=false,failurePolicy=fail,sideEffects=None,groups=api.cp.ei.telekom.de,resources=apisubscriptions,verbs=create;update,versions=v1,name=vapisubscription-v1.kb.io,admissionReviewVersions=v1

// ApiSubscriptionCustomValidator struct is responsible for validating the ApiSubscription resource
// when it is created, updated, or deleted.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as this struct is used only for temporary operations and does not need to be deeply copied.
type ApiSubscriptionCustomValidator struct {
	client client.Client
}

var _ webhook.CustomValidator = &ApiSubscriptionCustomValidator{}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type ApiSubscription.
func (v *ApiSubscriptionCustomValidator) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	apisubscription, ok := obj.(*apiv1.ApiSubscription)
	if !ok {
		return nil, fmt.Errorf("expected a ApiSubscription object but got %T", obj)
	}
	apisubscriptionlog.Info("Validation for ApiSubscription upon creation", "name", apisubscription.GetName())

	return validateCreateOrUpdate(ctx, v.client, apisubscription)
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type ApiSubscription.
func (v *ApiSubscriptionCustomValidator) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	apisubscription, ok := newObj.(*apiv1.ApiSubscription)
	if !ok {
		return nil, fmt.Errorf("expected a ApiSubscription object for the newObj but got %T", newObj)
	}
	apisubscriptionlog.Info("Validation for ApiSubscription upon update", "name", apisubscription.GetName())

	return validateCreateOrUpdate(ctx, v.client, apisubscription)
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type ApiSubscription.
func (v *ApiSubscriptionCustomValidator) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	apisubscription, ok := obj.(*apiv1.ApiSubscription)
	if !ok {
		return nil, fmt.Errorf("expected a ApiSubscription object but got %T", obj)
	}
	apisubscriptionlog.Info("Validation for ApiSubscription upon deletion", "name", apisubscription.GetName())

	// todo handle deletion

	return nil, nil
}

func validateCreateOrUpdate(ctx context.Context, c client.Client, sub *apiv1.ApiSubscription) (admission.Warnings, error) {
	apisubscriptionlog.Info("Validate for ApiSubscription upon creation", "name", sub.GetName())

	env, found := controller.GetEnvironment(sub)
	if !found {
		return nil, apierrors.NewBadRequest("Environment validation failed - label is not present on subscription")
	}

	scopedClient := cclient.NewScopedClient(c, env)

	apiExposureList := &apiv1.ApiExposureList{}
	err := scopedClient.List(ctx, apiExposureList,
		client.MatchingLabels{apiv1.BasePathLabelKey: labelutil.NormalizeValue(sub.Spec.ApiBasePath)},
		client.MatchingFields{"status.active": "true"})

	if err != nil {
		apisubscriptionlog.Error(err, "unable to list ApiExposure", "basePath", sub.Spec.ApiBasePath)
		return nil, apierrors.NewNotFound(schema.GroupResource{Group: apiv1.GroupVersion.Group, Resource: "ApiExposure"}, "Active Api Exposure for this subscription not found due to error")
	}

	if len(apiExposureList.Items) == 0 {
		apisubscriptionlog.Error(errors.New("unable to list ApiExposure(s) for api subscription"), "basePath", sub.Spec.ApiBasePath)
		return nil, apierrors.NewNotFound(schema.GroupResource{Group: apiv1.GroupVersion.Group, Resource: "ApiExposure"}, "Active Api Exposure for this subscription not found")
	}

	exposure := &apiExposureList.Items[0]
	exposureVisibility := exposure.Spec.Visibility

	// any subscription is valid for a WORLD exposure
	if exposureVisibility == apiv1.VisibilityWorld {
		return nil, nil
	}

	// get the subscription zone
	subZone := &adminv1.Zone{}
	err = scopedClient.Get(ctx, sub.Spec.Zone.K8s(), subZone)
	if err != nil {
		apisubscriptionlog.Error(err, "unable to get zone", "name", sub.Spec.Zone.K8s())
		return nil, apierrors.NewBadRequest("Zone '" + sub.Spec.Zone.GetName() + "' not found")
	}

	// only same zone
	if exposureVisibility == apiv1.VisibilityZone {
		if exposure.Spec.Zone.GetName() != subZone.GetName() {
			return nil, apierrors.NewBadRequest("Exposure visibility is ZONE and it doesnt match the subscription zone '" + subZone.GetName() + "'")
		}
	}

	// only enterprise zones
	if exposureVisibility == apiv1.VisibilityEnterprise {
		if subZone.Spec.Visibility != adminv1.ZoneVisibilityEnterprise {
			return nil, apierrors.NewBadRequest(fmt.Sprintf("Api is exposed with visibility '%s', but subscriptions is from zone with visibility '%s'", apiv1.VisibilityEnterprise, subZone.Spec.Visibility))
		}
	}
	return nil, nil
}
