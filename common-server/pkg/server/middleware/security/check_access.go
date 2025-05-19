// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package security

import (
	"context"
	"fmt"
	"strings"

	"github.com/go-logr/logr"
	"github.com/gofiber/fiber/v2"
	"github.com/telekom/controlplane/common-server/pkg/problems"
	"github.com/telekom/controlplane/common-server/pkg/server/middleware/util"
)

type MatchType string

const (
	MatchTypeEqual   MatchType = "equal"
	MatchTypePrefix  MatchType = "prefix"
	defaultMatchType           = MatchTypePrefix
)

var accessDenied = problems.Forbidden("Access Denied", "Access to requested resource not allowed")

// ResourcePathFunc must return the expected resource for the provided BusinessContext
type ResourcePathFunc func(*BusinessContext, map[string]string, map[ClientType]ComparisonTemplates) (string, error)

var defaultExpectedResourcePath ResourcePathFunc = func(bCtx *BusinessContext, params map[string]string, templates map[ClientType]ComparisonTemplates) (string, error) {
	m := NewMatcher(templates[bCtx.ClientType].ExpectedTemplate, templates[bCtx.ClientType].UserInputTemplate)
	return m.ToExpectedString(bCtx, params)
}

var toDatastorePrefix = func(rpf ResourcePathFunc, templates map[ClientType]ComparisonTemplates) ResourcePathFunc {
	return func(bCtx *BusinessContext, params map[string]string, templates map[ClientType]ComparisonTemplates) (string, error) {
		rp, err := rpf(bCtx, params, templates)
		if err != nil {
			return "", err
		}
		if bCtx.ClientType == ClientTypeTeam && !strings.HasSuffix(rp, "/") {
			rp += "/"
		}
		return rp, nil
	}
}

type CheckAccessOpts struct {
	PathParamKeys        []string
	ExpectedResourcePath ResourcePathFunc
	// Prefix is used to calculate the prefix that is used in the context
	// It is expected that this prefix is then used to determine the access to the store
	// The store uses a key-format `<namespace>/<name>` to store resources.
	// At most the prefix should match the namespace part of the key
	Prefix    ResourcePathFunc
	Templates map[ClientType]ComparisonTemplates
}

func WithExpectedResourcePathFunc(f ResourcePathFunc) Option[*CheckAccessOpts] {
	return func(o *CheckAccessOpts) {
		o.ExpectedResourcePath = f
	}
}

func WithPrefixFunc(f ResourcePathFunc) Option[*CheckAccessOpts] {
	return func(o *CheckAccessOpts) {
		o.Prefix = f
	}
}

func WithPathParamKey(keys ...string) Option[*CheckAccessOpts] {
	return func(o *CheckAccessOpts) {
		o.PathParamKeys = keys
	}
}

func WithTemplates(templates map[ClientType]ComparisonTemplates) Option[*CheckAccessOpts] {

	return func(o *CheckAccessOpts) {
		ts := make(map[ClientType]ComparisonTemplates)

		for k, t := range templates {

			switch t.MatchType {
			case MatchTypeEqual, MatchTypePrefix:
			default:
				t.MatchType = defaultMatchType
			}

			switch k {
			case ClientTypeAdmin:
				ts[ClientTypeAdmin] = t
			case ClientTypeGroup:
				ts[ClientTypeGroup] = t
			case ClientTypeTeam:
				ts[ClientTypeTeam] = t
			default:
				logr.FromContextOrDiscard(context.Background()).Error(nil, "unknown client type", "clientType", k)
			}
		}
		o.Templates = ts
	}
}

/*
NewCheckAccessMiddlewareWithOpts creates a new middleware that checks if the client has access to the requested resource

# It is expected that `business_context` middleware is executed before this middleware

The middleware checks the client's context and access rights to determine if the client has access to the requested resource.

As this middleware depends on path-params it has to be configured on the route level
```go
app.Get("/api/v1/foos/:namespace/:name", NewCheckAccessMiddlewareWithOpts(), handler)
```
*/
func NewCheckAccessMiddlewareWithOpts(opts ...Option[*CheckAccessOpts]) fiber.Handler {
	mwOpts := CheckAccessOpts{
		PathParamKeys:        []string{"namespace", "name"},
		ExpectedResourcePath: defaultExpectedResourcePath,
	}

	for _, f := range opts {
		f(&mwOpts)
	}
	if mwOpts.Prefix == nil {
		mwOpts.Prefix = toDatastorePrefix(mwOpts.ExpectedResourcePath, mwOpts.Templates)
	}

	if len(mwOpts.Templates) == 0 {
		mwOpts.Templates = map[ClientType]ComparisonTemplates{
			ClientTypeTeam: {
				ExpectedTemplate:  "{{ .B.Environment }}--{{ .B.Group }}--{{ .B.Team }}/",
				UserInputTemplate: "{{ .P.Namespace }}/{{ .P.Name }}",
				MatchType:         MatchTypePrefix,
			},
			ClientTypeGroup: {
				ExpectedTemplate:  "{{ .B.Environment }}--{{ .B.Group }}--",
				UserInputTemplate: "{{ .P.Namespace }}/{{ .P.Name }}",
				MatchType:         MatchTypePrefix,
			},
			ClientTypeAdmin: {
				ExpectedTemplate:  "{{ .B.Environment }}--",
				UserInputTemplate: "{{ .P.Namespace }}/{{ .P.Name }}",
				MatchType:         MatchTypePrefix,
			},
		}
	}
	return func(c *fiber.Ctx) error {
		params := extractParams(c, mwOpts.PathParamKeys...)
		bCtx, ok := util.NotNilOfType[*BusinessContext](c.Locals("businessContext"))
		if !ok {
			return c.Status(accessDenied.Code()).JSON(accessDenied, "application/problem+json")
		}
		// expectedNamespace := mwOpts.ExpectedNamespace(bCtx)

		allow := false
		var err error
		if areParamsEmpty(params) {
			if isClientTypeSupported(bCtx.ClientType, mwOpts.Templates) {
				allow = CheckGlobalRequest(c, bCtx)
				writeLog(c, bCtx, "global", allow)
			} else {
				writeLog(c, bCtx, "global", allow)
			}
		} else {
			allow, err = CheckNamespacedRequest(c, params, mwOpts.Templates, bCtx)
			if err != nil {
				logr.FromContextOrDiscard(c.UserContext()).Error(err, "failed to check access", "template", mwOpts.Templates[bCtx.ClientType], "params", params)
				return c.Status(accessDenied.Code()).JSON(accessDenied, "application/problem+json")
			}
			writeLog(c, bCtx, "namespaced", allow)
		}

		if allow {
			var prefix string
			prefix, err = mwOpts.Prefix(bCtx, params, mwOpts.Templates)
			if err != nil {
				logr.FromContextOrDiscard(c.UserContext()).Error(err, "failed to calculate prefix", "template", mwOpts.Templates[bCtx.ClientType], "params", params)
				return c.Status(accessDenied.Code()).JSON(accessDenied, "application/problem+json")
			}
			c.Locals("prefix", prefix)

			ctx := c.UserContext()
			ctx = context.WithValue(ctx, prefixKey, prefix)
			ctx = ToContext(ctx, bCtx)
			c.SetUserContext(ctx)
			return c.Next()
		}
		return c.Status(accessDenied.Code()).JSON(accessDenied, "application/problem+json")
	}
}

func areParamsEmpty(params map[string]string) bool {
	for _, v := range params {
		if v != "" {
			return false
		}
	}
	return true
}

func extractParams(c *fiber.Ctx, keys ...string) map[string]string {
	params := make(map[string]string)
	for _, key := range keys {
		params[key] = c.Params(key)
	}
	return params
}

func IsReadyOnlyRequest(c *fiber.Ctx) bool {
	return c.Method() == "GET" || c.Method() == "HEAD"
}

// CheckGlobalRequest performs access checks for global requests
// Global requests are requests are all requests that do not have a direct resource reference
// e.g. requests for to list resource `GET /api/v1/foos`
func CheckGlobalRequest(c *fiber.Ctx, bCtx *BusinessContext) (allow bool) {
	if bCtx.ClientType == ClientTypeAdmin {
		if IsReadyOnlyRequest(c) {
			allow = true
		} else {
			allow = bCtx.AccessType == AccessTypeReadWrite
		}
	}

	if bCtx.ClientType == ClientTypeGroup {
		if IsReadyOnlyRequest(c) {
			allow = bCtx.AccessType.IsRead()
		} else {
			allow = bCtx.AccessType.IsWrite()
		}
	}

	if bCtx.ClientType == ClientTypeTeam {
		if IsReadyOnlyRequest(c) {
			allow = bCtx.AccessType.IsRead()
		} else {
			allow = bCtx.AccessType.IsWrite()
		}
	}
	return allow
}

// CheckNamespacedRequest performs access checks for namespaced requests
// Namespaced requests are requests that have a direct resource reference
// e.g. requests for a specific resource `GET /api/v1/foos/<namespace>/<name>`
// It does to by matching the prefix of the namespace with the client's context
// and checking if the client has the required access rights
func CheckNamespacedRequest(c *fiber.Ctx, params map[string]string, templates map[ClientType]ComparisonTemplates, bCtx *BusinessContext) (allow bool, err error) {
	compareCtxInfo := NewCompareCtxInfo(bCtx, params)

	if bCtx.ClientType == ClientTypeAdmin {
		var adminAllow bool
		adminAllow, err = shouldAllow(templates[ClientTypeAdmin], compareCtxInfo)
		if err != nil {
			return false, err
		}
		if IsReadyOnlyRequest(c) {
			allow = adminAllow
		} else {
			allow = bCtx.AccessType.IsWrite() && adminAllow
		}
	}

	if bCtx.ClientType == ClientTypeGroup {
		var groupAllowed bool
		groupAllowed, err = shouldAllow(templates[ClientTypeGroup], compareCtxInfo)
		if err != nil {
			return false, err
		}
		if IsReadyOnlyRequest(c) {
			allow = groupAllowed && bCtx.AccessType.IsRead()
		} else {
			allow = groupAllowed && bCtx.AccessType.IsWrite()
		}
	}

	if bCtx.ClientType == ClientTypeTeam {
		var teamAllowed bool
		teamAllowed, err = shouldAllow(templates[ClientTypeTeam], compareCtxInfo)
		if err != nil {
			return false, err
		}
		if IsReadyOnlyRequest(c) {
			allow = teamAllowed && bCtx.AccessType.IsRead()
		} else {
			allow = teamAllowed && bCtx.AccessType.IsWrite()
		}
	}
	return allow, nil

}

func shouldAllow(template ComparisonTemplates, compareCtxInfo CompareCtxInfo) (bool, error) {
	var err error
	var allow bool
	matcher := NewMatcher(template.ExpectedTemplate, template.UserInputTemplate)
	switch template.MatchType {
	case MatchTypeEqual:
		allow, err = matcher.FullMatch(compareCtxInfo)
	case MatchTypePrefix:
		allow, err = matcher.StartsWith(compareCtxInfo)
	default:
		return false, fmt.Errorf("unknown MatchType: %v", template.MatchType)
	}
	return allow, err
}

func isClientTypeSupported(clientType ClientType, templates map[ClientType]ComparisonTemplates) bool {
	_, ok := templates[clientType]
	return ok
}

func writeLog(c *fiber.Ctx, bCtx *BusinessContext, requestType string, allow bool) {
	logArgs := []interface{}{"method", c.Method(), "type", requestType, "env", bCtx.Environment, "group", bCtx.Group, "team", bCtx.Team, "clientType", bCtx.ClientType, "accessType", bCtx.AccessType}
	if allow {
		logr.FromContextOrDiscard(c.UserContext()).Info("Access granted", logArgs...)
	} else {
		logr.FromContextOrDiscard(c.UserContext()).Info("Access denied", logArgs...)
	}
}
