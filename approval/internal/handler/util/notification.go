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
)

func SendNotification(ctx context.Context, owner client.Object, sendToChannelNamespace, state string, target *types.TypedObjectRef, requester *approvalv1.Requester) (*types.ObjectRef, error) {
	properties := map[string]any{
		"environment": contextutil.EnvFromContextOrDie(ctx),
		"state":       state,
		"target":      target.DeepCopy(),
		"requester":   requester.DeepCopy(),
	}

	notificationBuilder := builder.New().
		WithOwner(owner).
		WithSender(notificationv1.SenderTypeSystem, "ApprovalService").
		WithDefaultChannels(ctx, sendToChannelNamespace).
		WithPurpose(strings.ToLower(owner.GetObjectKind().GroupVersionKind().Kind) + "--" + owner.GetName()).
		WithProperties(properties)

	notification, err := notificationBuilder.Send(ctx)
	if err != nil {
		return nil, err
	}
	return types.ObjectRefFromObject(notification), nil
}
