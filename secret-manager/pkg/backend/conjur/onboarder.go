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
	"github.com/telekom/controlplane/secret-manager/pkg/tracing"
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

func (c *ConjurOnboarder) OnboardEnvironment(ctx context.Context, env string, opts ...backend.OnboardOption) (backend.OnboardResponse, error) {
	log := logr.FromContextOrDiscard(ctx)
	log.Info("Onboarding environment", "env", env)

	options := backend.OnboardOptions{}
	for _, opt := range opts {
		opt(&options)
	}

	policyPath := RootPolicyPath
	c.traceOnboardStart(ctx, "environment", policyPath, env, backend.NoTeam, backend.NoApp)
	buf := bytes.NewBuffer(nil)
	_, err := c.templates["env"].Execute(buf, map[string]any{
		"Environment": env,
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed to execute template")
	}

	var secretRefs map[string]backend.SecretRef
	mutator := func(ctx context.Context) error {
		log.V(1).Info("Loading policy", "policyPath", policyPath, "env", env)
		_, err = c.conjur.LoadPolicy(conjurapi.PolicyModePost, policyPath, buf)
		if err != nil {
			return err
		}

		allowedSecrets := backend.NewEnvironmentSecrets()
		if err = backend.TryAddSecrets(New, allowedSecrets, env, backend.NoTeam, backend.NoApp, options.SecretValues); err != nil {
			return err
		}
		secrets, err := allowedSecrets.GetSecrets()
		if err != nil {
			return errors.Wrap(err, "failed to get allowed secrets")
		}

		secretRefs, err = c.createSecrets(ctx, env, backend.NoTeam, backend.NoApp, secrets, backend.WithWriteStrategy(options.Strategy))
		if err != nil {
			return errors.Wrapf(err, "failed to create secrets for env %s", env)
		}
		return nil
	}

	err = c.MaybeRunWithBouncer(ctx, policyPath, mutator)
	if err != nil {
		return nil, err
	}
	backend.MergeSecretRefs(New, secretRefs, env, backend.NoTeam, backend.NoApp, options.SecretValues)
	c.traceOnboardEnd(ctx, "environment", policyPath, env, backend.NoTeam, backend.NoApp, secretRefs)

	return backend.NewDefaultOnboardResponse(secretRefs), nil
}

func (c *ConjurOnboarder) OnboardTeam(ctx context.Context, env, teamId string, opts ...backend.OnboardOption) (backend.OnboardResponse, error) {
	log := logr.FromContextOrDiscard(ctx)
	log.Info("Onboarding team", "env", env, "team", teamId)

	options := backend.OnboardOptions{}
	for _, opt := range opts {
		opt(&options)
	}
	policyPath := RootPolicyPath + "/" + env
	c.traceOnboardStart(ctx, "team", policyPath, env, teamId, backend.NoApp)

	buf := bytes.NewBuffer(nil)
	_, err := c.templates["team"].Execute(buf, map[string]any{
		"TeamId": teamId,
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed to execute template")
	}

	var secretRefs map[string]backend.SecretRef
	mutator := func(ctx context.Context) error {
		log.V(1).Info("Loading policy", "policyPath", policyPath, "env", env, "teamId", teamId)
		_, err = c.conjur.LoadPolicy(conjurapi.PolicyModePost, policyPath, buf)
		if err != nil {
			return err
		}

		allowedSecrets := backend.NewTeamSecrets()
		if err = backend.TryAddSecrets(New, allowedSecrets, env, teamId, backend.NoApp, options.SecretValues); err != nil {
			return err
		}
		secrets, err := allowedSecrets.GetSecrets()
		if err != nil {
			return errors.Wrap(err, "failed to get allowed secrets")
		}

		secretRefs, err = c.createSecrets(ctx, env, teamId, backend.NoApp, secrets, backend.WithWriteStrategy(options.Strategy))
		if err != nil {
			return errors.Wrapf(err, "failed to create secrets for team %s", teamId)
		}
		return nil
	}

	err = c.MaybeRunWithBouncer(ctx, policyPath, mutator)
	if err != nil {
		return nil, err
	}
	backend.MergeSecretRefs(New, secretRefs, env, teamId, backend.NoApp, options.SecretValues)
	c.traceOnboardEnd(ctx, "team", policyPath, env, teamId, backend.NoApp, secretRefs)

	return backend.NewDefaultOnboardResponse(secretRefs), nil
}

func (c *ConjurOnboarder) OnboardApplication(ctx context.Context, env, teamId, appId string, opts ...backend.OnboardOption) (backend.OnboardResponse, error) {
	log := logr.FromContextOrDiscard(ctx)
	options := backend.OnboardOptions{}
	for _, opt := range opts {
		opt(&options)
	}

	log.Info("Onboarding application", "env", env, "team", teamId, "app", appId)
	policyPath := RootPolicyPath + "/" + env + "/" + teamId
	c.traceOnboardStart(ctx, "application", policyPath, env, teamId, appId)

	buf := bytes.NewBuffer(nil)
	_, err := c.templates["app"].Execute(buf, map[string]any{
		"AppId": appId,
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed to execute template")
	}

	var secretRefs map[string]backend.SecretRef
	mutator := func(ctx context.Context) error {
		log.V(1).Info("Loading policy", "policyPath", policyPath, "env", env, "teamId", teamId, "appId", appId)
		_, err = c.conjur.LoadPolicy(conjurapi.PolicyModePost, policyPath, buf)
		if err != nil {
			return err
		}

		allowedSecrets := backend.NewApplicationSecrets()
		if err = backend.TryAddSecrets(New, allowedSecrets, env, teamId, appId, options.SecretValues); err != nil {
			return err
		}
		secrets, err := allowedSecrets.GetSecrets()
		if err != nil {
			return errors.Wrap(err, "failed to get allowed secrets")
		}

		secretRefs, err = c.createSecrets(ctx, env, teamId, appId, secrets, backend.WithWriteStrategy(options.Strategy))
		if err != nil {
			return errors.Wrapf(err, "failed to create secrets for application %s", appId)
		}
		return nil
	}

	err = c.MaybeRunWithBouncer(ctx, policyPath, mutator)
	if err != nil {
		return nil, err
	}
	backend.MergeSecretRefs(New, secretRefs, env, teamId, appId, options.SecretValues)
	c.traceOnboardEnd(ctx, "application", policyPath, env, teamId, appId, secretRefs)

	return backend.NewDefaultOnboardResponse(secretRefs), nil
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
		if cErr, ok := AsError(err); ok && cErr.Code == 404 {
			// Policy not found, nothing to delete
			return nil
		}
		return errors.Wrap(err, "failed to load delete policy")
	}

	return nil
}

func (c *ConjurOnboarder) createSecrets(ctx context.Context, env, teamId, appId string, secrets map[string]backend.SecretValue, opts ...backend.WriteOption) (map[string]backend.SecretRef, error) {
	log := logr.FromContextOrDiscard(ctx)
	secretRefMap := make(map[string]backend.SecretRef)
	if c.secretWriter == nil {
		return secretRefMap, nil
	}
	for secretPath, secretValue := range secrets {
		secretId := New(env, teamId, appId, secretPath, backend.MakeChecksum(secretValue.Value()))
		log.Info("Creating secret", "secretId", secretId.String())
		if tracing.Enabled() {
			log.V(1).Info("trace createSecrets input",
				"traceId", tracing.TraceID(ctx),
				"scope", "application",
				"env", env,
				"team", teamId,
				"app", appId,
				"secretPath", secretPath,
				"requestedSecretId", secretId.String(),
				"checksum", backend.MakeChecksum(secretValue.Value()),
			)
		}
		secret, err := c.secretWriter.Set(ctx, secretId, secretValue, opts...)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to initialize secret %s", secretId.VariableId())
		}
		if tracing.Enabled() {
			log.V(1).Info("trace createSecrets result",
				"traceId", tracing.TraceID(ctx),
				"secretPath", secretPath,
				"requestedSecretId", secretId.String(),
				"returnedSecretId", secret.Id().String(),
			)
		}
		secretRefMap[secretPath] = secret.Id()
	}

	return secretRefMap, nil
}

func (c *ConjurOnboarder) traceOnboardStart(ctx context.Context, scope, policyPath, env, team, app string) {
	if !tracing.Enabled() {
		return
	}
	log := logr.FromContextOrDiscard(ctx)
	log.V(1).Info("trace onboard start",
		"traceId", tracing.TraceID(ctx),
		"scope", scope,
		"policyPath", policyPath,
		"env", env,
		"team", team,
		"app", app,
	)
}

func (c *ConjurOnboarder) traceOnboardEnd(ctx context.Context, scope, policyPath, env, team, app string, refs map[string]backend.SecretRef) {
	if !tracing.Enabled() {
		return
	}
	log := logr.FromContextOrDiscard(ctx)
	log.V(1).Info("trace onboard end",
		"traceId", tracing.TraceID(ctx),
		"scope", scope,
		"policyPath", policyPath,
		"env", env,
		"team", team,
		"app", app,
		"secretRefCount", len(refs),
	)
	for name, ref := range refs {
		log.V(1).Info("trace onboard secretRef",
			"traceId", tracing.TraceID(ctx),
			"scope", scope,
			"secretName", name,
			"secretRef", ref.String(),
		)
	}
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
