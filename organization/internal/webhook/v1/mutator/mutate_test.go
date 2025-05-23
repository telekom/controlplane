// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package mutator

import (
	"context"
	"fmt"
	"testing"

	. "github.com/onsi/gomega"
	organizationv1 "github.com/telekom/controlplane/organization/api/v1"
	"github.com/telekom/controlplane/organization/internal/secret"
	"github.com/telekom/controlplane/organization/internal/secret/mock"
	"github.com/telekom/controlplane/secret-manager/api"
	"github.com/telekom/controlplane/secret-manager/api/gen"
)

// Future improvement: use automated mocks such as: https://github.com/uber-go/mock or https://github.com/stretchr/testify?tab=readme-ov-file#mock-package

func TestMutateSecret(t *testing.T) {
	RegisterTestingT(t)
	tests := []struct {
		name              string
		teamObj           *organizationv1.Team
		env               string
		mock              func() api.SecretManager
		mockFindSecretId  func([]gen.ListSecretItem, string) (string, bool)
		expectedError     bool
		expectedNewSecret bool
	}{
		{
			name:              "Empty Inserts (new team or empty secret)",
			teamObj:           &organizationv1.Team{},
			env:               "",
			mock:              mock.SecretManager,
			expectedError:     false,
			expectedNewSecret: true,
		},
		{
			name: "Set secret to Rotate keyword rotate",
			env:  "env",
			teamObj: &organizationv1.Team{
				Spec: organizationv1.TeamSpec{
					Secret: "rotate",
				},
			},
			mock:              mock.SecretManager,
			expectedError:     false,
			expectedNewSecret: true,
		},
		{
			name: "Have a secret value already set",
			env:  "env",
			teamObj: &organizationv1.Team{
				Spec: organizationv1.TeamSpec{
					Secret: "super-secret",
				},
			},
			mock:              mock.SecretManager,
			expectedError:     false,
			expectedNewSecret: false,
		},
		{
			name:              "Faulty Mock: Empty Inserts (new team or empty secret)",
			teamObj:           &organizationv1.Team{},
			env:               "",
			mock:              faultySecretManager,
			expectedError:     true,
			expectedNewSecret: false,
		},
		{
			name: "Faulty Mock: Set secret to Rotate keyword rotate",
			env:  "env",
			teamObj: &organizationv1.Team{
				Spec: organizationv1.TeamSpec{
					Secret: "rotate",
				},
			},
			mock:              faultySecretManager,
			expectedError:     true,
			expectedNewSecret: false,
		},
		{
			name: "Faulty Mock: Have a secret value already set",
			env:  "env",
			teamObj: &organizationv1.Team{
				Spec: organizationv1.TeamSpec{
					Secret: "super-secret",
				},
			},
			mock:              faultySecretManager,
			expectedError:     false,
			expectedNewSecret: false,
		},
		{
			name: "Faulty FindSecret: Upsert, but return no availableSecrets",
			env:  "env",
			teamObj: &organizationv1.Team{
				Spec: organizationv1.TeamSpec{
					Secret: "",
				},
			},
			mock:              mock.SecretManager,
			mockFindSecretId:  emptyAvailableSecrets,
			expectedError:     true,
			expectedNewSecret: false,
		},
		{
			name: "Faulty FindSecret: Rotate, but return no availableSecrets",
			env:  "env",
			teamObj: &organizationv1.Team{
				Spec: organizationv1.TeamSpec{
					Secret: "rotate",
				},
			},
			mock:              mock.SecretManager,
			mockFindSecretId:  emptyAvailableSecrets,
			expectedError:     true,
			expectedNewSecret: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			previousSecret := tt.teamObj.Spec.Secret
			secret.GetSecretManager = tt.mock
			if tt.mockFindSecretId != nil {
				secret.FindSecretId = tt.mockFindSecretId
			}
			err := MutateSecret(context.Background(), tt.env, tt.teamObj)
			if tt.expectedError {
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failure during communication with secret-manager when doing"))
			} else {
				Expect(err).NotTo(HaveOccurred())
			}
			if tt.expectedNewSecret {
				Expect(tt.teamObj.Spec.Secret).NotTo(Equal(previousSecret))
			} else {
				Expect(tt.teamObj.Spec.Secret).To(Equal(previousSecret))
			}
		})
	}
}

func faultySecretManager() api.SecretManager {
	return &faultyMock{}
}

type faultyMock struct {
}

func (f faultyMock) Get(ctx context.Context, secretID string) (value string, err error) {
	return "", fmt.Errorf("faulty mock")
}

func (f faultyMock) Set(ctx context.Context, secretID string, secretValue string) (newID string, err error) {
	return "", fmt.Errorf("faulty mock")
}

func (f faultyMock) Rotate(ctx context.Context, secretID string) (newID string, err error) {
	return "", fmt.Errorf("faulty mock")
}

func (f faultyMock) UpsertEnvironment(ctx context.Context, envID string) (availableSecrets []gen.ListSecretItem, err error) {
	return []gen.ListSecretItem{}, fmt.Errorf("faulty mock")
}

func (f faultyMock) UpsertTeam(ctx context.Context, envID, teamID string) (availableSecrets []gen.ListSecretItem, err error) {
	return []gen.ListSecretItem{}, fmt.Errorf("faulty mock")
}

func (f faultyMock) UpsertApplication(ctx context.Context, envID, teamID, appID string) (availableSecrets []gen.ListSecretItem, err error) {
	return []gen.ListSecretItem{}, fmt.Errorf("faulty mock")
}

func (f faultyMock) DeleteEnvironment(ctx context.Context, envID string) (err error) {
	return fmt.Errorf("faulty mock")
}

func (f faultyMock) DeleteTeam(ctx context.Context, envID, teamID string) (err error) {
	return fmt.Errorf("faulty mock")
}

func (f faultyMock) DeleteApplication(ctx context.Context, envID, teamID, appID string) (err error) {
	return fmt.Errorf("faulty mock")
}

func emptyAvailableSecrets(availableSecrets []gen.ListSecretItem, secretType string) (string, bool) {
	return "", false
}
