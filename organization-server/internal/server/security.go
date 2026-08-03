// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package server

import (
	"net/url"
	"strings"

	"github.com/telekom/controlplane/common-server/pkg/problems"
	"github.com/telekom/controlplane/common-server/pkg/server/middleware/security"
)

var SecurityTemplates = map[security.ClientType]security.ComparisonTemplates{
	security.ClientTypeTeam: {
		ExpectedTemplate:  "{{ .B.Group }}/{{ .B.Team }}",
		UserInputTemplate: "{{ .P.Hub }}/{{ if .P.Team }}{{ .P.Team }}{{ else }}{{ .B.Team }}{{ end }}",
		MatchType:         security.MatchTypeEqual,
	},
	security.ClientTypeGroup: {
		ExpectedTemplate:  "{{ .B.Group }}/",
		UserInputTemplate: "{{ .P.Hub }}/{{ .P.Team }}",
		MatchType:         security.MatchTypePrefix,
	},
	security.ClientTypeAdmin: {
		ExpectedTemplate:  "admin",
		UserInputTemplate: "admin",
		MatchType:         security.MatchTypePrefix,
	},
}

func EnvironmentDecoder(fallback string) security.ValueDecoder {
	return func(claims map[string]any, key string) (string, problems.Problem) {
		if environment, ok := claims[key].(string); ok && environment != "" {
			return environment, nil
		}

		environment := ""
		if issuer, ok := claims["iss"].(string); ok {
			environment = environmentFromIssuer(issuer)
		}
		if environment == "" {
			environment = fallback
		}
		if environment == "" {
			return "", problems.Unauthorized("Unauthorized", "Unable to determine environment")
		}
		return environment, nil
	}
}

func environmentFromIssuer(issuer string) string {
	parsed, err := url.Parse(issuer)
	if err != nil {
		return ""
	}
	segments := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	if len(segments) < 2 || segments[len(segments)-2] != "realms" {
		return ""
	}
	environment, found := strings.CutPrefix(segments[len(segments)-1], "team-")
	if !found {
		return ""
	}
	return environment
}
