// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package eventspecification

import (
	"context"

	"github.com/pkg/errors"
	"github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/handler"
	"github.com/telekom/controlplane/common/pkg/types"
	"github.com/telekom/controlplane/common/pkg/util/labelutil"
	eventv1 "github.com/telekom/controlplane/event/api/v1"
	roverv1 "github.com/telekom/controlplane/rover/api/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

var _ handler.Handler[*roverv1.EventSpecification] = (*EventSpecificationHandler)(nil)

type EventSpecificationHandler struct{}

func (h *EventSpecificationHandler) CreateOrUpdate(ctx context.Context, eventSpec *roverv1.EventSpecification) error {

	c := client.ClientFromContextOrDie(ctx)
	name := roverv1.MakeEventSpecificationName(eventSpec)

	eventType := &eventv1.EventType{
		ObjectMeta: metav1.ObjectMeta{
			Name:      labelutil.NormalizeNameValue(name),
			Namespace: eventSpec.Namespace,
		},
	}

	eventSpec.Status.EventType = *types.ObjectRefFromObject(eventType)

	mutator := func() error {
		err := controllerutil.SetControllerReference(eventSpec, eventType, c.Scheme())
		if err != nil {
			return errors.Wrap(err, "failed to set controller reference")
		}

		eventType.Labels = map[string]string{
			eventv1.EventTypeLabelKey: labelutil.NormalizeLabelValue(eventSpec.Spec.Type),
		}

		eventType.Spec = eventv1.EventTypeSpec{
			Type:          eventSpec.Spec.Type,
			Version:       eventSpec.Spec.Version,
			Description:   eventSpec.Spec.Description,
			Specification: eventSpec.Spec.Specification,
		}

		return nil
	}

	_, err := c.CreateOrUpdate(ctx, eventType, mutator)
	if err != nil {
		return errors.Wrap(err, "failed to create or update EventType")
	}

	if c.AnyChanged() {
		eventSpec.SetCondition(condition.NewProcessingCondition("Provisioning", "EventType updated"))
		eventSpec.SetCondition(condition.NewNotReadyCondition("Provisioning", "EventType is not ready"))

	} else {
		eventSpec.SetCondition(condition.NewDoneProcessingCondition("EventType created"))
		eventSpec.SetCondition(condition.NewReadyCondition("Provisioned", "EventType is ready"))
	}

	return nil
}

func (h *EventSpecificationHandler) Delete(ctx context.Context, obj *roverv1.EventSpecification) error {
	return nil
}
