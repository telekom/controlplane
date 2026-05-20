// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package util

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/pkg/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	approvalv1 "github.com/telekom/controlplane/approval/api/v1"
	"github.com/telekom/controlplane/common/pkg/types"
	"github.com/telekom/controlplane/common/pkg/util/contextutil"
	"github.com/telekom/controlplane/common/pkg/util/labelutil"
	notificationv1 "github.com/telekom/controlplane/notification/api/v1"
	"github.com/telekom/controlplane/notification/api/v1/builder"
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

	// TemplatePlaceholderResourceName represents the resource, if the resourceType is api, then resourceName is the basepath
	TemplatePlaceholderResourceName = "resource_name"

	// TemplatePlaceholderResourceType can be api/event
	TemplatePlaceholderResourceType = "resource_type"

	TemplatePlaceholderStateOld       = "state_old"
	TemplatePlaceholderStateNew       = "state_new"
	TemplatePlaceholderScopes         = "scopes"
	TemplatePlaceholderExpirationDate = "expiration_date"
	TemplatePlaceholderDaysRemaining  = "days_remaining"
	TemplatePlaceholderReminderType   = "reminder_type"
)

const (
	NotificationPropertiesBasePath  = "basePath"
	NotificationPropertiesEventType = "eventType"
)

const (
	NotificationResourceTypeApi   = "API"
	NotificationResourceTypeEvent = "event"
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
	Action                 string
	// Expiration-specific fields
	ExpirationDate string
	DaysRemaining  string
	ReminderType   string
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

	// handle the resource type and resource name
	// api - basepath (set in api subscription handler)
	if requesterPropertiesMap[NotificationPropertiesBasePath] != nil {
		requesterPropertiesMap[TemplatePlaceholderResourceType] = NotificationResourceTypeApi
		requesterPropertiesMap[TemplatePlaceholderResourceName] = requesterPropertiesMap[NotificationPropertiesBasePath]
	}

	// event - event type (set in event subscription handler)
	if requesterPropertiesMap[NotificationPropertiesEventType] != nil {
		requesterPropertiesMap[TemplatePlaceholderResourceType] = NotificationResourceTypeEvent
		requesterPropertiesMap[TemplatePlaceholderResourceName] = requesterPropertiesMap[NotificationPropertiesEventType]
	}

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

	// let's build the purpose <ownerKind>--<approvalAction>--<scenario>--<actor>
	// example: approvalrequest--subscribe--created--decider
	purposeStringBuilder := strings.Builder{}
	// owner kind
	purposeStringBuilder.WriteString(data.Owner.GetObjectKind().GroupVersionKind().Kind)
	purposeStringBuilder.WriteString(DELIMITER)

	// target kind
	// uses the approval/approvalRequest action - for example "subscribe"
	purposeStringBuilder.WriteString(data.Action)
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
	properties[TemplatePlaceholderResourceName] = defaultValue
	properties[TemplatePlaceholderResourceType] = defaultValue
	properties[TemplatePlaceholderStateOld] = defaultValue
	properties[TemplatePlaceholderStateNew] = defaultValue
	properties[TemplatePlaceholderScopes] = defaultValue
	// Note: Expiration fields (ExpirationDate, DaysRemaining, ReminderType) are only
	// initialized in SendReminderNotification, not for regular notifications

	return properties
}

// SendReminderNotification sends an expiration reminder notification
func SendReminderNotification(ctx context.Context, data *NotificationData) (*types.ObjectRef, error) {
	properties := initializeProperties()

	properties[TemplatePlaceholderEnvironment] = contextutil.EnvFromContextOrDie(ctx)
	properties[TemplatePlaceholderStateNew] = data.StateNew
	properties[TemplatePlaceholderStateOld] = data.StateOld
	properties[TemplatePlaceholderExpirationDate] = data.ExpirationDate
	properties[TemplatePlaceholderDaysRemaining] = data.DaysRemaining
	properties[TemplatePlaceholderReminderType] = data.ReminderType

	requesterMap, err := extractRequester(data.Requester)
	if err != nil {
		return nil, errors.Wrap(err, "failed to extract requester data")
	}
	for k, v := range requesterMap {
		properties[strings.ToLower(k)] = v
	}

	deciderMap, err := extractDecider(data.Decider)
	if err != nil {
		return nil, errors.Wrap(err, "failed to extract decider data")
	}
	for k, v := range deciderMap {
		properties[strings.ToLower(k)] = v
	}

	// Build purpose: <ownerKind>--<approvalAction>--reminder--<actor>
	// Example: approvalexpiration--subscribe--reminder--decider
	purposeStringBuilder := strings.Builder{}
	purposeStringBuilder.WriteString(strings.ToLower(data.Owner.GetObjectKind().GroupVersionKind().Kind))
	purposeStringBuilder.WriteString(DELIMITER)
	purposeStringBuilder.WriteString(data.Action)
	purposeStringBuilder.WriteString(DELIMITER)
	purposeStringBuilder.WriteString("reminder")
	purposeStringBuilder.WriteString(DELIMITER)
	purposeStringBuilder.WriteString(string(data.Actor))
	purpose := purposeStringBuilder.String()

	// Build notification name: <purpose>--<targetName>--<hash>
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
