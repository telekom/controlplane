// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package runner

import (
	"slices"
	"testing"

	"github.com/telekom/controlplane/tools/e2e-tester/pkg/config"
)

type testCase struct {
	name   string
	suite  *config.Suite
	want   []config.Case
	errMsg string
}

func TestSetupSuiteEnvironments(t *testing.T) {

	testCases := []testCase{
		{
			name:   "Env must be set on each case",
			errMsg: "suite 'test-suite' has no environments defined at either suite or case level",
			suite: &config.Suite{
				Name:         "test-suite",
				Environments: nil,
				Cases: []*config.Case{
					{
						Name: "test-case-1",
					},
					{
						Name: "test-case-2",
					},
				},
			},
		},
		{
			name:   "Env must be set on each case",
			errMsg: "test case 'test-case-2' in suite 'test-suite' does not have an environment defined, and the suite also does not define any environments",
			suite: &config.Suite{
				Name:         "test-suite",
				Environments: nil,
				Cases: []*config.Case{
					{
						Name:        "test-case-1",
						Environment: "test-env",
					},
					{
						Name: "test-case-2",
					},
				},
			},
		},
		{
			name: "Multiple suite environments with case envs",
			suite: &config.Suite{
				Name:         "test-suite",
				Environments: []string{"env-1", "env-2"},
				Cases: []*config.Case{
					{
						Name:        "test-case-1",
						Environment: "test-env",
					},
					{
						Name:        "test-case-2",
						Environment: "test-env-2",
					},
				},
			},
			errMsg: "suite 'test-suite' has multiple environments defined but some test cases also define specific environments. Please either define environments at the suite level or at the case level, not both",
		},
		{
			name: "Single suite environment as default",
			suite: &config.Suite{
				Name:         "test-suite",
				Environments: []string{"env-1"},
				Cases: []*config.Case{
					{
						Name:        "test-case-1",
						Environment: "env-2",
					},
					{
						Name: "test-case-2",
					},
				},
			},
			want: []config.Case{
				{
					Name:        "test-case-1",
					Environment: "env-2",
				},
				{
					Name:        "test-case-2",
					Environment: "env-1",
				},
			},
		},
		{
			name: "Single suite environment as default",
			suite: &config.Suite{
				Name:         "test-suite",
				Environments: nil,
				Cases: []*config.Case{
					{
						Name:        "test-case-1",
						Environment: "env-2",
					},
					{
						Name:        "test-case-2",
						Environment: "env-1",
					},
				},
			},
			want: []config.Case{
				{
					Name:        "test-case-1",
					Environment: "env-2",
				},
				{
					Name:        "test-case-2",
					Environment: "env-1",
				},
			},
		},
		{
			name: "Multiple suite environments split into multiple suites",
			suite: &config.Suite{
				Name:         "test-suite",
				Environments: []string{"env-1", "env-2"},
				Cases: []*config.Case{
					{
						Name: "test-case-1",
					},
					{
						Name: "test-case-2",
					},
				},
			},
			want: []config.Case{
				{
					Name:        "test-case-1",
					Environment: "env-1",
				},
				{
					Name:        "test-case-2",
					Environment: "env-1",
				},
				{
					Name:        "test-case-1",
					Environment: "env-2",
				},
				{
					Name:        "test-case-2",
					Environment: "env-2",
				},
			},
		},
	}

	t.Run("SetupSuiteEnvironments", func(t *testing.T) {
		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				suites, err := setupSuiteEnvironments(tc.suite)
				if tc.errMsg != "" {
					if err == nil || err.Error() != tc.errMsg {
						t.Errorf("expected error '%s', got '%v'", tc.errMsg, err)
					}
					return
				}
				if err != nil {
					t.Errorf("unexpected error: %v", err)
					return
				}
				var gotCases []*config.Case
				for _, suite := range suites {
					gotCases = append(gotCases, suite.Cases...)
				}
				if len(gotCases) != len(tc.want) {
					t.Errorf("expected %d cases, got %d", len(tc.want), len(gotCases))
					return
				}

				for _, expectedCase := range tc.want {
					found := slices.ContainsFunc(gotCases, func(c *config.Case) bool {
						return c.Name == expectedCase.Name && c.Environment == expectedCase.Environment
					})
					if !found {
						t.Errorf("expected case %q with env %q", expectedCase.Name, expectedCase.Environment)
					}
				}

			})
		}
	})

}
