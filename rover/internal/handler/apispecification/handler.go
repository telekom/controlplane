// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package apispecification

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	apiapi "github.com/telekom/controlplane/api/api/v1"
	"github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/handler"
	"github.com/telekom/controlplane/common/pkg/types"
	"github.com/telekom/controlplane/common/pkg/util/labelutil"
	roverv1 "github.com/telekom/controlplane/rover/api/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

var _ handler.Handler[*roverv1.ApiSpecification] = (*ApiSpecificationHandler)(nil)

// ApiSpecificationHandler reconciles ApiSpecification resources.
// Linting is performed by rover-server at upload time and stored in the CRD status fields.
// This handler reads the lint result and gates Api resource creation accordingly.
type ApiSpecificationHandler struct {
	GetApiCategory func(ctx context.Context, category string) (*apiapi.ApiCategory, error)
}

func (h *ApiSpecificationHandler) CreateOrUpdate(ctx context.Context, apiSpec *roverv1.ApiSpecification) error {
	log := logr.FromContextOrDiscard(ctx)
	mode := h.lookupLintingMode(ctx, apiSpec.Spec.Category)

	// Linting is pending (async) — Spec.Lint is nil, wait for rover-server to fill it in.
	if apiSpec.Spec.Lint == nil {
		if mode == apiapi.LintingModeNone {
			// No linting configured for this category — proceed normally.
			return h.createOrUpdateApi(ctx, apiSpec)
		}
		if mode == apiapi.LintingModeBlock {
			apiSpec.SetCondition(condition.NewNotReadyCondition("LintingPending",
				"API specification is being linted"))
			return nil
		}
		// warn mode: proceed without waiting for lint result
		log.V(0).Info("Linting pending in warn mode, proceeding with Api creation")
		return h.createOrUpdateApi(ctx, apiSpec)
	}

	// Check if linting failed and the category config blocks on failure
	if !apiSpec.Spec.Lint.Passed && mode == apiapi.LintingModeBlock {
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
	if h.GetApiCategory == nil {
		return apiapi.LintingModeNone
	}
	cat, err := h.GetApiCategory(ctx, category)
	if err != nil || cat == nil || cat.Spec.Linting == nil {
		return apiapi.LintingModeNone
	}
	mode := cat.Spec.Linting.Mode
	if mode == "" {
		mode = apiapi.LintingModeBlock
	}
	return mode
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
			Version:      apiSpec.Spec.Version,
			BasePath:     apiSpec.Spec.BasePath,
			Category:     apiSpec.Spec.Category,
			Oauth2Scopes: apiSpec.Spec.Oauth2Scopes,
			XVendor:      apiSpec.Spec.XVendor,
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
