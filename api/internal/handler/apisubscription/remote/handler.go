// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package remote

import (
	"context"

	"github.com/pkg/errors"
	apiapi "github.com/telekom/controlplane/api/api/v1"
	"github.com/telekom/controlplane/api/internal/handler/util"
	cclient "github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/config"
	"github.com/telekom/controlplane/common/pkg/types"
	"github.com/telekom/controlplane/common/pkg/util/contextutil"
	gatewayapi "github.com/telekom/controlplane/gateway/api/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// IsRemoteApiSubscription checks if the given object is a remote api subscription.
// It is considered a remote api subscription if the organization is set.
func IsRemoteApiSubscription(obj *apiapi.ApiSubscription) bool {
	return obj.Spec.Organization != ""
}

func HandleRemoteApiSubscription(ctx context.Context, owner *apiapi.ApiSubscription) (err error) {
	log := log.FromContext(ctx)
	c := cclient.ClientFromContextOrDie(ctx)

	cleanup := func() error {
		n, err := c.CleanupAll(ctx, cclient.OwnedBy(owner))
		if err != nil {
			return errors.Wrapf(err, "failed to cleanup all resources")
		}
		log.Info("🧹 Cleaned up resources", "count", n)
		return nil
	}

	defer func() {
		if err != nil {
			return
		}
		err = cleanup()
	}()

	req := &apiapi.RemoteApiSubscription{
		ObjectMeta: metav1.ObjectMeta{
			Name:      owner.Name,
			Namespace: owner.Namespace,
		},
	}

	application, err := util.GetApplication(ctx, owner.Spec.Requestor.Application)
	if err != nil {
		return errors.Wrapf(err,
			"unable to get application %s for Apisubscription", owner.Spec.Requestor.Application.String())
	}

	if !meta.IsStatusConditionTrue(application.GetConditions(), condition.ConditionTypeReady) {
		log.Info("🫷 Application not ready")
		return nil
	}

	mutate := func() error {
		err := controllerutil.SetControllerReference(owner, req, c.Scheme())
		if err != nil {
			return errors.Wrapf(err, "failed to set controller reference")
		}
		req.Labels = owner.Labels
		req.Labels[config.BuildLabelKey("org.id")] = owner.Spec.Organization

		req.Spec = apiapi.RemoteApiSubscriptionSpec{
			ApiBasePath:        owner.Spec.ApiBasePath,
			Security:           owner.Spec.Security,
			TargetOrganization: owner.Spec.Organization,
			SourceOrganization: "", // TODO: what is the value here?
			Requester: apiapi.RemoteRequester{
				Application: owner.Spec.Requestor.Application.Name,
				Team: apiapi.RemoteTeam{
					Name:  application.Spec.Team,
					Email: application.Spec.TeamEmail,
				},
			},
		}
		return nil
	}

	res, err := c.CreateOrUpdate(ctx, req, mutate)
	if err != nil {
		return errors.Wrapf(err, "failed to create or update remote api subscription")
	}
	owner.Status.RemoteApiSubscription = types.ObjectRefFromObject(req)

	owner.SetCondition(condition.NewProcessingCondition("RemoteApiSubscriptionPending", "RemoteApiSubscription created or updated"))
	owner.SetCondition(condition.NewNotReadyCondition("NotReady", "RemoteApiSubscription is provisoned but not ready yet"))

	if res != controllerutil.OperationResultNone {
		log.Info("🔗 RemoteApiSubscription created or updated", "result", res)
		return nil
	}

	approvalResult := IsApproved(ctx, req)

	if approvalResult == ApprovalResultCleanup {
		log.Info("🧹 RemoteApiSubscription not granted. We need to cleanup")
		owner.SetCondition(condition.NewBlockedCondition("RemoteApiSubscription not granted"))
		owner.SetCondition(condition.NewNotReadyCondition("RemoteApiSubscriptionNotGranted", "RemoteApiSubscription not granted"))

		err = util.CleanupProxyRoute(ctx, owner.Status.Route)
		if err != nil {
			return errors.Wrapf(err, "failed to cleanup proxy route")
		}
		return nil
	}

	if approvalResult == ApprovalResultBlock {
		log.Info("🫷 RemoteApiSubscription not approved yet")
		owner.SetCondition(condition.NewBlockedCondition("RemoteApiSubscription not granted yet"))
		return nil
	}

	if !meta.IsStatusConditionTrue(req.GetConditions(), condition.ConditionTypeReady) {
		log.Info("🫷 RemoteApiSubscription not ready")
		owner.SetCondition(condition.NewBlockedCondition("RemoteApiSubscription is granted but not rerady yet"))
		return nil
	}

	log.Info("👌 RemoteApiSubscription ready, we will continue")
	owner.SetCondition(condition.NewProcessingCondition("RemoteApiSubscriptionReady", "Continue processing"))

	remoteOrg, err := util.GetRemoteOrganization(ctx, types.ObjectRef{
		Name:      req.Spec.TargetOrganization,
		Namespace: contextutil.EnvFromContextOrDie(ctx),
	})
	if err != nil {
		return errors.Wrapf(err, "failed to get remote organization %s", req.Spec.TargetOrganization)
	}

	subscriptionZone, err := util.GetZone(ctx, c, owner.Spec.Zone.K8s())
	if err != nil {
		return err
	}

	// Create the Route
	// If the remote organization is in a different zone, we need to create a proxy-route
	// If the remote organization is in the same zone, we can use the real route
	// which was already created

	var routeRef *types.ObjectRef
	if !remoteOrg.Spec.Zone.Equals(subscriptionZone) {
		log.Info("RemoteApiSubscription is in a different zone")
		// We need to create a proxy-route
		owner.SetCondition(condition.NewProcessingCondition("CreatingProxyRoute", "Creating proxy route"))

		route, err := util.CreateProxyRoute(ctx, owner.Spec.Zone, remoteOrg.Spec.Zone, owner.Spec.ApiBasePath, remoteOrg.Spec.Id)
		if err != nil {
			return errors.Wrapf(err, "failed to create proxy route")
		}

		routeRef = types.ObjectRefFromObject(route)
	} else {
		log.Info("RemoteApiSubscription is in the same zone")
		// Set route as RealRoute
		routeRef = req.Status.Route
	}
	owner.Status.Route = routeRef

	// Create a RouteConsumer to consume the route
	owner.SetCondition(condition.NewProcessingCondition("CreatingConsumeRoute", "Creating consume route"))

	routeConsumer := &gatewayapi.ConsumeRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      owner.Name,
			Namespace: owner.Namespace,
		},
	}

	mutate = func() error {
		if err := controllerutil.SetControllerReference(owner, routeConsumer, c.Scheme()); err != nil {
			return errors.Wrapf(err, "failed to set owner-reference on %v", routeConsumer)
		}
		routeConsumer.Labels = owner.Labels

		routeConsumer.Spec = gatewayapi.ConsumeRouteSpec{
			Route:        *routeRef,
			ConsumerName: application.Status.ClientId,
		}
		return nil

	}

	_, err = c.CreateOrUpdate(ctx, routeConsumer, mutate)
	if err != nil {
		return errors.Wrapf(err, "failed to create consume route")
	}

	owner.Status.ConsumeRoute = types.ObjectRefFromObject(routeConsumer)
	owner.SetCondition(condition.NewDoneProcessingCondition("Successfully provisioned subresources"))
	owner.SetCondition(condition.NewReadyCondition("Ready", "ApiSubscription is ready"))
	return nil
}
