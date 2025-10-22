// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package remoteapisubscription

import (
	"context"

	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/telekom/controlplane/api/internal/handler/util"
	"github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/config"
	"github.com/telekom/controlplane/common/pkg/types"
	"github.com/telekom/controlplane/common/pkg/util/contextutil"
	"github.com/telekom/controlplane/common/pkg/util/labelutil"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	apiapi "github.com/telekom/controlplane/api/api/v1"
	applicationv1 "github.com/telekom/controlplane/application/api/v1"
)

// handleProviderScenario handles the case where the RemoteApiSubscription is handled by this CP.
// That means that the current CP needs to create an Application and an ApiSubscription.
func (h *RemoteApiSubscriptionHandler) handleProviderScenario(ctx context.Context, obj *apiapi.RemoteApiSubscription) (err error) {
	log := log.FromContext(ctx)
	c := client.ClientFromContextOrDie(ctx)

	// Handle it locally
	log.V(1).Info("I need to handle this locally")

	cleanup := func() error {
		n, err := c.CleanupAll(ctx, client.OwnedBy(obj))
		if err != nil {
			return errors.Wrapf(err, "failed to cleanup all resources")
		}
		log.V(1).Info("🧹 Cleaned up resources", "count", n)

		return nil
	}

	defer func() {
		if err != nil {
			return
		}
		err = cleanup()
	}()

	remoteOrgRef := types.ObjectRef{Name: obj.Spec.SourceOrganization, Namespace: contextutil.EnvFromContextOrDie(ctx)}
	remoteOrg, err := util.GetRemoteOrganization(ctx, remoteOrgRef)
	if err != nil {
		return errors.Wrapf(err, "failed to get remote organization %s", obj.Spec.SourceOrganization)
	}

	// Create Application

	application := &applicationv1.Application{
		ObjectMeta: metav1.ObjectMeta{
			Name:      obj.Spec.Requester.Application,
			Namespace: obj.Namespace,
		},
	}

	mutator := func() error {
		err := controllerutil.SetControllerReference(obj, application, c.Scheme())
		if err != nil {
			return errors.Wrapf(err, "failed to set owner reference")
		}

		application.Labels = map[string]string{
			config.BuildLabelKey("org.id"): remoteOrg.Spec.Id,
		}

		secret := application.Spec.Secret
		if secret == "" {
			secret = uuid.NewString()
		}

		application.Spec = applicationv1.ApplicationSpec{
			Team:          obj.Spec.Requester.Team.Name,
			TeamEmail:     obj.Spec.Requester.Team.Email,
			Zone:          CalculateRemoteOrgZone(remoteOrg),
			Secret:        secret,
			NeedsClient:   false,
			NeedsConsumer: true,
		}

		return nil
	}

	_, err = c.CreateOrUpdate(ctx, application, mutator)
	if err != nil {
		return errors.Wrapf(err, "failed to create application %s", application.Name)
	}
	obj.Status.Application = types.ObjectRefFromObject(application)

	_, err = c.Cleanup(ctx, &applicationv1.ApplicationList{}, client.OwnedBy(obj))
	if err != nil {
		return errors.Wrapf(err, "failed to cleanup applications")
	}

	// Create ApiSubscription

	apiSubscription := &apiapi.ApiSubscription{
		ObjectMeta: metav1.ObjectMeta{
			Name:      obj.Name,
			Namespace: obj.Namespace,
		},
	}

	mutator = func() error {
		err := controllerutil.SetControllerReference(obj, apiSubscription, c.Scheme())
		if err != nil {
			return errors.Wrapf(err, "failed to set owner reference")
		}
		apiSubscription.Labels = map[string]string{
			apiapi.BasePathLabelKey:             labelutil.NormalizeLabelValue(obj.Spec.ApiBasePath),
			config.BuildLabelKey("application"): application.Name,
		}

		apiSubscription.Spec = apiapi.ApiSubscriptionSpec{
			ApiBasePath:  obj.Spec.ApiBasePath,
			Security:     obj.Spec.Security,
			Organization: "",
			Requestor: apiapi.Requestor{
				Application: *types.ObjectRefFromObject(application),
			},
			Zone: CalculateRemoteOrgZone(remoteOrg),
		}

		return nil
	}

	res, err := c.CreateOrUpdate(ctx, apiSubscription, mutator)
	if err != nil {
		return errors.Wrapf(err, "failed to create api subscription %s", apiSubscription.Name)
	}
	obj.Status.ApiSubscription = types.ObjectRefFromObject(apiSubscription)

	_, err = c.Cleanup(ctx, &apiapi.ApiSubscriptionList{}, client.OwnedBy(obj))
	if err != nil {
		return errors.Wrapf(err, "failed to cleanup api subscriptions")
	}

	obj.SetCondition(condition.NewProcessingCondition("Processing", "Processing RemoteApiSubscription"))
	obj.SetCondition(condition.NewNotReadyCondition("Processing", "Processing RemoteApiSubscription"))

	// Check if the ApiSubscription is ready

	if res == controllerutil.OperationResultNone {
		// No update occurred

		if err = fillApprovalRequestInfo(ctx, obj, apiSubscription); err != nil {
			return errors.Wrapf(err, "failed to fill approvalrequest info")
		}

		if err = fillApprovalInfo(ctx, obj, apiSubscription); err != nil {
			return errors.Wrapf(err, "failed to fill approval info")
		}

		// check if ready
		if meta.IsStatusConditionTrue(apiSubscription.GetConditions(), condition.ConditionTypeReady) {
			obj.SetCondition(condition.NewReadyCondition("Ready", "RemoteApiSubscription is ready"))
			obj.SetCondition(condition.NewDoneProcessingCondition("RemoteApiSubscription is done processing"))

			if err = fillRouteInfo(ctx, obj, apiSubscription); err != nil {
				return errors.Wrapf(err, "failed to fill route info")
			}

		} else {
			obj.Status.Conditions = apiSubscription.Status.Conditions // TODO: good idea?
		}
	}

	// Send current state to other CP

	updated, _, err := h.SyncerFactory.NewClient(remoteOrg).SendStatus(ctx, obj)
	if err != nil {
		return errors.Wrapf(err, "failed to send status to remote CP")
	}
	if updated {
		log.Info("🔄 RemoteApiSubscription status updated")
	}

	return nil
}
