// SPDX-FileCopyrightText: 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"

	"github.com/telekom/controlplane/common/pkg/types"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

const (
	scopedApprovalPrefix = "ag-v1-"
	scopedRequestPrefix  = "ar-v1-"
	scopedHashLen        = 48 // 48 hex chars = 192 bits of SHA-256
	scopedNameLen        = 6 + scopedHashLen // prefix + digest = 54
)

// dnsLabelRe validates that a key is a DNS label: lowercase alphanumeric,
// may contain hyphens, must not start or end with hyphen, max 32 chars.
var dnsLabelRe = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,30}[a-z0-9])?$`)

// scopedHashInput is the deterministic JSON-serialized struct whose encoding
// is fed into SHA-256. Field order is fixed by the struct declaration and
// encoding/json serializes in declared order.
type scopedHashInput struct {
	Domain         string `json:"domain"`
	NamingVersion  string `json:"namingVersion"`
	TargetGroup    string `json:"targetGroup"`
	TargetKind     string `json:"targetKind"`
	TargetNs       string `json:"targetNs"`
	TargetName     string `json:"targetName"`
	TargetUID      string `json:"targetUID"`
	Key            string `json:"key"`
	IntentHash     string `json:"intentHash,omitempty"`
}

// ScopedApprovalName produces a deterministic, bounded DNS-label name for a
// scoped Approval. The result has prefix "ag-v1-" followed by 48 lowercase hex
// characters (total length 54). Identical canonical inputs always produce the
// same output; any field change produces a different output.
func ScopedApprovalName(target types.TypedObjectRef, key string) (string, error) {
	group, err := validateTarget(target, key)
	if err != nil {
		return "", err
	}

	input := scopedHashInput{
		Domain:        "approval-gate",
		NamingVersion: "v1",
		TargetGroup:   group,
		TargetKind:    target.Kind,
		TargetNs:      target.Namespace,
		TargetName:    target.Name,
		TargetUID:     string(target.UID),
		Key:           key,
	}

	digest, err := hashJSON(input)
	if err != nil {
		return "", fmt.Errorf("hashing approval name input: %w", err)
	}
	return scopedApprovalPrefix + digest[:scopedHashLen], nil
}

// ScopedApprovalRequestName produces a deterministic, bounded DNS-label name
// for a scoped ApprovalRequest. The result has prefix "ar-v1-" followed by 48
// lowercase hex characters (total length 54). A changed intentHash changes the
// request name while the same target+key still maps to the same Approval name.
func ScopedApprovalRequestName(target types.TypedObjectRef, key, intentHash string) (string, error) {
	group, err := validateTarget(target, key)
	if err != nil {
		return "", err
	}
	if intentHash == "" {
		return "", fmt.Errorf("scoped request name: intentHash must not be empty")
	}

	input := scopedHashInput{
		Domain:        "approval-request",
		NamingVersion: "v1",
		TargetGroup:   group,
		TargetKind:    target.Kind,
		TargetNs:      target.Namespace,
		TargetName:    target.Name,
		TargetUID:     string(target.UID),
		Key:           key,
		IntentHash:    intentHash,
	}

	digest, err := hashJSON(input)
	if err != nil {
		return "", fmt.Errorf("hashing approval request name input: %w", err)
	}
	return scopedRequestPrefix + digest[:scopedHashLen], nil
}

// validateTarget checks the target and key for required fields and format.
// It returns the parsed API group (empty string for core group is valid).
func validateTarget(target types.TypedObjectRef, key string) (string, error) {
	if target.UID == "" {
		return "", fmt.Errorf("scoped name: target UID must not be empty")
	}
	if target.Namespace == "" {
		return "", fmt.Errorf("scoped name: target namespace must not be empty")
	}
	if target.Kind == "" {
		return "", fmt.Errorf("scoped name: target kind must not be empty")
	}
	if target.APIVersion == "" {
		return "", fmt.Errorf("scoped name: target APIVersion must not be empty")
	}
	if key == "" {
		return "", fmt.Errorf("scoped name: key must not be empty")
	}
	if len(key) > 32 || !dnsLabelRe.MatchString(key) {
		return "", fmt.Errorf("scoped name: key %q must be a DNS label of at most 32 characters", key)
	}

	gv, err := schema.ParseGroupVersion(target.APIVersion)
	if err != nil {
		return "", fmt.Errorf("scoped name: parsing APIVersion %q: %w", target.APIVersion, err)
	}
	return gv.Group, nil
}

// hashJSON marshals v to JSON and returns the full lowercase hex SHA-256 digest.
func hashJSON(v any) (string, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}
