// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"

	"github.com/pkg/errors"

	"github.com/telekom/controlplane/secret-manager/pkg/backend"
)

type OnboardResponse struct {
	SecretRefs map[string]string
}

type ControllerOptions struct {
	SecretValues map[string]string
}

type OnboardOption func(*ControllerOptions)

func WithSecretValues(secrets map[string]string) OnboardOption {
	return func(opts *ControllerOptions) {
		opts.SecretValues = secrets
	}
}

type OnboardController interface {
	OnboardEnvironment(ctx context.Context, envId string) (OnboardResponse, error)
	OnboardTeam(ctx context.Context, envId, teamId string) (OnboardResponse, error)
	OnboardApplication(ctx context.Context, envId, teamId, appId string, opts ...OnboardOption) (OnboardResponse, error)

	DeleteEnvironment(ctx context.Context, envId string) error
	DeleteTeam(ctx context.Context, envId, teamId string) error
	DeleteApplication(ctx context.Context, envId, teamId, appId string) error
}

type onboardController struct {
	Onboarder backend.Onboarder
}

func NewOnboardController(o backend.Onboarder) OnboardController {
	return &onboardController{Onboarder: o}
}

func (c *onboardController) OnboardEnvironment(ctx context.Context, envId string) (res OnboardResponse, err error) {
	if envId == "" {
		return res, errors.New("envId cannot be empty")
	}

	o, err := c.Onboarder.OnboardEnvironment(ctx, envId)
	if err != nil {
		return res, errors.Wrap(err, "failed to onboard environment")
	}

	res.SecretRefs = make(map[string]string, len(o.SecretRefs()))
	for name, ref := range o.SecretRefs() {
		res.SecretRefs[name] = ref.String()
	}

	return res, nil
}

func (c *onboardController) OnboardTeam(ctx context.Context, envId, teamId string) (res OnboardResponse, err error) {
	if envId == "" {
		return res, errors.New("envId cannot be empty")
	}
	if teamId == "" {
		return res, errors.New("teamId cannot be empty")
	}

	o, err := c.Onboarder.OnboardTeam(ctx, envId, teamId)
	if err != nil {
		return res, errors.Wrap(err, "failed to onboard team")
	}

	res.SecretRefs = make(map[string]string, len(o.SecretRefs()))
	for name, ref := range o.SecretRefs() {
		res.SecretRefs[name] = ref.String()
	}
	return res, nil
}

func (c *onboardController) OnboardApplication(ctx context.Context, envId, teamId, appId string, opts ...OnboardOption) (res OnboardResponse, err error) {
	if envId == "" {
		return res, errors.New("envId cannot be empty")
	}
	if teamId == "" {
		return res, errors.New("teamId cannot be empty")
	}
	if appId == "" {
		return res, errors.New("appId cannot be empty")
	}
	options := ControllerOptions{}
	for _, opt := range opts {
		opt(&options)
	}

	onboardOpts := make([]backend.OnboardOption, 0, len(options.SecretValues))
	for key, value := range options.SecretValues {
		onboardOpts = append(onboardOpts, backend.WithSecretValue(key, backend.String(value)))
	}

	o, err := c.Onboarder.OnboardApplication(ctx, envId, teamId, appId, onboardOpts...)
	if err != nil {
		return res, errors.Wrap(err, "failed to onboard application")
	}

	res.SecretRefs = make(map[string]string, len(o.SecretRefs()))
	for name, ref := range o.SecretRefs() {
		res.SecretRefs[name] = ref.String()
	}

	return res, nil
}

func (c *onboardController) DeleteEnvironment(ctx context.Context, envId string) error {
	if envId == "" {
		return errors.New("envId cannot be empty")
	}

	err := c.Onboarder.DeleteEnvironment(ctx, envId)
	if err != nil {
		return errors.Wrap(err, "failed to delete environment")
	}

	return nil
}

func (c *onboardController) DeleteTeam(ctx context.Context, envId, teamId string) error {
	if envId == "" {
		return errors.New("envId cannot be empty")
	}
	if teamId == "" {
		return errors.New("teamId cannot be empty")
	}

	err := c.Onboarder.DeleteTeam(ctx, envId, teamId)
	if err != nil {
		return errors.Wrap(err, "failed to delete team")
	}

	return nil
}

func (c *onboardController) DeleteApplication(ctx context.Context, envId, teamId, appId string) error {
	if envId == "" {
		return errors.New("envId cannot be empty")
	}
	if teamId == "" {
		return errors.New("teamId cannot be empty")
	}
	if appId == "" {
		return errors.New("appId cannot be empty")
	}

	err := c.Onboarder.DeleteApplication(ctx, envId, teamId, appId)
	if err != nil {
		return errors.Wrap(err, "failed to delete application")
	}

	return nil
}
