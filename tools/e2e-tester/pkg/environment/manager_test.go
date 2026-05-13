// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package environment

import (
	"sort"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/telekom/controlplane/tools/e2e-tester/pkg/config"
)

func newTestManager(envs []config.Environments) *Manager {
	return NewManager(envs, config.RoverCtlConfig{Binary: "roverctl"})
}

var _ = Describe("Manager", func() {

	Describe("NewManager", func() {
		It("should create a manager with the given environments", func() {
			envs := []config.Environments{
				{Name: "env-1", Token: "token-1"},
			}
			m := newTestManager(envs)
			Expect(m).NotTo(BeNil())
			Expect(m.environments).To(HaveLen(1))
		})
	})

	Describe("GetEnvironment", func() {
		var m *Manager

		BeforeEach(func() {
			m = newTestManager([]config.Environments{
				{Name: "env-a", Token: "token-a"},
				{Name: "env-b", Token: "token-b"},
			})
		})

		It("should return an existing environment", func() {
			env, err := m.GetEnvironment("env-a")
			Expect(err).NotTo(HaveOccurred())
			Expect(env.Name).To(Equal("env-a"))
			Expect(env.Token).To(Equal("token-a"))
		})

		It("should return an error for a non-existing environment", func() {
			_, err := m.GetEnvironment("env-c")
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("GetAllEnvironments", func() {
		It("should return all environments", func() {
			m := newTestManager([]config.Environments{
				{Name: "env-a", Token: "token-a"},
				{Name: "env-b", Token: "token-b"},
			})
			Expect(m.GetAllEnvironments()).To(HaveLen(2))
		})
	})

	Describe("ResolveTokenFromEnv", func() {
		It("should resolve env: prefix tokens", func() {
			GinkgoT().Setenv("TEST_TOKEN_A", "resolved-token-a")

			m := newTestManager([]config.Environments{
				{Name: "env-a", Token: "env:TEST_TOKEN_A"},
			})
			Expect(m.ResolveTokenFromEnv()).To(Succeed())
			Expect(m.environments[0].Token).To(Equal("resolved-token-a"))
		})

		It("should leave direct tokens unchanged", func() {
			m := newTestManager([]config.Environments{
				{Name: "env-a", Token: "direct-token"},
			})
			Expect(m.ResolveTokenFromEnv()).To(Succeed())
			Expect(m.environments[0].Token).To(Equal("direct-token"))
		})

		It("should return an error for unset env variables", func() {
			m := newTestManager([]config.Environments{
				{Name: "env-a", Token: "env:UNSET_TOKEN_VAR_E2E_TEST"},
			})
			Expect(m.ResolveTokenFromEnv()).To(HaveOccurred())
		})
	})

	Describe("ResolveVariablesFromEnv", func() {
		It("should resolve env: prefix variables", func() {
			GinkgoT().Setenv("MY_VAR", "resolved-value")

			m := newTestManager([]config.Environments{{
				Name:      "env-a",
				Token:     "token",
				Variables: []config.Variable{{Name: "var1", Value: "env:MY_VAR"}},
			}})
			Expect(m.ResolveVariablesFromEnv()).To(Succeed())
			Expect(m.environments[0].Variables[0].Value).To(Equal("resolved-value"))
		})

		It("should leave direct variables unchanged", func() {
			m := newTestManager([]config.Environments{{
				Name:      "env-a",
				Token:     "token",
				Variables: []config.Variable{{Name: "var1", Value: "static-value"}},
			}})
			Expect(m.ResolveVariablesFromEnv()).To(Succeed())
			Expect(m.environments[0].Variables[0].Value).To(Equal("static-value"))
		})

		It("should return an error for unset env variables", func() {
			m := newTestManager([]config.Environments{{
				Name:      "env-a",
				Token:     "token",
				Variables: []config.Variable{{Name: "var1", Value: "env:UNSET_VAR_E2E_TEST"}},
			}})
			Expect(m.ResolveVariablesFromEnv()).To(HaveOccurred())
		})
	})

	Describe("SetupTestEnvironment", func() {
		It("should return an environment by name", func() {
			m := newTestManager([]config.Environments{
				{Name: "env-a", Token: "token-a"},
			})
			env, err := m.SetupTestEnvironment("env-a")
			Expect(err).NotTo(HaveOccurred())
			Expect(env.Name).To(Equal("env-a"))
		})

		It("should resolve env: token on the fly", func() {
			GinkgoT().Setenv("SETUP_TOKEN", "resolved-setup-token")

			m := newTestManager([]config.Environments{
				{Name: "env-a", Token: "env:SETUP_TOKEN"},
			})
			env, err := m.SetupTestEnvironment("env-a")
			Expect(err).NotTo(HaveOccurred())
			Expect(env.Token).To(Equal("resolved-setup-token"))
		})

		It("should return an error for unset env token", func() {
			m := newTestManager([]config.Environments{
				{Name: "env-a", Token: "env:MISSING_SETUP_TOKEN_E2E_TEST"},
			})
			_, err := m.SetupTestEnvironment("env-a")
			Expect(err).To(HaveOccurred())
		})

		It("should return an error for unknown environment", func() {
			m := newTestManager(nil)
			_, err := m.SetupTestEnvironment("nonexistent")
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("ValidateEnvironments", func() {
		var m *Manager

		BeforeEach(func() {
			m = newTestManager([]config.Environments{
				{Name: "env-a", Token: "token-a"},
				{Name: "env-b", Token: "token-b"},
			})
		})

		It("should succeed for valid environments", func() {
			Expect(m.ValidateEnvironments([]string{"env-a", "env-b"})).To(Succeed())
		})

		It("should ignore empty strings", func() {
			Expect(m.ValidateEnvironments([]string{"env-a", ""})).To(Succeed())
		})

		It("should return an error for invalid environments", func() {
			Expect(m.ValidateEnvironments([]string{"env-a", "nonexistent"})).To(HaveOccurred())
		})
	})

	Describe("CollectEnvironmentNames", func() {
		It("should collect unique environment names from suites and cases", func() {
			m := newTestManager(nil)

			suites := []config.Suite{
				{
					Name:         "suite-1",
					Environments: []string{"env-a", "env-b"},
					Cases:        []*config.Case{{Name: "case-1", Environment: "env-c"}},
				},
				{
					Name:         "suite-2",
					Environments: []string{"env-b"},
					Cases:        []*config.Case{{Name: "case-2", Environment: ""}},
				},
			}

			names := m.CollectEnvironmentNames(suites)
			sort.Strings(names)
			Expect(names).To(Equal([]string{"env-a", "env-b", "env-c"}))
		})
	})

	Describe("GetExecutor", func() {
		var m *Manager

		BeforeEach(func() {
			m = newTestManager([]config.Environments{
				{Name: "env-a", Token: "token-a"},
			})
		})

		It("should create and cache an executor", func() {
			exec1, err := m.GetExecutor("env-a")
			Expect(err).NotTo(HaveOccurred())
			Expect(exec1).NotTo(BeNil())

			exec2, err := m.GetExecutor("env-a")
			Expect(err).NotTo(HaveOccurred())
			Expect(exec2).To(BeIdenticalTo(exec1))
		})

		It("should default to first environment when empty", func() {
			exec, err := m.GetExecutor("")
			Expect(err).NotTo(HaveOccurred())
			Expect(exec).NotTo(BeNil())
		})

		It("should return an error for unknown environment", func() {
			_, err := m.GetExecutor("nonexistent")
			Expect(err).To(HaveOccurred())
		})
	})
})
