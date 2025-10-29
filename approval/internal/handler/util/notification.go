// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package util

import (
	"context"
	"encoding/json"
	"strings"

	approvalv1 "github.com/telekom/controlplane/approval/api/v1"
	"github.com/telekom/controlplane/common/pkg/types"
	"github.com/telekom/controlplane/common/pkg/util/contextutil"
	"github.com/telekom/controlplane/common/pkg/util/labelutil"
	notificationv1 "github.com/telekom/controlplane/notification/api/v1"
	"github.com/telekom/controlplane/notification/api/v1/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func SendNotification(ctx context.Context, owner client.Object, sendToChannelNamespace, state string, target *types.TypedObjectRef, requester *approvalv1.Requester) (*types.ObjectRef, error) {
	properties := map[string]any{
		"environment": contextutil.EnvFromContextOrDie(ctx),
		"state":       state,
	}

	requesterMap, err := extractRequester(requester)
	if err != nil {
		return nil, err
	}
	for k, v := range requesterMap {
		properties[strings.ToLower(k)] = v
	}

	targetMap, targetKind, targetName := extractTarget(target)
	for k, v := range targetMap {
		properties[strings.ToLower(k)] = v
	}

	notificationBuilder := builder.New().
		WithOwner(owner).
		WithSender(notificationv1.SenderTypeSystem, "ApprovalService").
		WithDefaultChannels(ctx, sendToChannelNamespace).
		WithPurpose(strings.ToLower(owner.GetObjectKind().GroupVersionKind().Kind + "--" + targetKind)). // e.g. approval--apisubscription, approvalrequest--eventsubscription
		WithName(labelutil.NormalizeValue(targetKind + "--" + targetName)).                              //e.g. api-subscription--application--basepath-foo-bar-v1
		WithProperties(properties)

	notification, err := notificationBuilder.Send(ctx)
	if err != nil {
		return nil, err
	}
	return types.ObjectRefFromObject(notification), nil
}

func extractTarget(target *types.TypedObjectRef) (map[string]any, string, string) {
	var targetKind, targetApplication, targetGroup, targetTeam, targetName string
	properties := map[string]any{}
	if target != nil {
		targetKind, targetApplication, _, targetGroup, targetTeam = builder.ExtractApplicationInformation(*target)
		properties["target-kind"] = targetKind
		properties["target-application"] = targetApplication
		properties["target-group"] = targetGroup
		properties["target-team"] = targetTeam
		targetName = target.Name
	}
	return properties, targetKind, targetName
}

func extractRequester(requester *approvalv1.Requester) (map[string]any, error) {

	requesterPropertiesMap := map[string]any{}

	if requester.Properties.Size() != 0 {
		err := json.Unmarshal(requester.Properties.Raw, &requesterPropertiesMap)
		if err != nil {
			return nil, err
		}
	}

	requesterName := strings.Split(requester.Name, "--")
	if len(requesterName) > 1 {
		requesterPropertiesMap["requester-group"] = requesterName[0]
		requesterPropertiesMap["requester-team"] = requesterName[1]
	} else {
		requesterPropertiesMap["requester-group"] = requester.Name
		requesterPropertiesMap["requester-team"] = requester.Name
	}

	return requesterPropertiesMap, nil
}
