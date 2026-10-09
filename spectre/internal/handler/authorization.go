// SPDX-FileCopyrightText: 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package handler

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	applicationv1 "github.com/telekom/controlplane/application/api/v1"
	approvalv1 "github.com/telekom/controlplane/approval/api/v1"
	spectrev1 "github.com/telekom/controlplane/spectre/api/v1"
)

// AuthorizationFingerprintLabelKey labels Listener-owned children with the
// authorization intent that was active when they were created. A provider,
// application, path, delivery, or capture-scope change produces a new
// fingerprint, causing stale children to be removed before the replacement
// approval is evaluated.
const AuthorizationFingerprintLabelKey = "spectre.cp.ei.telekom.de/authorization-fingerprint"

// maxLabelValueLen is the Kubernetes limit for label values.
const maxLabelValueLen = 63

// PlacementIntent identifies validated placement references that affect where
// data is captured or delivered. Only stable routing identifiers are included —
// never entire resource specs/statuses, resourceVersions, Ready conditions,
// emails, token/secret values, or reconciliation timestamps.
type PlacementIntent struct {
	ApiExposureName             string
	ApiExposureNamespace        string
	CaptureRouteName            string
	CaptureRouteNamespace       string
	CaptureZoneName             string
	CaptureZoneNamespace        string
	CaptureEventStoreName       string
	CaptureEventStoreNamespace  string
	CallbackOriginZoneName      string
	CallbackOriginZoneNamespace string
	DeliveryZoneName            string
	DeliveryZoneNamespace       string
	DeliveryEventStoreName      string
	DeliveryEventStoreNamespace string
	CallbackBaseURL             string
}

// authorizationIntent captures every field that changes the meaning of an
// approval grant. Two Listeners with different intents must not share the
// same approval — a grant for one does not cover the other.
type authorizationIntent struct {
	PolicyVersion      string
	ConsumerGroup      string
	ConsumerKind       string
	ConsumerName       string
	ConsumerNamespace  string
	ConsumerUID        string
	ConsumerTeam       string
	ConsumerClientId   string
	ProviderGroup      string
	ProviderKind       string
	ProviderName       string
	ProviderNamespace  string
	ProviderUID        string
	ProviderTeam       string
	ProviderClientId   string
	SpectreAppGroup    string
	SpectreAppKind     string
	SpectreAppName     string
	SpectreAppNs       string
	SpectreAppUID      string
	ObserverGroup      string
	ObserverKind       string
	ObserverName       string
	ObserverNamespace  string
	ObserverUID        string
	ObserverTeam       string
	ObserverClientId   string
	ApiBasePath        string
	CaptureRequest     bool
	CaptureResponse    bool
	DeliveryType       string
	CallbackTarget     string
	RequestFilterJSON  string
	ResponseFilterJSON string
	Placement          PlacementIntent
}

// buildAuthorizationIntent constructs the canonical intent from resolved
// objects. The intent includes every field that should invalidate an
// existing approval when it changes.
func buildAuthorizationIntent(
	listener *spectrev1.Listener,
	consumerApp *applicationv1.Application,
	providerApp *applicationv1.Application,
	spectreApp *spectrev1.SpectreApplication,
	observerApp *applicationv1.Application,
	placement PlacementIntent,
) authorizationIntent {
	intent := authorizationIntent{
		PolicyVersion:     "v2",
		ConsumerGroup:     applicationv1.GroupVersion.Group,
		ConsumerKind:      "Application",
		ConsumerName:      consumerApp.Name,
		ConsumerNamespace: consumerApp.Namespace,
		ConsumerUID:       string(consumerApp.UID),
		ConsumerTeam:      consumerApp.Spec.Team,
		ConsumerClientId:  consumerApp.Status.ClientId,
		ProviderGroup:     applicationv1.GroupVersion.Group,
		ProviderKind:      "Application",
		ProviderName:      providerApp.Name,
		ProviderNamespace: providerApp.Namespace,
		ProviderUID:       string(providerApp.UID),
		ProviderTeam:      providerApp.Spec.Team,
		ProviderClientId:  providerApp.Status.ClientId,
		SpectreAppGroup:   spectrev1.GroupVersion.Group,
		SpectreAppKind:    "SpectreApplication",
		SpectreAppName:    spectreApp.Name,
		SpectreAppNs:      spectreApp.Namespace,
		SpectreAppUID:     string(spectreApp.UID),
		ObserverGroup:     applicationv1.GroupVersion.Group,
		ObserverKind:      "Application",
		ObserverName:      observerApp.Name,
		ObserverNamespace: observerApp.Namespace,
		ObserverUID:       string(observerApp.UID),
		ObserverTeam:      observerApp.Spec.Team,
		ObserverClientId:  observerApp.Status.ClientId,
		DeliveryType:      spectreApp.Spec.DeliveryType,
		CallbackTarget:    spectreApp.Spec.Callback,
		Placement:         placement,
	}

	if listener.Spec.ApiListener != nil {
		intent.ApiBasePath = listener.Spec.ApiListener.ApiBasePath
		intent.CaptureRequest = true
		intent.CaptureResponse = true
		if listener.Spec.ApiListener.RequestFilter != nil {
			if data, err := json.Marshal(listener.Spec.ApiListener.RequestFilter); err == nil {
				intent.RequestFilterJSON = string(data)
			}
		}
		if listener.Spec.ApiListener.ResponseFilter != nil {
			if data, err := json.Marshal(listener.Spec.ApiListener.ResponseFilter); err == nil {
				intent.ResponseFilterJSON = string(data)
			}
		}
	}

	return intent
}

// fingerprint returns a deterministic, K8s-safe label value (≤63 chars,
// lowercase hex) that represents this authorization intent.
func (a *authorizationIntent) fingerprint() string {
	h := sha256.New()

	// Write fields in a fixed order. Each field is separated by a newline
	// and prefixed with its name so that values cannot collide across fields
	// (e.g. name="a" ns="b" vs name="ab" ns="").
	fields := []struct {
		key string
		val string
	}{
		{"policyVersion", a.PolicyVersion},
		{"consumer.group", a.ConsumerGroup},
		{"consumer.kind", a.ConsumerKind},
		{"consumer.name", a.ConsumerName},
		{"consumer.namespace", a.ConsumerNamespace},
		{"consumer.uid", a.ConsumerUID},
		{"consumer.team", a.ConsumerTeam},
		{"consumer.clientId", a.ConsumerClientId},
		{"provider.group", a.ProviderGroup},
		{"provider.kind", a.ProviderKind},
		{"provider.name", a.ProviderName},
		{"provider.namespace", a.ProviderNamespace},
		{"provider.uid", a.ProviderUID},
		{"provider.team", a.ProviderTeam},
		{"provider.clientId", a.ProviderClientId},
		{"spectreApp.group", a.SpectreAppGroup},
		{"spectreApp.kind", a.SpectreAppKind},
		{"spectreApp.name", a.SpectreAppName},
		{"spectreApp.namespace", a.SpectreAppNs},
		{"spectreApp.uid", a.SpectreAppUID},
		{"observer.group", a.ObserverGroup},
		{"observer.kind", a.ObserverKind},
		{"observer.name", a.ObserverName},
		{"observer.namespace", a.ObserverNamespace},
		{"observer.uid", a.ObserverUID},
		{"observer.team", a.ObserverTeam},
		{"observer.clientId", a.ObserverClientId},
		{"apiBasePath", a.ApiBasePath},
		{"captureRequest", fmt.Sprintf("%t", a.CaptureRequest)},
		{"captureResponse", fmt.Sprintf("%t", a.CaptureResponse)},
		{"deliveryType", a.DeliveryType},
		{"callbackTarget", a.CallbackTarget},
		{"requestFilter", filterFingerprintValue(a.RequestFilterJSON)},
		{"responseFilter", filterFingerprintValue(a.ResponseFilterJSON)},
		// Placement fields — any change to routing topology invalidates consent.
		{"placement.apiExposure.name", a.Placement.ApiExposureName},
		{"placement.apiExposure.namespace", a.Placement.ApiExposureNamespace},
		{"placement.captureRoute.name", a.Placement.CaptureRouteName},
		{"placement.captureRoute.namespace", a.Placement.CaptureRouteNamespace},
		{"placement.captureZone.name", a.Placement.CaptureZoneName},
		{"placement.captureZone.namespace", a.Placement.CaptureZoneNamespace},
		{"placement.captureEventStore.name", a.Placement.CaptureEventStoreName},
		{"placement.captureEventStore.namespace", a.Placement.CaptureEventStoreNamespace},
		{"placement.callbackOriginZone.name", a.Placement.CallbackOriginZoneName},
		{"placement.callbackOriginZone.namespace", a.Placement.CallbackOriginZoneNamespace},
		{"placement.deliveryZone.name", a.Placement.DeliveryZoneName},
		{"placement.deliveryZone.namespace", a.Placement.DeliveryZoneNamespace},
		{"placement.deliveryEventStore.name", a.Placement.DeliveryEventStoreName},
		{"placement.deliveryEventStore.namespace", a.Placement.DeliveryEventStoreNamespace},
		{"placement.callbackBaseURL", a.Placement.CallbackBaseURL},
	}

	for _, f := range fields {
		fmt.Fprintf(h, "%s=%s\n", f.key, f.val)
	}

	full := hex.EncodeToString(h.Sum(nil))
	// Truncate to fit K8s label value constraint (≤63 chars).
	if len(full) > maxLabelValueLen {
		return full[:maxLabelValueLen]
	}
	return full
}

// gateRequestHash produces a per-gate hash that binds the common authorization
// intent to a specific approval gate identified by key, requesterTeam, and
// deciderTeam. It uses ScopedIntentHash for canonical JSON encoding so map key
// ordering does not affect the result.
//
// The returned value is a full-length hex SHA-256 digest (not truncated to 63
// chars) — it is passed to WithHashValue on the approval builder, not used as
// a label value.
func (a *authorizationIntent) gateRequestHash(key, requesterTeam, deciderTeam string) (string, error) {
	if key == "" {
		return "", fmt.Errorf("gateRequestHash: key must not be empty")
	}

	payload := struct {
		Intent    authorizationIntent `json:"intent"`
		Key       string              `json:"key"`
		Requester string              `json:"requester"`
		Decider   string              `json:"decider"`
	}{
		Intent:    *a,
		Key:       key,
		Requester: requesterTeam,
		Decider:   deciderTeam,
	}

	return approvalv1.ScopedIntentHash(payload)
}

// gateApprovalProperties returns per-gate approval properties. Each gate gets
// its own action string so approvers can see which gate the request is for.
func (a *authorizationIntent) gateApprovalProperties(action string) map[string]any {
	props := map[string]any{
		"action":              action,
		"consumer":            a.ConsumerNamespace + "/" + a.ConsumerName,
		"provider":            a.ProviderNamespace + "/" + a.ProviderName,
		"observer":            a.ObserverNamespace + "/" + a.ObserverName,
		"listenerApplication": a.SpectreAppNs + "/" + a.SpectreAppName,
		"apiBasePath":         a.ApiBasePath,
		"captureRequest":      a.CaptureRequest,
		"captureResponse":     a.CaptureResponse,
		"deliveryType":        a.DeliveryType,
		"requestFilter":       filterPropertyValue(a.RequestFilterJSON),
		"responseFilter":      filterPropertyValue(a.ResponseFilterJSON),
		"policyVersion":       a.PolicyVersion,
		"consumerTeam":        a.ConsumerTeam,
		"providerTeam":        a.ProviderTeam,
		"observerTeam":        a.ObserverTeam,
	}
	if a.CallbackTarget != "" {
		props["callbackTarget"] = a.CallbackTarget
	}
	// Include placement if any field is populated.
	if a.Placement.ApiExposureName != "" {
		props["apiExposure"] = a.Placement.ApiExposureNamespace + "/" + a.Placement.ApiExposureName
	}
	if a.Placement.CaptureRouteName != "" {
		props["captureRoute"] = a.Placement.CaptureRouteNamespace + "/" + a.Placement.CaptureRouteName
	}
	if a.Placement.CaptureZoneName != "" {
		props["captureZone"] = a.Placement.CaptureZoneNamespace + "/" + a.Placement.CaptureZoneName
	}
	if a.Placement.DeliveryZoneName != "" {
		props["deliveryZone"] = a.Placement.DeliveryZoneNamespace + "/" + a.Placement.DeliveryZoneName
	}
	if a.Placement.CallbackBaseURL != "" {
		props["callbackBaseURL"] = a.Placement.CallbackBaseURL
	}
	return props
}

// filterFingerprintValue returns "false" for an empty string (backward-compat
// with the old boolean fingerprint) or the JSON content otherwise.
func filterFingerprintValue(jsonStr string) string {
	if jsonStr == "" {
		return "false"
	}
	return jsonStr
}

// filterPropertyValue returns false (bool) when no filter is set, or the JSON
// string when one is present — for human-readable approval properties.
func filterPropertyValue(jsonStr string) any {
	if jsonStr == "" {
		return false
	}
	return jsonStr
}

// isStaleChild returns true if the resource's fingerprint label is missing
// or differs from the current fingerprint.
func isStaleChild(labels map[string]string, currentFingerprint string) bool {
	fp, ok := labels[AuthorizationFingerprintLabelKey]
	return !ok || fp != currentFingerprint
}
