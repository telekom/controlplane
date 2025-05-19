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
	Env() string
	String() string
}

// Secret contains the value of the secret and its ID.
type Secret[T SecretId] interface {
	Id() T
	Value() string
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
}

// Reader is used to read a secret from the backend.
type Reader[T SecretId, S Secret[T]] interface {
	Get(context.Context, T) (S, error)
}

// Writer is used to write or delete a secret in the backend.
type Writer[T SecretId, S Secret[T]] interface {
	Set(context.Context, T, SecretValue) (S, error)
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

// Onboarder is the interface that must be implemented by all onboarders.
// Each steps of this process depends on the previous one.
// It is used to onboard a new environment, team or application.
// It is also used to delete an environment, team or application.
type Onboarder interface {
	OnboardEnvironment(ctx context.Context, env string) (OnboardResponse, error)
	OnboardTeam(ctx context.Context, env, id string) (OnboardResponse, error)
	OnboardApplication(ctx context.Context, env, teamId, appId string) (OnboardResponse, error)

	DeleteEnvironment(ctx context.Context, env string) error
	DeleteTeam(ctx context.Context, env, id string) error
	DeleteApplication(ctx context.Context, env, teamId, appId string) error
}

var (
	// Secrets that are per-default created for each environment
	EnvironmentSecrets = []string{"zones"}
	// Secrets that are per-default created for each team
	TeamSecrets = []string{"clientSecret", "teamToken"}
	// Secrets that are per-default created for each application
	ApplicationSecrets = []string{"clientSecret", "externalSecrets"}
)
