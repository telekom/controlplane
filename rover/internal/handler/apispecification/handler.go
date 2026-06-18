// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package apispecification

import (
	"context"
	"fmt"
	"strings"

	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	apiapi "github.com/telekom/controlplane/api/api/v1"
	"github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/handler"
	"github.com/telekom/controlplane/common/pkg/types"
	"github.com/telekom/controlplane/common/pkg/util/labelutil"
	roverv1 "github.com/telekom/controlplane/rover/api/v1"
	roverindex "github.com/telekom/controlplane/rover/internal/index"
)

var _ handler.Handler[*roverv1.ApiSpecification] = (*ApiSpecificationHandler)(nil)

// ApiSpecificationHandler reconciles ApiSpecification resources.
// Linting is performed by rover-server at upload time and stored in Spec.Lint.
// This handler reads the lint result and gates Api resource creation accordingly.
type ApiSpecificationHandler struct{}

func (h *ApiSpecificationHandler) CreateOrUpdate(ctx context.Context, apiSpec *roverv1.ApiSpecification) error {
	mode := h.lookupLintingMode(ctx, apiSpec.Spec.Category)

	// Check if linting failed and the category config blocks on failure.
	// If Spec.Lint is nil (no result yet or linting not configured), proceed normally
	// to avoid blocking indefinitely if the linter is unavailable.
	if apiSpec.Spec.Lint != nil && !apiSpec.Spec.Lint.Passed && mode == apiapi.LintingModeBlock {
		msg := fmt.Sprintf("OAS linting failed: %s", apiSpec.Spec.Lint.Message)
		if apiSpec.Spec.Lint.DashboardURL != "" {
			msg = fmt.Sprintf("%s. View details: %s", msg, apiSpec.Spec.Lint.DashboardURL)
		}
		apiSpec.SetCondition(condition.NewBlockedCondition(msg))
		apiSpec.SetCondition(condition.NewNotReadyCondition("LintingFailed",
			"API specification did not pass linting"))
		return nil
	}

	return h.createOrUpdateApi(ctx, apiSpec)
}

func (h *ApiSpecificationHandler) Delete(_ context.Context, _ *roverv1.ApiSpecification) error {
	return nil
}

// lookupLintingMode finds the ApiCategory and returns the effective linting mode.
func (h *ApiSpecificationHandler) lookupLintingMode(ctx context.Context, category string) apiapi.LintingMode {
	cat, err := h.getApiCategory(ctx, category)
	if err != nil || cat == nil || cat.Spec.Linting == nil {
		return apiapi.LintingModeNone
	}
	mode := cat.Spec.Linting.Mode
	if mode == "" {
		mode = apiapi.LintingModeBlock
	}
	return mode
}

func (h *ApiSpecificationHandler) getApiCategory(ctx context.Context, category string) (*apiapi.ApiCategory, error) {
	c := client.ClientFromContextOrDie(ctx)
	list := &apiapi.ApiCategoryList{}
	if err := c.List(ctx, list, ctrlclient.MatchingFields{roverindex.FieldApiCategoryLabelValue: strings.ToLower(category)}); err != nil {
		return nil, err
	}
	if len(list.Items) == 0 {
		return nil, nil
	}
	return &list.Items[0], nil
}

// createOrUpdateApi contains the Api resource creation logic.
func (h *ApiSpecificationHandler) createOrUpdateApi(ctx context.Context, apiSpec *roverv1.ApiSpecification) error {
	c := client.ClientFromContextOrDie(ctx)
	name := roverv1.MakeName(apiSpec)

	api := &apiapi.Api{
		ObjectMeta: metav1.ObjectMeta{
			Name:      labelutil.NormalizeNameValue(name),
			Namespace: apiSpec.Namespace,
		},
	}

	apiSpec.Status.Api = *types.ObjectRefFromObject(api)

	mutator := func() error {
		err := controllerutil.SetControllerReference(apiSpec, api, c.Scheme())
		if err != nil {
			return errors.Wrap(err, "failed to set controller reference")
		}

		api.Labels = map[string]string{
			apiapi.BasePathLabelKey: labelutil.NormalizeLabelValue(apiSpec.Spec.BasePath),
		}

		api.Spec = apiapi.ApiSpec{
			Version:       apiSpec.Spec.Version,
			BasePath:      apiSpec.Spec.BasePath,
			Category:      apiSpec.Spec.Category,
			Oauth2Scopes:  apiSpec.Spec.Oauth2Scopes,
			XVendor:       apiSpec.Spec.XVendor,
			Specification: apiSpec.Spec.Specification,
		}

		return nil
	}

	_, err := c.CreateOrUpdate(ctx, api, mutator)
	if err != nil {
		return errors.Wrap(err, "failed to create or update api")
	}

	if c.AnyChanged() {
		apiSpec.SetCondition(condition.NewProcessingCondition("Provisioning", "API updated"))
		apiSpec.SetCondition(condition.NewNotReadyCondition("Provisioning", "API is not ready"))
	} else {
		apiSpec.SetCondition(condition.NewDoneProcessingCondition("API created"))
		apiSpec.SetCondition(condition.NewReadyCondition("Provisioned", "API is ready"))
	}

	return nil
}
