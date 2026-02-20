// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package subscriber

import (
	"bytes"
	"context"

	"github.com/pkg/errors"
	cclient "github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/condition"
	cconfig "github.com/telekom/controlplane/common/pkg/config"
	"github.com/telekom/controlplane/common/pkg/errors/ctrlerrors"
	"github.com/telekom/controlplane/common/pkg/handler"
	"github.com/telekom/controlplane/common/pkg/util/contextutil"
	file_api "github.com/telekom/controlplane/file-manager/api"
	pubsubv1 "github.com/telekom/controlplane/pubsub/api/v1"
	"github.com/telekom/controlplane/pubsub/internal/handler/util"
	"github.com/telekom/controlplane/pubsub/internal/service"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

var _ handler.Handler[*pubsubv1.Subscriber] = &SubscriberHandler{}

type SubscriberHandler struct{}

func (h *SubscriberHandler) CreateOrUpdate(ctx context.Context, obj *pubsubv1.Subscriber) error {
	logger := log.FromContext(ctx)
	environment := contextutil.EnvFromContextOrDie(ctx)

	publisher, err := util.GetPublisher(ctx, obj.Spec.Publisher)
	if err != nil {
		return errors.Wrapf(err, "failed to resolve Publisher %q", obj.Spec.Publisher.String())
	}

	eventStore, err := util.GetEventStore(ctx, publisher.Spec.EventStore)
	if err != nil {
		return errors.Wrapf(err, "failed to resolve EventStore %q from Publisher %q", publisher.Spec.EventStore.String(), obj.Spec.Publisher.String())
	}

	if cconfig.FeatureFileManager.IsEnabled() {
		buf := bytes.NewBuffer(nil)
		res, err := file_api.GetFileManager().DownloadFile(ctx, publisher.Spec.JsonSchema, buf)
		if err != nil {
			return errors.Wrapf(err, "failed to download JSON schema from Publisher %q", obj.Spec.Publisher.String())
		}
		if res.ContentType != "application/json" {
			return ctrlerrors.BlockedErrorf("Expected content type application/json for JSON schema, got %q", res.ContentType)
		}
		// Set the downloaded JSON schema in the payload so it's included in the subscription resource sent to the configuration backend.
		publisher.Spec.JsonSchema = buf.String()
	}

	subscriptionID := getOrGenerateSubscriptionID(obj, environment, publisher.Spec.EventType)
	resource := BuildSubscriptionResource(obj, publisher, subscriptionID, environment)

	configSvc := getConfigService(eventStore)

	err = configSvc.PutSubscription(ctx, subscriptionID, resource)
	if err != nil {
		return errors.Wrap(err, "failed to register subscription in configuration backend")
	}

	logger.Info("Subscription registered in configuration backend",
		"subscriptionId", subscriptionID,
		"publisher", publisher.Name,
		"eventStore", eventStore.Name,
		"subscriberId", obj.Spec.SubscriberId)

	obj.Status.SubscriptionId = subscriptionID
	obj.SetCondition(condition.NewReadyCondition("SubscriberReady",
		"Subscription registered in configuration backend"))
	obj.SetCondition(condition.NewDoneProcessingCondition("Subscriber is ready"))

	return nil
}

func (h *SubscriberHandler) Delete(ctx context.Context, obj *pubsubv1.Subscriber) error {
	logger := log.FromContext(ctx)
	environment := contextutil.EnvFromContextOrDie(ctx)
	c := cclient.ClientFromContextOrDie(ctx)

	publisher := &pubsubv1.Publisher{}
	err := c.Get(ctx, obj.Spec.Publisher.K8s(), publisher)
	if err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("Publisher already deleted, skipping cleanup",
				"publisher", obj.Spec.Publisher.String(),
				"subscriberId", obj.Spec.SubscriberId)
			return nil
		}
		return errors.Wrapf(err, "failed to resolve Publisher %q during delete", obj.Spec.Publisher.String())
	}

	eventStore := &pubsubv1.EventStore{}
	err = c.Get(ctx, publisher.Spec.EventStore.K8s(), eventStore)
	if err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("EventStore already deleted, skipping cleanup",
				"eventStore", publisher.Spec.EventStore.String(),
				"subscriberId", obj.Spec.SubscriberId)
			return nil
		}
		return errors.Wrapf(err, "failed to resolve EventStore %q during delete", publisher.Spec.EventStore.String())
	}

	subscriptionID := getOrGenerateSubscriptionID(obj, environment, publisher.Spec.EventType)
	resource := BuildSubscriptionResource(obj, publisher, subscriptionID, environment)

	configSvc := getConfigService(eventStore)

	err = configSvc.DeleteSubscription(ctx, subscriptionID, resource)
	if err != nil {
		return errors.Wrap(err, "failed to deregister subscription from configuration backend")
	}

	logger.Info("Subscription deregistered from configuration backend",
		"subscriptionId", subscriptionID,
		"subscriberId", obj.Spec.SubscriberId)

	return nil
}

var getConfigService = func(eventStore *pubsubv1.EventStore) service.ConfigService {
	return service.NewConfigService(service.ConfigServiceConfig{
		BaseURL:      eventStore.Spec.Url,
		TokenURL:     eventStore.Spec.TokenUrl,
		ClientID:     eventStore.Spec.ClientId,
		ClientSecret: eventStore.Spec.ClientSecret,
	})
}

// getOrGenerateSubscriptionID returns the subscription ID from status if available,
// otherwise generates it deterministically from environment, eventType, and subscriberId.
func getOrGenerateSubscriptionID(obj *pubsubv1.Subscriber, environment, eventType string) string {
	if obj.Status.SubscriptionId != "" {
		return obj.Status.SubscriptionId
	}
	return GenerateSubscriptionID(environment, eventType, obj.Spec.SubscriberId)
}
