// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package backend

const Separator = ":"

var _ Secret[SecretId] = DefaultSecret[SecretId]{}

type DefaultSecret[T SecretId] struct {
	id    T
	value string
}

func NewDefaultSecret[T SecretId](id T, value string) DefaultSecret[T] {
	return DefaultSecret[T]{id: id, value: value}
}

func (d DefaultSecret[T]) Value() string {
	return d.value
}

func (d DefaultSecret[T]) Id() T {
	return d.id
}

var _ OnboardResponse = DefaultOnboardResponse{}

type DefaultOnboardResponse struct {
	secretRefs map[string]SecretRef
}

func NewDefaultOnboardResponse(secretRefs map[string]SecretRef) OnboardResponse {
	return DefaultOnboardResponse{secretRefs: secretRefs}
}

func (d DefaultOnboardResponse) SecretRefs() map[string]SecretRef {
	return d.secretRefs
}

type StringSecretRef struct {
	secretId string
}

func NewStringSecretRef(secretId string) StringSecretRef {
	return StringSecretRef{secretId: secretId}
}

func (s StringSecretRef) String() string {
	return s.secretId
}
