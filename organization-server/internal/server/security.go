// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package server

import (
	"github.com/telekom/controlplane/common-server/pkg/server/middleware/security"
)

// SecurityTemplates restrict organization resources by comparing trusted JWT
// business context (.B) with the hub and team URL parameters (.P). For example,
// a team token for group-a/team-a exactly matches group-a/team-a; when the team
// parameter is absent, the token's team is used. A group token for group-a uses
// the group-a/ prefix to match every team in that hub. Admin compares constant
// values, so resource ownership is unrestricted; scopes and access type still
// control whether the request may read or write.
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
