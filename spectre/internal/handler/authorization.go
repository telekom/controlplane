// SPDX-FileCopyrightText: 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package handler

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	applicationv1 "github.com/telekom/controlplane/application/api/v1"
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

// authorizationIntent captures every field that changes the meaning of an
// approval grant. Two Listeners with different intents must not share the
// same approval — a grant for one does not cover the other.
type authorizationIntent struct {
	ConsumerName      string
	ConsumerNamespace string
	ConsumerUID       string
	ProviderName      string
	ProviderNamespace string
	ProviderUID       string
	ApplicationName   string
	ApplicationNs     string
	ApiBasePath       string
	CaptureRequest    bool
	CaptureResponse   bool
	DeliveryType      string
	CallbackTarget    string
	RequestFilter     bool
	ResponseFilter    bool
}

// buildAuthorizationIntent constructs the canonical intent from resolved
// objects. The intent includes every field that should invalidate an
// existing approval when it changes.
func buildAuthorizationIntent(
	listener *spectrev1.Listener,
	consumerApp *applicationv1.Application,
	providerApp *applicationv1.Application,
	spectreApp *spectrev1.SpectreApplication,
) authorizationIntent {
	intent := authorizationIntent{
		ConsumerName:      consumerApp.Name,
		ConsumerNamespace: consumerApp.Namespace,
		ConsumerUID:       string(consumerApp.UID),
		ProviderName:      providerApp.Name,
		ProviderNamespace: providerApp.Namespace,
		ProviderUID:       string(providerApp.UID),
		ApplicationName:   spectreApp.Name,
		ApplicationNs:     spectreApp.Namespace,
		DeliveryType:      spectreApp.Spec.DeliveryType,
		CallbackTarget:    spectreApp.Spec.Callback,
	}

	if listener.Spec.ApiListener != nil {
		intent.ApiBasePath = listener.Spec.ApiListener.ApiBasePath
		intent.CaptureRequest = true
		intent.CaptureResponse = true
		intent.RequestFilter = listener.Spec.ApiListener.RequestFilter != nil
		intent.ResponseFilter = listener.Spec.ApiListener.ResponseFilter != nil
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
		{"consumer.name", a.ConsumerName},
		{"consumer.namespace", a.ConsumerNamespace},
		{"consumer.uid", a.ConsumerUID},
		{"provider.name", a.ProviderName},
		{"provider.namespace", a.ProviderNamespace},
		{"provider.uid", a.ProviderUID},
		{"application.name", a.ApplicationName},
		{"application.namespace", a.ApplicationNs},
		{"apiBasePath", a.ApiBasePath},
		{"captureRequest", fmt.Sprintf("%t", a.CaptureRequest)},
		{"captureResponse", fmt.Sprintf("%t", a.CaptureResponse)},
		{"deliveryType", a.DeliveryType},
		{"callbackTarget", a.CallbackTarget},
		{"requestFilter", fmt.Sprintf("%t", a.RequestFilter)},
		{"responseFilter", fmt.Sprintf("%t", a.ResponseFilter)},
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

// approvalProperties returns a human-readable map suitable for
// Requester.SetProperties, exposing the capture intent to approvers.
func (a *authorizationIntent) approvalProperties() map[string]any {
	props := map[string]any{
		"action":              "listen-provider",
		"consumer":            a.ConsumerNamespace + "/" + a.ConsumerName,
		"provider":            a.ProviderNamespace + "/" + a.ProviderName,
		"listenerApplication": a.ApplicationNs + "/" + a.ApplicationName,
		"apiBasePath":         a.ApiBasePath,
		"captureRequest":      a.CaptureRequest,
		"captureResponse":     a.CaptureResponse,
		"deliveryType":        a.DeliveryType,
		"requestFilter":       a.RequestFilter,
		"responseFilter":      a.ResponseFilter,
	}
	if a.CallbackTarget != "" {
		props["callbackTarget"] = a.CallbackTarget
	}
	return props
}

// isStaleChild returns true if the resource's fingerprint label is missing
// or differs from the current fingerprint.
func isStaleChild(labels map[string]string, currentFingerprint string) bool {
	fp, ok := labels[AuthorizationFingerprintLabelKey]
	return !ok || fp != currentFingerprint
}
