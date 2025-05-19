// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package middleware

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/go-logr/logr"
	jwtware "github.com/gofiber/contrib/jwt"
	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"
	"github.com/telekom/controlplane/common-server/pkg/problems"
)

type AccessType string

const (
	AccessTypeNone               AccessType = "none"
	AccessTypeSecretsRead        AccessType = "secrets_read"
	AccessTypeSecretsWrite       AccessType = "secrets_write"
	AccessTypeAppOnboardingWrite AccessType = "onboarding_write"
)

type AccessTypeSet map[AccessType]struct{}

func (ats AccessTypeSet) Has(at AccessType) bool {
	_, ok := ats[at]
	return ok
}

type ServiceAccessConfig struct {
	ServiceAccountName string       `yaml:"service_account_name" json:"service_account_name"`
	DeploymentName     string       `yaml:"deployment_name" json:"deployment_name"`
	Namespace          string       `yaml:"namespace" json:"namespace"`
	AllowedAccess      []AccessType `yaml:"allowed_access" json:"allowed_access"`
	AllowedSecrets     []string     `yaml:"allowed_secrets" json:"allowed_secrets"`
	allowedAccessSet   AccessTypeSet
}

type KubernetesAuthzOptions struct {
	JWKSetURLs     []string
	TrustedIssuers []string
	Audience       string
	AccessConfig   []ServiceAccessConfig
}

func (o *KubernetesAuthzOptions) ServiceAccessConfig() map[string]ServiceAccessConfig {
	cfg := make(map[string]ServiceAccessConfig)
	for _, config := range o.AccessConfig {
		key := config.ServiceAccountName + config.Namespace
		if config.AllowedAccess != nil {
			config.allowedAccessSet = make(map[AccessType]struct{})
			for _, access := range config.AllowedAccess {
				config.allowedAccessSet[access] = struct{}{}
			}
		}
		cfg[key] = config
	}
	return cfg
}

func WithTrustedIssuers(issuers ...string) KubernetesAuthOption {
	return func(o *KubernetesAuthzOptions) {
		o.TrustedIssuers = append(o.TrustedIssuers, issuers...)
	}
}

func WithJWKSetURLs(urls ...string) KubernetesAuthOption {
	return func(o *KubernetesAuthzOptions) {
		o.JWKSetURLs = append(o.JWKSetURLs, urls...)
	}
}

func WithInClusterIssuer() KubernetesAuthOption {
	return func(o *KubernetesAuthzOptions) {
		c, err := getClusterInfo()
		if err != nil {
			panic(err)
		}
		o.TrustedIssuers = append(o.TrustedIssuers, c.Issuer)
		// Workaround as the returned jwks URL is not reachable
		jwksUrl := c.Issuer + "/keys"
		o.JWKSetURLs = append(o.JWKSetURLs, jwksUrl)
	}
}

func WithAccessConfig(configs ...ServiceAccessConfig) KubernetesAuthOption {
	return func(o *KubernetesAuthzOptions) {
		o.AccessConfig = append(o.AccessConfig, configs...)
	}
}

type KubernetesAuthOption func(*KubernetesAuthzOptions)

func NewKubernetesAuthz(opts ...KubernetesAuthOption) fiber.Handler {
	options := defaultOpts()
	for _, opt := range opts {
		opt(options)
	}

	if len(options.TrustedIssuers) == 0 {
		fmt.Println("⚠️\tDisabling Kubernetes Authz middleware, no trusted issuers provided")
		return func(c *fiber.Ctx) error {
			return c.Next()
		}
	}

	return jwtware.New(jwtware.Config{
		ContextKey:     "user",
		JWKSetURLs:     options.JWKSetURLs,
		SuccessHandler: newSuccessHandler(options),
		Claims:         &ServiceAccountTokenClaims{},
		ErrorHandler: func(c *fiber.Ctx, err error) error {
			var pErr problems.Problem
			if errors.As(err, &pErr) {
				return pErr
			}
			return problems.Unauthorized("Failed to authenticate", err.Error())
		},
	})
}

func defaultOpts() *KubernetesAuthzOptions {
	return &KubernetesAuthzOptions{
		JWKSetURLs:     []string{},
		TrustedIssuers: []string{},
		Audience:       "secret-manager",
		AccessConfig:   []ServiceAccessConfig{},
	}
}

func isReadOnly(c *fiber.Ctx) bool {
	if c.Method() == fiber.MethodGet || c.Method() == fiber.MethodHead || c.Method() == fiber.MethodOptions {
		return true
	}
	return false
}

func isOnboardingRequest(c *fiber.Ctx) bool {
	return strings.HasPrefix(c.Path(), "/api/v1/onboarding/")
}

func isAccessAllowed(c *fiber.Ctx, accessTypesSet AccessTypeSet) bool {
	if accessTypesSet == nil {
		logr.FromContextOrDiscard(c.UserContext()).Error(nil, "No access types defined")
		return false
	}

	if isOnboardingRequest(c) {
		return accessTypesSet.Has(AccessTypeAppOnboardingWrite)
	}

	if isReadOnly(c) {
		return accessTypesSet.Has(AccessTypeSecretsRead)
	} else {
		return accessTypesSet.Has(AccessTypeSecretsWrite)
	}
}

func newSuccessHandler(options *KubernetesAuthzOptions) fiber.Handler {
	cfg := options.ServiceAccessConfig()
	return func(c *fiber.Ctx) error {
		user := c.Locals("user").(*jwt.Token)
		claims := user.Claims.(*ServiceAccountTokenClaims)

		if claims == nil {
			return problems.Unauthorized("Failed to authenticate", "Invalid token structure")
		}

		log := logr.FromContextOrDiscard(c.UserContext())
		log = log.WithValues("san", claims.Kubernetes.ServiceAccount.Name, "ns", claims.Kubernetes.Namespace)

		if slices.Contains(claims.Audience, options.Audience) {
			return problems.Forbidden("Access denied", "Invalid audience")
		}

		serviceAccountName := claims.Kubernetes.ServiceAccount.Name
		namespace := claims.Kubernetes.Namespace
		podName := claims.Kubernetes.Pod.Name

		key := serviceAccountName + namespace
		if len(cfg) > 0 {
			config, ok := cfg[key]
			if !ok {
				log.Info("Forbidden", "service_account_name", serviceAccountName, "namespace", namespace)
				return problems.Forbidden("Access denied", "Invalid source")
			}

			if !strings.HasPrefix(podName, config.DeploymentName) { // TODO: improve this with an pod-informer!?
				log.Info("Forbidden", "pod_name", podName, "deployment_name", config.DeploymentName)
				return problems.Forbidden("Access denied", "Invalid source")
			}

			if !isAccessAllowed(c, config.allowedAccessSet) {
				log.Info("Forbidden", "allowed_access", config.AllowedAccess)
				return problems.Forbidden("Access denied", "Invalid access")
			}
		} else {
			log.Info("No access config defined. Assuming all access is allowed")
		}

		log.Info("Authorized", "service_account_name", serviceAccountName, "namespace", namespace)

		c.SetUserContext(logr.NewContext(c.UserContext(), log))
		return c.Next()
	}
}
