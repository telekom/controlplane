// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"context"
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/telekom/controlplane/common/pkg/controller"
	cerrors "github.com/telekom/controlplane/common/pkg/errors"
	"github.com/telekom/controlplane/common/pkg/types"
	organizationv1 "github.com/telekom/controlplane/organization/api/v1"
	roverv1 "github.com/telekom/controlplane/rover/api/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	apiv1 "github.com/telekom/controlplane/api/api/v1"

	cclient "github.com/telekom/controlplane/common/pkg/client"
)

// SetupApiSpecificationWebhookWithManager registers the webhook for ApiSpecification in the manager.
func SetupApiSpecificationWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).For(&roverv1.ApiSpecification{}).
		WithValidator(&ApiSpecificationCustomValidator{
			client:   mgr.GetClient(),
			FindTeam: organizationv1.FindTeamForObject,
			ListApiCategories: func(ctx context.Context) (*apiv1.ApiCategoryList, error) {
				client := cclient.ClientFromContextOrDie(ctx)
				apiCategories := &apiv1.ApiCategoryList{}
				err := client.List(ctx, apiCategories)
				return apiCategories, err
			},
		}).Complete()
}

// +kubebuilder:webhook:path=/validate-rover-cp-ei-telekom-de-v1-apispecification,mutating=false,failurePolicy=fail,sideEffects=None,groups=rover.cp.ei.telekom.de,resources=apispecifications,verbs=create;update,versions=v1,name=vapispecification-v1.kb.io,admissionReviewVersions=v1
// +kubebuilder:rbac:groups=organization.cp.ei.telekom.de,resources=teams,verbs=get;list;watch

type ApiSpecificationCustomValidator struct {
	client            client.Client
	FindTeam          func(ctx context.Context, obj types.NamedObject) (*organizationv1.Team, error)
	ListApiCategories func(ctx context.Context) (*apiv1.ApiCategoryList, error)
}

var _ webhook.CustomValidator = &ApiSpecificationCustomValidator{}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type ApiSpecification.
func (v *ApiSpecificationCustomValidator) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	return v.ValidateCreateOrUpdate(ctx, obj)
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type ApiSpecification.
func (v *ApiSpecificationCustomValidator) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	return v.ValidateCreateOrUpdate(ctx, newObj)
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type ApiSpecification.
func (v *ApiSpecificationCustomValidator) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	return nil, nil
}

func (v *ApiSpecificationCustomValidator) ValidateCreateOrUpdate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	apispecification, ok := obj.(*roverv1.ApiSpecification)
	if !ok {
		return nil, apierrors.NewBadRequest("not an apispecification")
	}

	valErr := cerrors.NewValidationError(roverv1.GroupVersion.WithKind("ApiSpecification").GroupKind(), apispecification)

	environment, ok := controller.GetEnvironment(apispecification)
	if !ok {
		valErr.AddInvalidError(cerrors.MetadataEnvPath, "", "environment label is required")
		return valErr.BuildWarnings(), valErr.BuildError()
	}

	ctx = cclient.WithClient(ctx, cclient.NewJanitorClient(cclient.NewScopedClient(v.client, environment)))

	if err := v.ApiCategoryValidation(ctx, valErr, apispecification); err != nil {
		return valErr.BuildWarnings(), err
	}

	return valErr.BuildWarnings(), valErr.BuildError()
}

func (v *ApiSpecificationCustomValidator) ApiCategoryValidation(ctx context.Context, valErr *cerrors.ValidationError, apispecification *roverv1.ApiSpecification) error {

	apiCategories, err := v.ListApiCategories(ctx)
	if err != nil {
		return apierrors.NewInternalError(err)
	}

	if apiCategories == nil || len(apiCategories.Items) == 0 {
		// allow all
		return nil
	}

	providedCategory := apispecification.Spec.Category
	foundApiCategory, found := apiCategories.FindByLabelValue(providedCategory)
	if !found {
		allowedLabels := strings.Join(apiCategories.AllowedLabelValues(), ", ")
		valErr.AddInvalidError(field.NewPath("spec").Child("category"), providedCategory, fmt.Sprintf("ApiCategory %q not found. Allowed values are: [%s]", providedCategory, allowedLabels))
	} else {
		if !foundApiCategory.Spec.Active {
			valErr.AddInvalidError(field.NewPath("spec").Child("category"), providedCategory, "the provided ApiCategory is not active")
		}
		team, err := v.FindTeam(ctx, apispecification)
		if err != nil {
			return apierrors.NewInternalError(err)
		}

		if !foundApiCategory.IsAllowedForTeamCategory(string(team.Spec.Category)) {
			valErr.AddInvalidError(field.NewPath("spec").Child("category"), providedCategory, fmt.Sprintf("ApiCategory %q is not allowed for team category %q", providedCategory, team.Spec.Category))
		}
		if !foundApiCategory.IsAllowedForTeam(team.GetName()) {
			valErr.AddInvalidError(field.NewPath("spec").Child("category"), providedCategory, fmt.Sprintf("ApiCategory %q is not allowed for team name %q", providedCategory, team.GetName()))
		}

		if foundApiCategory.Spec.MustHaveGroupPrefix {
			expectedPrefix := team.Spec.Group
			providedPrefix := strings.Split(strings.Trim(apispecification.Spec.BasePath, "/"), "/")[0]
			if expectedPrefix != providedPrefix {
				valErr.AddInvalidError(field.NewPath("spec").Child("basePath"), providedPrefix, fmt.Sprintf("basePath must start with the team group prefix %q as ApiCategory %q requires it", expectedPrefix, providedCategory))
			}
		}
	}

	return nil
}
