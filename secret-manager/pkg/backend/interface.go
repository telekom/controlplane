// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package backend

import (
	"context"
)

// IdParser is responsible for parsing a secret ID from a string
// and returning the corresponding SecretId type of the backend implementation.
type IdParser[T SecretId] interface {
	ParseSecretId(string) (T, error)
}

// SecretId represents the minimal interface for a secret ID.
// It must be extended based on the needs to the backend implementation.
type SecretId interface {
	Cacheable

	// Env returns the environment of the secret.
	Env() string
	// String returns the string representation of the secret ID. It must be unique for each secret.
	String() string
	// Path returns the path of the secret, if it has one. If it doesn't have a path, it returns an empty string.
	Path() string
	// SubPath returns the subpath of the secret, if it has one. If it doesn't have a subpath, it returns an empty string.
	SubPath() string
	// Copy returns a copy of the SecretId.
	Copy() SecretId
	// ParentId returns the parent ID of the secret, if it has a subpath. If it doesn't have a subpath, it returns itself.
	ParentId() SecretId
}

// Secret contains the value of the secret and its ID.
type Secret[T SecretId] interface {
	Id() T
	Value() string
	Copy() Secret[T]
}

// SecretValue is used to set the value of a secret.
type SecretValue interface {
	// Desired value
	Value() string
	// compare the value with the current value
	EqualString(string) bool
	// if this value can only be used to initialize a secret
	AllowChange() bool
	// if this value is empty
	IsEmpty() bool
	// Copy the value to a new instance
	Copy() SecretValue
}

// Reader is used to read a secret from the backend.
type Reader[T SecretId, S Secret[T]] interface {
	Get(context.Context, T) (S, error)
}

// WriteStrategy defines how a secret value should be written.
type WriteStrategy string

// WriteOptions holds options that modify how a secret is written.
type WriteOptions struct {
	Strategy WriteStrategy
}

// WriteOption is a functional option for Writer.Set.
type WriteOption func(*WriteOptions)

// WithWriteStrategy sets the strategy for how a secret value should be written.
// With merge, JSON object values are shallow-merged with the existing value.
// With replace, the value is overwritten directly (default).
func WithWriteStrategy(strategy WriteStrategy) WriteOption {
	return func(o *WriteOptions) {
		o.Strategy = strategy
	}
}

// Writer is used to write or delete a secret in the backend.
type Writer[T SecretId, S Secret[T]] interface {
	Set(context.Context, T, SecretValue, ...WriteOption) (S, error)
	Delete(context.Context, T) error
}

// Backend is the interface that must be implemented by all backends.
type Backend[T SecretId, S Secret[T]] interface {
	IdParser[T]
	Reader[T, S]
	Writer[T, S]
}

// SecretRef is a simpler version of SecretId
// and is also implemented by it.
type SecretRef interface {
	String() string
}

// OnboardResponse is used to return the result of the onboarding process.
// It contains the secret references that were created during the onboarding process.
type OnboardResponse interface {
	SecretRefs() map[string]SecretRef
}

const (
	// StrategyMerge merges provided secrets with existing ones.
	// Existing secrets not present in the request are preserved.
	StrategyMerge WriteStrategy = "merge"
	// StrategyReplace replaces all existing secrets with the provided ones.
	StrategyReplace WriteStrategy = "replace"
)

type OnboardOptions struct {
	SecretValues map[string]SecretValue
	Strategy     WriteStrategy
}

type OnboardOption func(*OnboardOptions)

func WithSecretValue(key string, value SecretValue) OnboardOption {
	return func(o *OnboardOptions) {
		if o.SecretValues == nil {
			o.SecretValues = make(map[string]SecretValue)
		}
		o.SecretValues[key] = value
	}
}

// WithStrategy sets the strategy for how secrets should be applied.
func WithStrategy(strategy WriteStrategy) OnboardOption {
	return func(o *OnboardOptions) {
		o.Strategy = strategy
	}
}

// Onboarder is the interface that must be implemented by all onboarders.
// Each steps of this process depends on the previous one.
// It is used to onboard a new environment, team or application.
// It is also used to delete an environment, team or application.
type Onboarder interface {
	OnboardEnvironment(ctx context.Context, env string, opts ...OnboardOption) (OnboardResponse, error)
	OnboardTeam(ctx context.Context, env, id string, opts ...OnboardOption) (OnboardResponse, error)
	OnboardApplication(ctx context.Context, env, teamId, appId string, opts ...OnboardOption) (OnboardResponse, error)

	DeleteEnvironment(ctx context.Context, env string) error
	DeleteTeam(ctx context.Context, env, id string) error
	DeleteApplication(ctx context.Context, env, teamId, appId string) error
}

// Cacheable is an interface that can be implemented by any type that can be cached.
// CacheKey returns a stable cache key that identifies the logical secret
// independent of mutable properties like checksums. Unlike String(), this key
// does not change when the secret value changes.
type Cacheable interface {
	CacheKey() string
}
