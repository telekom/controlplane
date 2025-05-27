// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package apispecification

import (
	"context"

	"github.com/pkg/errors"
	"github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/handler"
	"github.com/telekom/controlplane/common/pkg/types"
	"github.com/telekom/controlplane/common/pkg/util/labelutil"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	apiapi "github.com/telekom/controlplane/api/api/v1"
	rover "github.com/telekom/controlplane/rover/api/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ handler.Handler[*rover.ApiSpecification] = (*ApiSpecificationHandler)(nil)

type ApiSpecificationHandler struct{}

func (h *ApiSpecificationHandler) CreateOrUpdate(ctx context.Context, apiSpec *rover.ApiSpecification) error {

	log := log.FromContext(ctx)
	log.Info("ApiSpecificationHandler CreateOrUpdate", "apispecification", apiSpec)

	apiSpec.SetCondition(condition.NewProcessingCondition("Provisioning", "Provisioning API"))

	c := client.ClientFromContextOrDie(ctx)

	// Parsing APiSpecification
	log.Info("Parsing APiSpecification", "apispecification", apiSpec)
	parsedApi, err := ParseSpecification(ctx, apiSpec.Spec.Specification)
	if err != nil {
		return err
	}

	api := &apiapi.Api{
		ObjectMeta: metav1.ObjectMeta{
			Name:      parsedApi.Name,
			Namespace: apiSpec.Namespace,
		},
	}

	apiSpec.Status.BasePath = parsedApi.Spec.BasePath
	apiSpec.Status.Category = parsedApi.Spec.Category
	apiSpec.Status.Api = *types.ObjectRefFromObject(api)

	mutator := func() error {
		err := controllerutil.SetControllerReference(apiSpec, api, c.Scheme())
		if err != nil {
			log.Error(err, "❌ failed to set controller reference")
			return errors.Wrap(err, "failed to set controller reference")
		}

		api.Labels = map[string]string{
			apiapi.BasePathLabelKey: labelutil.NormalizeValue(parsedApi.Spec.BasePath),
		}

		api.Spec = parsedApi.Spec

		return nil
	}

	_, err = c.CreateOrUpdate(ctx, api, mutator)
	if err != nil {
		log.Error(err, "❌ failed to create or update api")
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

func (h *ApiSpecificationHandler) Delete(ctx context.Context, obj *rover.ApiSpecification) error {
	return nil
}
