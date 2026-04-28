// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package apispecification

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	apiapi "github.com/telekom/controlplane/api/api/v1"
	"github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/config"
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
	ListZones func(ctx context.Context, environment string) (*adminv1.ZoneList, error)
}

func (h *ApiSpecificationHandler) CreateOrUpdate(ctx context.Context, apiSpec *roverv1.ApiSpecification) error {
	log := logr.FromContextOrDiscard(ctx)

	// Linting is pending (async) — set processing condition and wait for the result.
	if apiSpec.Status.LintPassed == nil && apiSpec.Status.LintReason == "Linting in progress" {
		apiSpec.SetCondition(condition.NewProcessingCondition("LintingPending",
			"OAS linting is in progress, waiting for result"))
		apiSpec.SetCondition(condition.NewNotReadyCondition("LintingPending",
			"API specification is being linted"))
		log.V(0).Info("Linting in progress, waiting for result")
		return nil
	}

	// Check if linting failed and the zone config blocks on failure
	if apiSpec.Status.LintPassed != nil && !*apiSpec.Status.LintPassed {
		environment := apiSpec.Labels[config.EnvironmentLabelKey]
		mode := h.lookupLintingMode(ctx, environment)
		if mode == adminv1.LintingModeBlock {
			msg := fmt.Sprintf("OAS linting failed: %s", apiSpec.Status.LintReason)
			if apiSpec.Status.LintDashboardURL != "" {
				msg = fmt.Sprintf("%s. View details: %s", msg, apiSpec.Status.LintDashboardURL)
			}
			apiSpec.SetCondition(condition.NewBlockedCondition(msg))
			apiSpec.SetCondition(condition.NewNotReadyCondition("LintingFailed",
				"API specification did not pass linting"))
			log.V(0).Info("Linting failed in block mode, skipping Api creation",
				"reason", apiSpec.Status.LintReason, "errors", apiSpec.Status.LintErrors)
			return nil
		}
		// warn mode: log and continue
		log.V(0).Info("Linting failed in warn mode, proceeding with Api creation",
			"reason", apiSpec.Status.LintReason, "errors", apiSpec.Status.LintErrors)
	}

	return h.createOrUpdateApi(ctx, apiSpec)
}

func (h *ApiSpecificationHandler) Delete(_ context.Context, _ *roverv1.ApiSpecification) error {
	return nil
}

// lookupLintingMode finds the Zone in the environment and returns the effective linting mode.
// Defaults to LintingModeBlock if any zone has linting enabled but no explicit mode.
func (h *ApiSpecificationHandler) lookupLintingMode(ctx context.Context, environment string) adminv1.LintingMode {
	if h.ListZones == nil {
		return adminv1.LintingModeBlock
	}
	zones, err := h.ListZones(ctx, environment)
	if err != nil || zones == nil {
		return adminv1.LintingModeBlock
	}
	for i := range zones.Items {
		zone := &zones.Items[i]
		if zone.Spec.Linting != nil && zone.Spec.Linting.Enabled {
			mode := zone.Spec.Linting.Mode
			if mode == "" {
				mode = adminv1.LintingModeBlock
			}
			return mode
		}
	}
	return adminv1.LintingModeBlock
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
