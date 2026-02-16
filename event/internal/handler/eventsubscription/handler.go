// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package eventsubscription

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
	applicationv1 "github.com/telekom/controlplane/application/api/v1"
	approvalapi "github.com/telekom/controlplane/approval/api/v1"
	"github.com/telekom/controlplane/approval/api/v1/builder"
	cclient "github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/config"
	"github.com/telekom/controlplane/common/pkg/errors/ctrlerrors"
	"github.com/telekom/controlplane/common/pkg/handler"
	"github.com/telekom/controlplane/common/pkg/types"
	"github.com/telekom/controlplane/common/pkg/util/labelutil"
	eventv1 "github.com/telekom/controlplane/event/api/v1"
	"github.com/telekom/controlplane/event/internal/handler/util"
	pubsubv1 "github.com/telekom/controlplane/pubsub/api/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

var _ handler.Handler[*eventv1.EventSubscription] = &EventSubscriptionHandler{}

type EventSubscriptionHandler struct{}

func (h *EventSubscriptionHandler) CreateOrUpdate(ctx context.Context, obj *eventv1.EventSubscription) error {
	logger := log.FromContext(ctx)
	c := cclient.ClientFromContextOrDie(ctx)

	found, _, err := util.FindActiveEventType(ctx, obj.Spec.EventType)
	if err != nil {
		return err
	}
	if !found {
		obj.SetCondition(condition.NewNotReadyCondition("EventTypeNotFound",
			"No active EventType found for type "+obj.Spec.EventType))
		obj.SetCondition(condition.NewBlockedCondition(
			"EventType " + obj.Spec.EventType + " does not exist or is not active. " +
				"EventSubscription will be automatically processed when the EventType is registered"))
		return nil
	}

	exposures, err := util.FindEventExposures(ctx, obj.Spec.EventType)
	if err != nil {
		return err
	}

	if len(exposures) == 0 {
		// no exposure found, cleanup
		deleted, err := c.Cleanup(ctx, &pubsubv1.SubscriberList{}, cclient.OwnedBy(obj))
		if err != nil {
			return errors.Wrapf(err, "unable to cleanup Subscriber for EventSubscription %q in namespace %q",
				obj.Name, obj.Namespace)
		}
		logger.Info("No EventExposure found for event type — cleaned up Subscriber resources", "deleted", deleted)
	}

	exposureFound, exposure, err := util.FindActiveEventExposure(exposures)
	if err != nil {
		return errors.Wrapf(err, "failed to find active EventExposure for event type %q", obj.Spec.EventType)
	}

	if !exposureFound {
		obj.SetCondition(condition.NewNotReadyCondition("EventExposureNotFound",
			"No active EventExposure found for type "+obj.Spec.EventType))
		obj.SetCondition(condition.NewBlockedCondition(
			"EventExposure for " + obj.Spec.EventType + " does not exist or is not active. " +
				"EventSubscription will be automatically processed when the EventExposure is registered"))
		return nil
	}

	// TODO: Validate category — check if the subscriber's team category allows subscription of this event category
	// TODO: Validate visibility — check if subscription zone is compatible with exposure visibility
	// TODO: Validate scopes — check if requested scopes are a subset of exposure scopes

	exposureEventConfig, err := util.GetEventConfigForZone(ctx, exposure.Spec.Zone.Name)
	if err != nil {
		return errors.Wrapf(err, "failed to get EventConfig for exposure zone %q", exposure.Spec.Zone.Name)
	}

	if !exposureEventConfig.SupportsZone(obj.Spec.Zone.Name) {
		obj.SetCondition(condition.NewNotReadyCondition("ZoneNotSupported",
			fmt.Sprintf("EventConfig for zone %q does not support this subscription zone", exposure.Spec.Zone.Name)))
		obj.SetCondition(condition.NewBlockedCondition(
			fmt.Sprintf("EventConfig for zone %q does not support this subscription zone. "+
				"EventSubscription will be automatically processed when an EventConfig that supports the subscription zone is registered",
				exposure.Spec.Zone.Name)))

		return nil
	}

	subscriberEventConfig, err := util.GetEventConfigForZone(ctx, obj.Spec.Zone.Name)
	if err != nil {
		return errors.Wrapf(err, "failed to get EventConfig for subscription zone %q", obj.Spec.Zone.Name)
	}

	// We need to update the callbackURL if this is a proxy scenario (cross-zone subscription), because the callbackURL must point to the provider zone's callback route.
	isProxy := !exposure.Spec.Zone.Equals(&obj.Spec.Zone)
	isCallback := obj.Spec.Delivery.Type == eventv1.DeliveryTypeCallback
	if isCallback && isProxy {
		rawProxyCallbackUrl, ok := subscriberEventConfig.Status.ProxyCallbackURLs[exposure.Spec.Zone.Name]
		if !ok {
			return ctrlerrors.BlockedErrorf("no proxy callback URL found in subscription zone's EventConfig for exposure zone %q", exposure.Spec.Zone.Name)
		}
		// Use proxyCallbackUrl as new callback URL and add actual callback URL as query parameter so that provider can use it for callbacks.
		obj.Spec.Delivery.Callback = rawProxyCallbackUrl + "?" + util.CallbackUrlQUeryParam + "=" + obj.Spec.Delivery.Callback

		logger.V(1).Info("Updated callback URL for proxy scenario", "callback", obj.Spec.Delivery.Callback)
	}

	if obj.Spec.Requestor.Kind != "Application" {
		obj.SetCondition(condition.NewNotReadyCondition("InvalidRequestor",
			"Only requestors of kind 'Application' are supported"))
		obj.SetCondition(condition.NewBlockedCondition(
			"EventSubscription with requestor kind " + obj.Spec.Requestor.Kind + " is not supported"))
		return nil
	}
	requestorApp, err := util.GetApplication(ctx, obj.Spec.Requestor.ObjectRef)
	if err != nil {
		return err
	}

	providerApp, err := util.GetApplication(ctx, exposure.Spec.Provider.ObjectRef)
	if err != nil {
		return errors.Wrapf(err, "unable to get application from EventExposure provider %q while handling EventSubscription %q",
			exposure.Spec.Provider.Name, obj.Name)
	}

	requester := &approvalapi.Requester{
		TeamName:       requestorApp.Spec.Team,
		TeamEmail:      requestorApp.Spec.TeamEmail,
		ApplicationRef: types.TypedObjectRefFromObject(requestorApp, c.Scheme()),
		Reason: fmt.Sprintf("Team %s requested subscription to event %s from zone %s",
			requestorApp.Spec.Team, obj.Spec.EventType, obj.Spec.Zone.Name),
	}

	properties := map[string]any{
		"eventType": obj.Spec.EventType,
		"scopes":    obj.Spec.Scopes,
	}
	if err := requester.SetProperties(properties); err != nil {
		return errors.Wrapf(err, "unable to set approvalRequest properties for EventSubscription %q in namespace %q",
			obj.Name, obj.Namespace)
	}

	decider := &approvalapi.Decider{
		TeamName:       providerApp.Spec.Team,
		TeamEmail:      providerApp.Spec.TeamEmail,
		ApplicationRef: types.TypedObjectRefFromObject(providerApp, c.Scheme()),
	}

	approvalBuilder := builder.NewApprovalBuilder(c, obj)
	approvalBuilder.WithAction("subscribe")
	approvalBuilder.WithHashValue(requester.Properties)
	approvalBuilder.WithRequester(requester)
	approvalBuilder.WithDecider(decider)
	approvalBuilder.WithStrategy(approvalapi.ApprovalStrategy(exposure.Spec.Approval.Strategy))

	if len(exposure.Spec.Approval.TrustedTeams) > 0 {
		approvalBuilder.WithTrustedRequesters(exposure.Spec.Approval.TrustedTeams)
	}

	res, err := approvalBuilder.Build(ctx)
	if err != nil {
		return err
	}
	obj.Status.ApprovalRequest = types.ObjectRefFromObject(approvalBuilder.GetApprovalRequest())
	obj.Status.Approval = types.ObjectRefFromObject(approvalBuilder.GetApproval())

	switch res {
	case builder.ApprovalResultRequestDenied:
		logger.Info("ApprovalRequest was denied — not touching child resources")
		obj.SetCondition(condition.NewNotReadyCondition("ApprovalRequestDenied", "ApprovalRequest has been denied"))
		obj.SetCondition(condition.NewDoneProcessingCondition("ApprovalRequest has been denied"))
		return nil

	case builder.ApprovalResultPending:
		logger.Info("Approval is pending — waiting for approval")
		obj.SetCondition(condition.NewNotReadyCondition("ApprovalPending", "Approval has not been approved"))
		obj.SetCondition(condition.NewBlockedCondition("Approval has not been approved"))
		return nil

	case builder.ApprovalResultDenied:
		logger.Info("Approval was denied — cleaning up Subscriber")
		obj.SetCondition(condition.NewNotReadyCondition("ApprovalDenied", "Approval has been denied"))
		obj.SetCondition(condition.NewDoneProcessingCondition("Approval has been denied"))

		deleted, err := c.Cleanup(ctx, &pubsubv1.SubscriberList{}, cclient.OwnedBy(obj))
		if err != nil {
			return errors.Wrapf(err, "unable to cleanup Subscriber for EventSubscription %q in namespace %q",
				obj.Name, obj.Namespace)
		}
		logger.Info("Cleaned up Subscriber resources", "deleted", deleted)
		return nil

	case builder.ApprovalResultGranted:
		logger.Info("Approval is granted — continuing with provisioning")

	default:
		return errors.Errorf("unknown approval-builder result %q", res)
	}

	if exposure.Status.Publisher == nil {
		obj.SetCondition(condition.NewNotReadyCondition("PublisherNotReady",
			"EventExposure does not have a Publisher reference yet"))
		obj.SetCondition(condition.NewBlockedCondition(
			"EventExposure " + exposure.Name + " has no Publisher reference. " +
				"EventSubscription will be automatically processed when the Publisher is created"))
		return nil
	}

	subscriber, err := h.createSubscriber(ctx, obj, requestorApp, exposure)
	if err != nil {
		return errors.Wrap(err, "failed to create Subscriber")
	}
	obj.Status.Subscriber = types.ObjectRefFromObject(subscriber)

	if obj.Spec.Delivery.Type == eventv1.DeliveryTypeServerSentEvent {
		obj.Status.URL = exposure.Status.SseURLs[obj.Spec.Zone.Name]
	}

	logger.V(1).Info("Subscriber created/updated", "subscriber", subscriber.Name)

	if !c.AllReady() {
		obj.SetCondition(condition.NewNotReadyCondition("ChildResourcesNotReady",
			"One or more child resources are not yet ready"))
		obj.SetCondition(condition.NewDoneProcessingCondition("Waiting for child resources"))
		return nil
	}

	obj.SetCondition(condition.NewReadyCondition("EventSubscriptionProvisioned",
		"EventSubscription has been provisioned"))
	obj.SetCondition(condition.NewDoneProcessingCondition(
		"EventSubscription has been provisioned"))

	return nil
}

func (h *EventSubscriptionHandler) Delete(ctx context.Context, obj *eventv1.EventSubscription) error {
	// Child resources (Subscriber, ApprovalRequest) are cleaned up via owner references.
	// No additional manual cleanup needed.
	return nil
}

// createSubscriber creates a pubsub.Subscriber child resource for this EventSubscription.
func (h *EventSubscriptionHandler) createSubscriber(
	ctx context.Context,
	obj *eventv1.EventSubscription,
	application *applicationv1.Application,
	exposure *eventv1.EventExposure,
) (*pubsubv1.Subscriber, error) {
	c := cclient.ClientFromContextOrDie(ctx)

	subscriber := &pubsubv1.Subscriber{
		ObjectMeta: metav1.ObjectMeta{
			Name:      labelutil.NormalizeNameValue(obj.Name),
			Namespace: obj.Namespace,
		},
	}

	mutator := func() error {
		if err := controllerutil.SetControllerReference(obj, subscriber, c.Scheme()); err != nil {
			return errors.Wrap(err, "failed to set controller reference")
		}

		subscriber.Labels = map[string]string{
			config.BuildLabelKey("application"): labelutil.NormalizeLabelValue(application.Name),
			eventv1.EventTypeLabelKey:           labelutil.NormalizeLabelValue(obj.Spec.EventType),
			config.BuildLabelKey("zone"):        obj.Spec.Zone.Name,
		}

		subscriber.Spec = pubsubv1.SubscriberSpec{
			Publisher:    *exposure.Status.Publisher,
			SubscriberId: application.Status.ClientId,
			Delivery:     mapDelivery(obj.Spec.Delivery),
			Trigger:      mapTrigger(obj.Spec.Trigger),
			// TODO: Populate PublisherTrigger from matching scope on EventExposure
			PublisherTrigger: nil,
			AppliedScopes:    obj.Spec.Scopes,
		}
		return nil
	}

	_, err := c.CreateOrUpdate(ctx, subscriber, mutator)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create or update Subscriber %s", obj.Name)
	}

	return subscriber, nil
}

// mapDelivery maps event domain Delivery to pubsub domain SubscriptionDelivery.
func mapDelivery(d eventv1.Delivery) pubsubv1.SubscriptionDelivery {
	return pubsubv1.SubscriptionDelivery{
		Type:                  pubsubv1.DeliveryType(d.Type),
		Payload:               pubsubv1.PayloadType(d.Payload),
		Callback:              d.Callback,
		EventRetentionTime:    d.EventRetentionTime,
		CircuitBreakerOptOut:  d.CircuitBreakerOptOut,
		RetryableStatusCodes:  d.RetryableStatusCodes,
		RedeliveriesPerSecond: d.RedeliveriesPerSecond,
		EnforceGetHttpRequestMethodForHealthCheck: d.EnforceGetHttpRequestMethodForHealthCheck,
	}
}

// mapTrigger maps event domain EventTrigger to pubsub domain SubscriptionTrigger.
func mapTrigger(t *eventv1.EventTrigger) *pubsubv1.SubscriptionTrigger {
	if t == nil {
		return nil
	}

	result := &pubsubv1.SubscriptionTrigger{}

	if t.ResponseFilter != nil {
		result.ResponseFilter = &pubsubv1.SubscriptionResponseFilter{
			Paths: t.ResponseFilter.Paths,
			Mode:  pubsubv1.ResponseFilterMode(t.ResponseFilter.Mode),
		}
	}

	if t.SelectionFilter != nil {
		result.SelectionFilter = &pubsubv1.SubscriptionSelectionFilter{
			Attributes: t.SelectionFilter.Attributes,
			Expression: t.SelectionFilter.Expression,
		}
	}

	return result
}
