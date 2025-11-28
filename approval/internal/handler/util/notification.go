// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package util

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/pkg/errors"
	"strings"

	approvalv1 "github.com/telekom/controlplane/approval/api/v1"
	"github.com/telekom/controlplane/common/pkg/types"
	"github.com/telekom/controlplane/common/pkg/util/contextutil"
	"github.com/telekom/controlplane/common/pkg/util/labelutil"
	notificationv1 "github.com/telekom/controlplane/notification/api/v1"
	"github.com/telekom/controlplane/notification/api/v1/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type NotificationScenario string

const (
	NotificationScenarioCreated NotificationScenario = "created"
	NotificationScenarioUpdated NotificationScenario = "updated"
)

const (
	TemplatePlaceholderDeciderTeam        = "decider_team"
	TemplatePlaceholderDeciderGroup       = "decider_group"
	TemplatePlaceholderDeciderApplication = "decider_application"

	TemplatePlaceholderRequesterTeam        = "requester_team"
	TemplatePlaceholderRequesterGroup       = "requester_group"
	TemplatePlaceholderRequesterApplication = "requester_application"

	TemplatePlaceholderEnvironment = "environment"
	TemplatePlaceholderBasepath    = "basepath"
	TemplatePlaceholderStateOld    = "state_old"
	TemplatePlaceholderStateNew    = "state_new"
	TemplatePlaceholderScopes      = "scopes"
)

type Actor string

const (
	ActorDecider   Actor = "decider"
	ActorRequester Actor = "requester"
)

type NotificationData struct {
	Owner                  client.Object
	SendToChannelNamespace string
	StateNew               string
	StateOld               string
	Target                 *types.TypedObjectRef
	Requester              *approvalv1.Requester
	Decider                *approvalv1.Decider
	Scenario               NotificationScenario
	Actor                  Actor
}

func extractDecider(decider *approvalv1.Decider) (map[string]any, error) {
	deciderPropertiesMap := map[string]any{}

	groupName, teamName, err := splitTeamName(decider.TeamName)
	if err != nil {
		return nil, errors.New(fmt.Sprintf("Failed to parse decider teamName %+v", decider))
	}

	deciderPropertiesMap[TemplatePlaceholderDeciderTeam] = teamName
	deciderPropertiesMap[TemplatePlaceholderDeciderGroup] = groupName
	deciderPropertiesMap[TemplatePlaceholderDeciderApplication] = decider.ApplicationRef.Name

	return deciderPropertiesMap, nil
}

func extractRequester(requester *approvalv1.Requester) (map[string]any, error) {

	requesterPropertiesMap := map[string]any{}

	if requester.Properties.Size() != 0 {
		err := json.Unmarshal(requester.Properties.Raw, &requesterPropertiesMap)
		if err != nil {
			return nil, errors.Wrapf(err, "Failed to extract requester properties from %q", requester.Properties.Raw)
		}
	}

	// basepath
	// the property is already present from the original requester properties

	// scopes
	if requesterPropertiesMap[TemplatePlaceholderScopes] == nil {
		requesterPropertiesMap[TemplatePlaceholderScopes] = "undefined"
	}

	// team and group
	groupName, teamName, err := splitTeamName(requester.TeamName)
	if err != nil {
		return nil, errors.New(fmt.Sprintf("Failed to parse requester teamName %+v", requester))
	}
	requesterPropertiesMap[TemplatePlaceholderRequesterGroup] = groupName
	requesterPropertiesMap[TemplatePlaceholderRequesterTeam] = teamName

	// application
	requesterPropertiesMap[TemplatePlaceholderRequesterApplication] = requester.ApplicationRef.Name

	return requesterPropertiesMap, nil
}

func SendNotification(ctx context.Context, data *NotificationData) (*types.ObjectRef, error) {
	properties := initializeProperties()

	properties[TemplatePlaceholderEnvironment] = contextutil.EnvFromContextOrDie(ctx)
	properties[TemplatePlaceholderStateNew] = data.StateNew
	properties[TemplatePlaceholderStateOld] = data.StateOld

	requesterMap, err := extractRequester(data.Requester)
	if err != nil {
		return nil, errors.Wrapf(err, "Failed to extract requester data")
	}
	for k, v := range requesterMap {
		properties[strings.ToLower(k)] = v
	}

	deciderMap, err := extractDecider(data.Decider)
	if err != nil {
		return nil, errors.Wrapf(err, "Failed to extract decider data")
	}
	for k, v := range deciderMap {
		properties[strings.ToLower(k)] = v
	}

	// let's build the purpose <ownerKind>--<targetKind>--<scenario>--<actor>
	// example: approvalrequest--apisubscription--created--decider
	purposeStringBuilder := strings.Builder{}
	// owner kind
	purposeStringBuilder.WriteString(data.Owner.GetObjectKind().GroupVersionKind().Kind)
	purposeStringBuilder.WriteString(DELIMITER)

	// target kind
	purposeStringBuilder.WriteString(data.Target.GetKind())
	purposeStringBuilder.WriteString(DELIMITER)

	// scenario
	purposeStringBuilder.WriteString(string(data.Scenario))
	purposeStringBuilder.WriteString(DELIMITER)

	// actor (decider / requester)
	purposeStringBuilder.WriteString(string(data.Actor))
	purpose := purposeStringBuilder.String()

	// let's build the notifications name - <purpose>--<targetName>
	// example: ...
	nameStringBuilder := strings.Builder{}
	nameStringBuilder.WriteString(purpose)
	nameStringBuilder.WriteString(DELIMITER)
	nameStringBuilder.WriteString(data.Target.GetName())
	name := nameStringBuilder.String()

	notificationBuilder := builder.New().
		WithOwner(data.Owner).
		WithSender(notificationv1.SenderTypeSystem, "ApprovalService").
		WithDefaultChannels(ctx, data.SendToChannelNamespace).
		WithPurpose(strings.ToLower(purpose)).
		WithName(labelutil.NormalizeNameValue(name)).
		WithProperties(properties)

	notification, err := notificationBuilder.Send(ctx)
	if err != nil {
		return nil, err
	}
	return types.ObjectRefFromObject(notification), nil
}

// initializeProperties - useful for detecting unresolved TemplatePlaceholder values
func initializeProperties() map[string]any {
	properties := map[string]any{}

	defaultValue := "UNDEFINED"

	// decider TemplatePlaceholders
	properties[TemplatePlaceholderDeciderTeam] = defaultValue
	properties[TemplatePlaceholderDeciderGroup] = defaultValue
	properties[TemplatePlaceholderDeciderApplication] = defaultValue

	// requester TemplatePlaceholders
	properties[TemplatePlaceholderRequesterTeam] = defaultValue
	properties[TemplatePlaceholderRequesterGroup] = defaultValue
	properties[TemplatePlaceholderRequesterApplication] = defaultValue

	// other
	properties[TemplatePlaceholderEnvironment] = defaultValue
	properties[TemplatePlaceholderBasepath] = defaultValue
	properties[TemplatePlaceholderStateOld] = defaultValue
	properties[TemplatePlaceholderStateNew] = defaultValue
	properties[TemplatePlaceholderScopes] = defaultValue

	return properties
}
