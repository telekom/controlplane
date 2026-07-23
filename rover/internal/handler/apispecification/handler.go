// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package apispecification

import (
	"context"

	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	apiapi "github.com/telekom/controlplane/api/api/v1"
	"github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/handler"
	"github.com/telekom/controlplane/common/pkg/types"
	"github.com/telekom/controlplane/common/pkg/util/labelutil"
	roverv1 "github.com/telekom/controlplane/rover/api/v1"
)

var _ handler.Handler[*roverv1.ApiSpecification] = (*ApiSpecificationHandler)(nil)

// ApiSpecificationHandler reconciles ApiSpecification resources.
// Linting is enforced by rover-server at upload time; specs that fail linting
// in block mode are rejected and never stored in the cluster.
// This handler creates the downstream Api resource unconditionally.
type ApiSpecificationHandler struct{}

func (h *ApiSpecificationHandler) CreateOrUpdate(ctx context.Context, apiSpec *roverv1.ApiSpecification) error {
	return h.createOrUpdateApi(ctx, apiSpec)
}

func (h *ApiSpecificationHandler) Delete(_ context.Context, _ *roverv1.ApiSpecification) error {
	return nil
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
		apiSpec.SetCondition(condition.NewProcessingCondition(condition.ReasonProvisioning, "API updated"))
		apiSpec.SetCondition(condition.NewNotReadyCondition(condition.ReasonProvisioning, "API is not ready"))
	} else {
		apiSpec.SetCondition(condition.NewDoneProcessingCondition("API created"))
		apiSpec.SetCondition(condition.NewReadyCondition(condition.ReasonProvisioned, "API is ready"))
	}

	return nil
}
