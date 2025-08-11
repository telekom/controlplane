// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package backend

import "encoding/json"

var _ SecretValue = secretValue{}

type secretValue struct {
	value       string
	allowChange bool
}

func (s secretValue) Value() string {
	return s.value
}

// IsEmpty checks if the secret value is empty.
// An empty value is defined as an empty string or an empty JSON object "{}".
func (s secretValue) IsEmpty() bool {
	return s.value == "" || s.value == "{}"
}

func (s secretValue) EqualString(value string) bool {
	return s.value == value
}

func (s secretValue) AllowChange() bool {
	return s.allowChange
}

// Empty returns a SecretValue that is empty and not allowed to be changed
func Empty() SecretValue {
	return secretValue{}
}

// String returns a SecretValue that is allowed to be changed
func String(s string) SecretValue {
	return secretValue{s, true}
}

// InitialString returns a SecretValue that is not allowed to be changed
// after it is set. This is useful for onboarding.
func InitialString(s string) SecretValue {
	return secretValue{s, false}
}

// JSON returns a SecretValue that is marshaled to JSON.
// It is allowed to be changed after it is set.
func JSON(value any) (SecretValue, error) {
	b, err := json.Marshal(value)
	if err != nil {
		return Empty(), err
	}
	return secretValue{string(b), true}, nil
}
