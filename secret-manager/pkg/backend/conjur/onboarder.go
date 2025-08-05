// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package conjur

import (
	"bytes"
	"context"

	"github.com/cyberark/conjur-api-go/conjurapi"
	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	"github.com/telekom/controlplane/secret-manager/pkg/backend"
	"github.com/telekom/controlplane/secret-manager/pkg/backend/conjur/bouncer"
	"github.com/valyala/fasttemplate"
)

var _ backend.Onboarder = &ConjurOnboarder{}

type ConjurOnboarder struct {
	conjur       ConjurAPI
	secretWriter backend.Writer[ConjurSecretId, backend.DefaultSecret[ConjurSecretId]]
	templates    map[string]*fasttemplate.Template

	bouncer bouncer.Bouncer
}

func NewOnboarder(writeAPI ConjurAPI, secretWriter backend.Writer[ConjurSecretId, backend.DefaultSecret[ConjurSecretId]]) *ConjurOnboarder {
	return &ConjurOnboarder{
		conjur: writeAPI,
		templates: map[string]*fasttemplate.Template{
			"env":    fasttemplate.New(EnvironmentPolicyTemplate, startTag, endTag),
			"team":   fasttemplate.New(TeamPolicyTemplate, startTag, endTag),
			"app":    fasttemplate.New(ApplicationPolicyTemplate, startTag, endTag),
			"delete": fasttemplate.New(DeletePolicyTemplate, startTag, endTag),
		},
		secretWriter: secretWriter,
	}
}

func (c *ConjurOnboarder) WithBouncer(bouncer bouncer.Bouncer) *ConjurOnboarder {
	if bouncer == nil {
		return c
	}
	c.bouncer = bouncer
	return c
}

func (c *ConjurOnboarder) OnboardEnvironment(ctx context.Context, env string) (backend.OnboardResponse, error) {
	log := logr.FromContextOrDiscard(ctx)
	log.Info("Onboarding environment", "env", env)

	policyPath := RootPolicyPath
	buf := bytes.NewBuffer(nil)
	_, err := c.templates["env"].Execute(buf, map[string]any{
		"Environment": env,
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed to execute template")
	}

	mutator := func(ctx context.Context) error {
		log.V(1).Info("Loading policy", "policyPath", policyPath, "env", env)
		_, err = c.conjur.LoadPolicy(conjurapi.PolicyModePost, policyPath, buf)
		return err
	}

	err = c.MaybeRunWithBouncer(ctx, policyPath, mutator)
	if err != nil {
		return nil, err
	}

	secrets, err := backend.NewEnvironmentSecrets().GetSecrets()
	if err != nil {
		return nil, errors.Wrap(err, "failed to get allowed secrets")
	}
	secretsIds, err := c.createSecrets(ctx, env, "", "", secrets)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create secrets for environment %s", env)
	}

	return backend.NewDefaultOnboardResponse(secretsIds), nil
}

func (c *ConjurOnboarder) OnboardTeam(ctx context.Context, env, teamId string) (backend.OnboardResponse, error) {
	log := logr.FromContextOrDiscard(ctx)
	log.Info("Onboarding team", "env", env, "team", teamId)
	policyPath := RootPolicyPath + "/" + env

	buf := bytes.NewBuffer(nil)
	_, err := c.templates["team"].Execute(buf, map[string]any{
		"TeamId": teamId,
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed to execute template")
	}

	mutator := func(ctx context.Context) error {
		log.V(1).Info("Loading policy", "policyPath", policyPath, "env", env, "teamId", teamId)
		_, err = c.conjur.LoadPolicy(conjurapi.PolicyModePost, policyPath, buf)
		return err
	}

	err = c.MaybeRunWithBouncer(ctx, policyPath, mutator)
	if err != nil {
		return nil, err
	}

	secrets, err := backend.NewTeamSecrets().GetSecrets()
	if err != nil {
		return nil, errors.Wrap(err, "failed to get allowed secrets")
	}
	secretsIds, err := c.createSecrets(ctx, env, teamId, "", secrets)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create secrets for team %s", teamId)
	}

	return backend.NewDefaultOnboardResponse(secretsIds), nil
}

func (c *ConjurOnboarder) OnboardApplication(ctx context.Context, env, teamId, appId string, opts ...backend.OnboardOption) (backend.OnboardResponse, error) {
	log := logr.FromContextOrDiscard(ctx)
	options := backend.OnboardOptions{}
	for _, opt := range opts {
		opt(&options)
	}

	log.Info("Onboarding application", "env", env, "team", teamId, "app", appId)
	policyPath := RootPolicyPath + "/" + env + "/" + teamId

	buf := bytes.NewBuffer(nil)
	_, err := c.templates["app"].Execute(buf, map[string]any{
		"AppId": appId,
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed to execute template")
	}

	mutator := func(ctx context.Context) error {
		log.V(1).Info("Loading policy", "policyPath", policyPath, "env", env, "teamId", teamId, "appId", appId)
		_, err = c.conjur.LoadPolicy(conjurapi.PolicyModePost, policyPath, buf)
		return err
	}

	err = c.MaybeRunWithBouncer(ctx, policyPath, mutator)
	if err != nil {
		return nil, err
	}

	allowedSecrets := backend.NewApplicationSecrets()
	for key, value := range options.SecretValues {
		secretId := New(env, teamId, appId, key, "")
		ok := allowedSecrets.TrySetSecret(key, value)
		if !ok {
			return nil, backend.Forbidden(secretId, errors.Errorf("secret %s is not allowed for application onboarding", key))
		}
	}
	secrets, err := allowedSecrets.GetSecrets()
	if err != nil {
		return nil, errors.Wrap(err, "failed to get allowed secrets")
	}
	secretsIds, err := c.createSecrets(ctx, env, teamId, appId, secrets)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create secrets for application %s", appId)
	}
	for secretPath, secretValue := range options.SecretValues {
		if _, ok := secretsIds[secretPath]; !ok {
			secretsIds[secretPath] = New(env, teamId, appId, secretPath, backend.MakeChecksum(secretValue.Value()))
		} else {
			log.V(0).Info("Value for secret already exists", "secretPath", secretPath)
		}
	}

	return backend.NewDefaultOnboardResponse(secretsIds), nil
}

func (c *ConjurOnboarder) DeleteEnvironment(ctx context.Context, env string) error {
	log := logr.FromContextOrDiscard(ctx)
	policyPath := RootPolicyPath
	log.Info("Deleting environment", "env", env, "policyPath", policyPath)

	mutator := func(ctx context.Context) error {
		return c.deletePolicy(ctx, policyPath, env)
	}
	err := c.MaybeRunWithBouncer(ctx, policyPath, mutator)
	if err != nil {
		return errors.Wrapf(err, "failed to delete environment %s", env)
	}
	return nil
}

func (c *ConjurOnboarder) DeleteTeam(ctx context.Context, env, teamId string) error {
	log := logr.FromContextOrDiscard(ctx)
	policyPath := RootPolicyPath + "/" + env
	log.Info("Deleting team", "env", env, "team", teamId, "policyPath", policyPath)

	mutator := func(ctx context.Context) error {
		return c.deletePolicy(ctx, policyPath, teamId)
	}
	err := c.MaybeRunWithBouncer(ctx, policyPath, mutator)
	if err != nil {
		return errors.Wrapf(err, "failed to delete team %s in environment %s", teamId, env)
	}
	return nil
}
func (c *ConjurOnboarder) DeleteApplication(ctx context.Context, env, teamId, appId string) error {
	log := logr.FromContextOrDiscard(ctx)
	policyPath := RootPolicyPath + "/" + env + "/" + teamId
	log.Info("Deleting application", "env", env, "team", teamId, "app", appId, "policyPath", policyPath)

	mutator := func(ctx context.Context) error {
		return c.deletePolicy(ctx, policyPath, appId)
	}
	err := c.MaybeRunWithBouncer(ctx, policyPath, mutator)
	if err != nil {
		return errors.Wrapf(err, "failed to delete application %s in team %s in environment %s", appId, teamId, env)
	}
	return nil
}

func (c *ConjurOnboarder) deletePolicy(ctx context.Context, policyPath, policyKey string) error {
	log := logr.FromContextOrDiscard(ctx)
	buf := bytes.NewBuffer(nil)
	_, err := c.templates["delete"].Execute(buf, map[string]any{
		"PolicyPath": policyKey,
	})
	if err != nil {
		return errors.Wrap(err, "failed to execute delete template")
	}
	log.Info("Deleting policy", "policyPath", policyPath, "policyKey", policyKey)
	_, err = c.conjur.LoadPolicy(conjurapi.PolicyModePatch, policyPath, buf)
	if err != nil {
		return errors.Wrap(err, "failed to load delete policy")
	}

	return nil
}

func (c *ConjurOnboarder) createSecrets(ctx context.Context, env, teamId, appId string, secrets map[string]backend.SecretValue) (map[string]backend.SecretRef, error) {
	log := logr.FromContextOrDiscard(ctx)
	secretRefMap := make(map[string]backend.SecretRef)
	if c.secretWriter == nil {
		return secretRefMap, nil
	}
	for secretPath, secretValue := range secrets {
		secretId := New(env, teamId, appId, secretPath, backend.MakeChecksum(secretValue.Value()))
		log.Info("Creating secret", "secretId", secretId.String())
		secret, err := c.secretWriter.Set(ctx, secretId, secretValue)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to initialize secret %s", secretId.VariableId())
		}
		secretRefMap[secretPath] = secret.Id()
	}

	return secretRefMap, nil
}

func (c *ConjurOnboarder) MaybeRunWithBouncer(ctx context.Context, policyPath string, run bouncer.Runnable) error {
	if c.bouncer == nil {
		return run(ctx)
	}
	err := c.bouncer.RunB(ctx, policyPath, run)
	if err != nil && errors.Is(err, bouncer.ErrLockNotAcquired) {
		return backend.NewBackendError(nil, err, backend.TypeErrTooManyRequests)
	}
	return err
}
