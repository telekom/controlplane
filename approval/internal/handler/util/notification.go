// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package util

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
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

	TemplatePlaceholderStateOld = "state_old"
	TemplatePlaceholderStateNew = "state_new"
	TemplatePlaceholderScopes   = "scopes"

	TemplatePlaceholderExpirationDate = "expiration_date"
	TemplatePlaceholderDaysRemaining  = "days_remaining"
	TemplatePlaceholderIsExpired      = "is_expired"
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
	// ApprovalKey isolates notification names for scoped approvals.
	// Empty preserves the legacy unscoped notification naming.
	ApprovalKey string
}

// ReminderNotificationData extends NotificationData with expiration-specific fields.
type ReminderNotificationData struct {
	NotificationData
	ExpirationDate string
	DaysRemaining  string
	IsExpired      bool
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

	// resource_type and resource_name are set directly by the subscription
	// handler that creates the ApprovalRequest (api/event/mcp), so they flow
	// through as ordinary properties.

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

	// Build the notification base name.
	// Scoped (non-empty ApprovalKey): deterministic hash of source identity + key + purpose.
	// Unscoped: legacy format <purpose>--<targetName>.
	var name string
	if data.ApprovalKey != "" {
		var err error
		name, err = scopedNotificationBaseName(data.Owner, data.ApprovalKey, strings.ToLower(purpose))
		if err != nil {
			return nil, fmt.Errorf("computing scoped notification name: %w", err)
		}
	} else {
		nameStringBuilder := strings.Builder{}
		nameStringBuilder.WriteString(purpose)
		nameStringBuilder.WriteString(DELIMITER)
		nameStringBuilder.WriteString(data.Target.GetName())
		name = nameStringBuilder.String()
	}

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
	// Note: Expiration fields (ExpirationDate, DaysRemaining, IsExpired) are only
	// initialized in SendReminderNotification, not for regular notifications

	return properties
}

// SendReminderNotification sends an expiration reminder notification.
func SendReminderNotification(ctx context.Context, data *ReminderNotificationData) (*types.ObjectRef, error) {
	properties := initializeProperties()

	properties[TemplatePlaceholderEnvironment] = contextutil.EnvFromContextOrDie(ctx)
	properties[TemplatePlaceholderStateNew] = data.StateNew
	properties[TemplatePlaceholderStateOld] = data.StateOld
	properties[TemplatePlaceholderExpirationDate] = data.ExpirationDate
	properties[TemplatePlaceholderDaysRemaining] = data.DaysRemaining
	properties[TemplatePlaceholderIsExpired] = data.IsExpired

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

	// purpose: <ownerKind-lowercased>--<action>--reminder--<actor>
	// example: approvalexpiration--subscribe--reminder--decider
	purposeStringBuilder := strings.Builder{}
	purposeStringBuilder.WriteString(strings.ToLower(data.Owner.GetObjectKind().GroupVersionKind().Kind))
	purposeStringBuilder.WriteString(DELIMITER)
	purposeStringBuilder.WriteString(data.Action)
	purposeStringBuilder.WriteString(DELIMITER)
	purposeStringBuilder.WriteString("reminder")
	purposeStringBuilder.WriteString(DELIMITER)
	purposeStringBuilder.WriteString(string(data.Actor))
	purpose := purposeStringBuilder.String()

	// Build the notification base name.
	// Scoped (non-empty ApprovalKey): deterministic hash of source identity + key + purpose.
	// Unscoped: legacy format <purpose>--<targetName>.
	var name string
	if data.ApprovalKey != "" {
		var err error
		name, err = scopedNotificationBaseName(data.Owner, data.ApprovalKey, strings.ToLower(purpose))
		if err != nil {
			return nil, fmt.Errorf("computing scoped reminder notification name: %w", err)
		}
	} else {
		nameStringBuilder := strings.Builder{}
		nameStringBuilder.WriteString(purpose)
		nameStringBuilder.WriteString(DELIMITER)
		nameStringBuilder.WriteString(data.Target.GetName())
		name = nameStringBuilder.String()
	}

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

const (
	scopedNotifPrefix  = "an-v1-"
	scopedNotifHashLen = 48 // 48 hex chars = 192 bits of SHA-256
)

// scopedNotifHashInput is the deterministic JSON-serialized struct whose
// encoding is fed into SHA-256. Field order is fixed by the struct declaration.
type scopedNotifHashInput struct {
	Domain        string `json:"domain"`
	NamingVersion string `json:"namingVersion"`
	SourceGroup   string `json:"sourceGroup"`
	SourceKind    string `json:"sourceKind"`
	SourceNs      string `json:"sourceNs"`
	SourceName    string `json:"sourceName"`
	SourceUID     string `json:"sourceUID"`
	ApprovalKey   string `json:"approvalKey"`
	Purpose       string `json:"purpose"`
}

// scopedNotificationBaseName produces a deterministic, bounded base name for a
// scoped notification. The result has prefix "an-v1-" followed by 48 lowercase
// hex characters (total length 54). Source identity = the notification controller
// owner (the ApprovalRequest for AR notifications, the Approval for Approval
// notifications). Same inputs always produce the same output.
func scopedNotificationBaseName(owner client.Object, approvalKey, purpose string) (string, error) {
	if owner.GetUID() == "" {
		return "", fmt.Errorf("scoped notification name: owner UID must not be empty")
	}

	gvk := owner.GetObjectKind().GroupVersionKind()

	input := scopedNotifHashInput{
		Domain:        "approval-notification",
		NamingVersion: "v1",
		SourceGroup:   gvk.Group,
		SourceKind:    gvk.Kind,
		SourceNs:      owner.GetNamespace(),
		SourceName:    owner.GetName(),
		SourceUID:     string(owner.GetUID()),
		ApprovalKey:   approvalKey,
		Purpose:       purpose,
	}

	b, err := json.Marshal(input)
	if err != nil {
		return "", fmt.Errorf("marshaling notification name input: %w", err)
	}
	sum := sha256.Sum256(b)
	digest := hex.EncodeToString(sum[:])
	return scopedNotifPrefix + digest[:scopedNotifHashLen], nil
}
