// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package util

import (
	"context"
	"strings"

	approvalv1 "github.com/telekom/controlplane/approval/api/v1"
	"github.com/telekom/controlplane/common/pkg/types"
	"github.com/telekom/controlplane/common/pkg/util/contextutil"
	notificationv1 "github.com/telekom/controlplane/notification/api/v1"
	"github.com/telekom/controlplane/notification/api/v1/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

func SendNotification(ctx context.Context, owner client.Object, state string, target *types.TypedObjectRef, requester *approvalv1.Requester) (*types.ObjectRef, error) {
	log := log.FromContext(ctx)

	properties := map[string]any{
		"environment": contextutil.EnvFromContextOrDie(ctx),
		"state":       state,
	}

	switch target.Kind {
	case "ApiSubscription":
		approvalProperties, err := requester.GetProperties()
		if err != nil || approvalProperties == nil {
			return nil, err
		}
		properties["API"] = approvalProperties["basePath"]
		scopes, ok := approvalProperties["scopes"]
		if !ok {
			scopes = "no scopes"
		} else {
			scopesStr, _ := scopes.(string)
			if scopesStr == "" || strings.EqualFold(scopesStr, "null") || strings.EqualFold(scopesStr, "nil") {
				scopes = "no scopes"
			}
		}
		properties["Oauth-Scopes"] = scopes
	default:
		log.V(1).Info("unknown resource kind", "kind", target.Kind)
	}

	notificationBuilder := builder.New().
		WithOwner(owner).
		WithSender(notificationv1.SenderTypeSystem, "ApprovalService").
		WithDefaultChannels(ctx, owner.GetNamespace()).
		WithPurpose(strings.ToLower(owner.GetObjectKind().GroupVersionKind().Kind) + "--" + owner.GetName()).
		WithProperties(properties)

	notification, err := notificationBuilder.Send(ctx)
	if err != nil {
		return nil, err
	}
	return types.ObjectRefFromObject(notification), nil
}
