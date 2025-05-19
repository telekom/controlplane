// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package security_test

import (
	"testing"

	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
	"github.com/telekom/controlplane/common-server/pkg/server/middleware/security"
)

type testEnvironment struct {
	Name      string
	testCases []testCase
}
type testCase struct {
	Name              string
	ExpectedTemplate  string
	UserInputTemplate string
	ctxInfo           security.CompareCtxInfo
	Be                func() types.GomegaMatcher
}

func TestMatcher_StartsWith(t *testing.T) {
	RegisterTestingT(t)

	var NewCtxInfo = func(pathParams ...string) security.CompareCtxInfo {
		b := security.BusinessContext{
			Environment: "test",
			Group:       "eni",
			Team:        "hyperion",
			ClientType:  security.ClientTypeTeam,
			AccessType:  security.AccessTypeReadWrite,
		}

		params := make(map[string]string)
		for i := 0; i < len(pathParams); i += 2 {
			params[pathParams[i]] = pathParams[i+1]
		}

		return security.NewCompareCtxInfo(&b, params)
	}

	testEnvironments := []testEnvironment{
		{
			Name: "Team-Resource: namespace==<env--group--team>, name==<anyName>",
			testCases: []testCase{
				{
					Name:              "clientType==group",
					ExpectedTemplate:  "{{ .B.Environment }}--{{ .B.Group }}--",
					UserInputTemplate: "{{ .P.Namespace }}/{{ .P.Name }}",
					ctxInfo:           NewCtxInfo("namespace", "test--eni--hyper", "name", "foo-app"),
					Be:                BeTrue,
				},
				{
					Name:              "clientType==group and shorter group name",
					ExpectedTemplate:  "{{ .B.Environment }}--{{ .B.Group }}--",
					UserInputTemplate: "{{ .P.Namespace }}/{{ .P.Name }}",
					ctxInfo:           NewCtxInfo("namespace", "test--en--hyper", "name", "foo-app"),
					Be:                BeFalse,
				},
				{
					Name:              "clientType==group and longer group name",
					ExpectedTemplate:  "{{ .B.Environment }}--{{ .B.Group }}--",
					UserInputTemplate: "{{ .P.Namespace }}/{{ .P.Name }}",
					ctxInfo:           NewCtxInfo("namespace", "test--enigma--hyper", "name", "foo-app"),
					Be:                BeFalse,
				},
				{
					Name:              "clientType==group and different group name",
					ExpectedTemplate:  "{{ .B.Environment }}--{{ .B.Group }}--",
					UserInputTemplate: "{{ .P.Namespace }}/{{ .P.Name }}",
					ctxInfo:           NewCtxInfo("namespace", "test--alpha--hyper", "name", "foo-app"),
					Be:                BeFalse,
				},
				{
					Name:              "clientType==admin",
					ExpectedTemplate:  "{{ .B.Environment }}--",
					UserInputTemplate: "{{ .P.Namespace }}/{{ .P.Name }}",
					ctxInfo:           NewCtxInfo("namespace", "test--other-group--hyper", "name", "foo-app"),
					Be:                BeTrue,
				},
				{
					Name:              "clientType==admin and short environment name",
					ExpectedTemplate:  "{{ .B.Environment }}--",
					UserInputTemplate: "{{ .P.Namespace }}/{{ .P.Name }}",
					ctxInfo:           NewCtxInfo("namespace", "te--other-group--hyper", "name", "foo-app"),
					Be:                BeFalse,
				},
				{
					Name:              "clientType==admin and long environment name",
					ExpectedTemplate:  "{{ .B.Environment }}--",
					UserInputTemplate: "{{ .P.Namespace }}/{{ .P.Name }}",
					ctxInfo:           NewCtxInfo("namespace", "testenv--other-group--hyper", "name", "foo-app"),
					Be:                BeFalse,
				},
				{
					Name:              "clientType==admin and different environment name",
					ExpectedTemplate:  "{{ .B.Environment }}--",
					UserInputTemplate: "{{ .P.Namespace }}/{{ .P.Name }}",
					ctxInfo:           NewCtxInfo("namespace", "poc--other-group--hyper", "name", "foo-app"),
					Be:                BeFalse,
				},
			},
		},
		{
			Name: "Rover-Resource: resourceId==<group--team--name>",
			testCases: []testCase{
				{
					Name:              "clientType==group",
					ExpectedTemplate:  "{{ .B.Environment }}--{{ .B.Group }}--",
					UserInputTemplate: "{{ .B.Environment }}--{{ .P.Resourceid }}",
					ctxInfo:           NewCtxInfo("resourceId", "eni--hyper--foo-app"),
					Be:                BeTrue,
				},
				{
					Name:              "clientType==admin",
					ExpectedTemplate:  "{{ .B.Environment }}--",
					UserInputTemplate: "{{ .B.Environment }}--{{ .P.Resourceid }}",
					ctxInfo:           NewCtxInfo("resourceId", "other-group--hyper"),
					Be:                BeTrue,
				},
			},
		},
		{
			Name: "Environment-Team: namespace==<env>, name==<group--team>",
			testCases: []testCase{
				{
					Name:              "clientType==group",
					ExpectedTemplate:  "{{ .B.Environment }}/{{ .B.Group }}--",
					UserInputTemplate: "{{ .P.Environment }}/{{ .P.Name }}",
					ctxInfo:           NewCtxInfo("environment", "test", "name", "eni--hyper"),
					Be:                BeTrue,
				},
				{
					Name:              "clientType==admin",
					ExpectedTemplate:  "{{ .B.Environment }}/",
					UserInputTemplate: "{{ .P.Environment }}/{{ .P.Name }}",
					ctxInfo:           NewCtxInfo("environment", "test", "name", "foo--hyper"),
					Be:                BeTrue,
				},
			},
		},
		{
			Name: "Environment-Group: namespace==<env>, name==<group>",
			testCases: []testCase{
				{
					Name:              "clientType==group",
					ExpectedTemplate:  "{{ .B.Environment }}/{{ .B.Group }}",
					UserInputTemplate: "{{ .P.Environment }}/{{ .P.Name }}",
					ctxInfo:           NewCtxInfo("environment", "test", "name", "eni"),
					Be:                BeTrue,
				},
				{
					Name:              "clientType==admin",
					ExpectedTemplate:  "{{ .B.Environment }}/",
					UserInputTemplate: "{{ .P.Environment }}/{{ .P.Name }}",
					ctxInfo:           NewCtxInfo("environment", "test", "name", "foo"),
					Be:                BeTrue,
				},
			},
		},
	}

	for i := range testEnvironments {
		t.Run(testEnvironments[i].Name, func(tt *testing.T) {
			for _, tc := range testEnvironments[i].testCases {
				tt.Run(tc.Name, func(ttt *testing.T) {
					g := NewWithT(ttt)
					matcher := security.NewMatcher(tc.ExpectedTemplate, tc.UserInputTemplate)
					matches, err := matcher.StartsWith(tc.ctxInfo)
					g.Expect(err).ToNot(HaveOccurred())
					g.Expect(matches).To(tc.Be())
				})
			}
		})

	}
}

func TestMatcher_FullMatch(t *testing.T) {
	RegisterTestingT(t)

	var NewCtxInfo = func(pathParams ...string) security.CompareCtxInfo {
		b := security.BusinessContext{
			Environment: "test",
			Group:       "eni",
			Team:        "hyperion",
			ClientType:  security.ClientTypeTeam,
			AccessType:  security.AccessTypeReadWrite,
		}

		params := make(map[string]string)
		for i := 0; i < len(pathParams); i += 2 {
			params[pathParams[i]] = pathParams[i+1]
		}

		return security.NewCompareCtxInfo(&b, params)
	}

	testEnvironments := []testEnvironment{
		{
			Name: "Team-Resource: namespace==<env--group--team>, name==<anyName>",
			testCases: []testCase{
				{
					Name:              "clientType==team",
					ExpectedTemplate:  "{{ .B.Environment }}--{{ .B.Group }}--{{ .B.Team }}",
					UserInputTemplate: "{{ .P.Namespace }}/{{ .P.Name }}",
					ctxInfo:           NewCtxInfo("namespace", "test--eni--hyperion", "name", "foo-app"),
					Be:                BeFalse, // removal of .P.Name happens somewhere else
				},
				{
					Name:              "clientType==team and shorter team name",
					ExpectedTemplate:  "{{ .B.Environment }}--{{ .B.Group }}--{{ .B.Team }}",
					UserInputTemplate: "{{ .P.Namespace }}/{{ .P.Name }}",
					ctxInfo:           NewCtxInfo("namespace", "test--eni--hyper", "name", "foo-app"),
					Be:                BeFalse,
				},
				{
					Name:              "clientType==team and longer team name",
					ExpectedTemplate:  "{{ .B.Environment }}--{{ .B.Group }}--{{ .B.Team }}",
					UserInputTemplate: "{{ .P.Namespace }}/{{ .P.Name }}",
					ctxInfo:           NewCtxInfo("namespace", "test--eni--hyperionship", "name", "foo-app"),
					Be:                BeFalse,
				},
				{
					Name:              "clientType==team and different team name",
					ExpectedTemplate:  "{{ .B.Environment }}--{{ .B.Group }}--{{ .B.Team }}",
					UserInputTemplate: "{{ .P.Namespace }}/{{ .P.Name }}",
					ctxInfo:           NewCtxInfo("namespace", "test--eni--jupiter", "name", "foo-app"),
					Be:                BeFalse,
				},
			},
		},
		{
			Name: "Rover-Resource: resourceId==<group--team--name>",
			testCases: []testCase{
				{
					Name:              "clientType==team",
					ExpectedTemplate:  "{{ .B.Environment }}--{{ .B.Group }}--{{ .B.Team }}",
					UserInputTemplate: "{{ .B.Environment }}--{{ .P.Resourceid }}",
					ctxInfo:           NewCtxInfo("resourceId", "eni--hyperion--foo-app"),
					Be:                BeFalse, // removal of .P.ResourceId happens somewhere else
				},
				{
					Name:              "clientType==team and shorter team name",
					ExpectedTemplate:  "{{ .B.Environment }}--{{ .B.Group }}--{{ .B.Team }}",
					UserInputTemplate: "{{ .B.Environment }}--{{ .P.Resourceid }}",
					ctxInfo:           NewCtxInfo("resourceId", "eni--hyper--foo-app"),
					Be:                BeFalse,
				},
				{
					Name:              "clientType==team and longer team name",
					ExpectedTemplate:  "{{ .B.Environment }}--{{ .B.Group }}--{{ .B.Team }}",
					UserInputTemplate: "{{ .B.Environment }}--{{ .P.Resourceid }}",
					ctxInfo:           NewCtxInfo("resourceId", "eni--hyperionship--foo-app"),
					Be:                BeFalse,
				},
				{
					Name:              "clientType==team and different team name",
					ExpectedTemplate:  "{{ .B.Environment }}--{{ .B.Group }}--{{ .B.Team }}",
					UserInputTemplate: "{{ .B.Environment }}--{{ .P.Resourceid }}",
					ctxInfo:           NewCtxInfo("resourceId", "eni--jupiter--foo-app"),
					Be:                BeFalse,
				},
			},
		},
		{
			Name: "Environment-Team: namespace==<env>, name==<group--team>",
			testCases: []testCase{
				{
					Name:              "clientType==team",
					ExpectedTemplate:  "{{ .B.Environment }}/{{ .B.Group }}--{{ .B.Team }}",
					UserInputTemplate: "{{ .P.Environment }}/{{ .P.Name }}",
					ctxInfo:           NewCtxInfo("environment", "test", "name", "eni--hyperion"),
					Be:                BeTrue,
				},
				{
					Name:              "clientType==team and shorter team name",
					ExpectedTemplate:  "{{ .B.Environment }}/{{ .B.Group }}--{{ .B.Team }}",
					UserInputTemplate: "{{ .P.Environment }}/{{ .P.Name }}",
					ctxInfo:           NewCtxInfo("environment", "test", "name", "eni--hyper"),
					Be:                BeFalse,
				},
				{
					Name:              "clientType==team and longer team name",
					ExpectedTemplate:  "{{ .B.Environment }}/{{ .B.Group }}--{{ .B.Team }}",
					UserInputTemplate: "{{ .P.Environment }}/{{ .P.Name }}",
					ctxInfo:           NewCtxInfo("environment", "test", "name", "eni--hyperionship"),
					Be:                BeFalse,
				},
				{
					Name:              "clientType==team and different team name",
					ExpectedTemplate:  "{{ .B.Environment }}/{{ .B.Group }}--{{ .B.Team }}",
					UserInputTemplate: "{{ .P.Environment }}/{{ .P.Name }}",
					ctxInfo:           NewCtxInfo("environment", "test", "name", "eni--jupiter"),
					Be:                BeFalse,
				},
			},
		},
		{
			Name: "Environment-Group: namespace==<env>, name==<group>",
			testCases: []testCase{
				{
					Name:              "clientType==group",
					ExpectedTemplate:  "{{ .B.Environment }}/{{ .B.Group }}",
					UserInputTemplate: "{{ .P.Environment }}/{{ .P.Group }}",
					ctxInfo:           NewCtxInfo("environment", "test", "group", "eni"),
					Be:                BeTrue,
				},
				{
					Name:              "clientType==group and shorter group name",
					ExpectedTemplate:  "{{ .B.Environment }}/{{ .B.Group }}",
					UserInputTemplate: "{{ .P.Environment }}/{{ .P.Group }}",
					ctxInfo:           NewCtxInfo("environment", "test", "group", "en"),
					Be:                BeFalse,
				},
				{
					Name:              "clientType==group and longer group name",
					ExpectedTemplate:  "{{ .B.Environment }}/{{ .B.Group }}",
					UserInputTemplate: "{{ .P.Environment }}/{{ .P.Group }}",
					ctxInfo:           NewCtxInfo("environment", "test", "group", "enigma"),
					Be:                BeFalse,
				},
				{
					Name:              "clientType==group and different group name",
					ExpectedTemplate:  "{{ .B.Environment }}/{{ .B.Group }}",
					UserInputTemplate: "{{ .P.Environment }}/{{ .P.Group }}",
					ctxInfo:           NewCtxInfo("environment", "test", "group", "alpha"),
					Be:                BeFalse,
				},
			},
		},
		{
			Name: "Mixed Environment in Domain: namespace==<env>, name==<group> or <group>--<team>",
			testCases: []testCase{
				{
					Name:              "clientType==group, <group>",
					ExpectedTemplate:  "{{ .B.Environment }}/{{ .B.Group }}{{ if contains .P.Name \"--\"}}--{{ lastPart .P.Name \"--\"}}{{ end }}",
					UserInputTemplate: "{{ .P.Environment }}/{{ .P.Name }}",
					ctxInfo:           NewCtxInfo("environment", "test", "name", "eni"),
					Be:                BeTrue,
				},
				{
					Name:              "clientType==group, <group>--<team>",
					ExpectedTemplate:  "{{ .B.Environment }}/{{ .B.Group }}{{ if contains .P.Name \"--\"}}--{{ lastPart .P.Name \"--\"}}{{ end }}",
					UserInputTemplate: "{{ .P.Environment }}/{{ .P.Name }}",
					ctxInfo:           NewCtxInfo("environment", "test", "name", "eni--team"),
					Be:                BeTrue,
				},
			},
		},
	}

	for i := range testEnvironments {
		t.Run(testEnvironments[i].Name, func(tt *testing.T) {
			for _, tc := range testEnvironments[i].testCases {
				tt.Run(tc.Name, func(ttt *testing.T) {
					g := NewWithT(ttt)
					matcher := security.NewMatcher(tc.ExpectedTemplate, tc.UserInputTemplate)
					matches, err := matcher.FullMatch(tc.ctxInfo)
					g.Expect(err).ToNot(HaveOccurred())
					g.Expect(matches).To(tc.Be())
				})
			}
		})

	}
}
