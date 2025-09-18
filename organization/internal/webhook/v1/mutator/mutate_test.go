// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package mutator

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/mock"

	. "github.com/onsi/gomega"
	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	organizationv1 "github.com/telekom/controlplane/organization/api/v1"
	"github.com/telekom/controlplane/organization/internal/secret"
	"github.com/telekom/controlplane/secret-manager/api"
	"github.com/telekom/controlplane/secret-manager/api/fake"
)

func TestMutateSecret(t *testing.T) {

	zone := &adminv1.Zone{
		Spec: adminv1.ZoneSpec{
			Gateway:          adminv1.GatewayConfig{Url: "https://example.com/gateway"},
			IdentityProvider: adminv1.IdentityProviderConfig{Url: "https://example.com/identity"},
		},
	}

	RegisterTestingT(t)
	tests := []struct {
		name              string
		teamObj           *organizationv1.Team
		zoneObj           *adminv1.Zone
		env               string
		mock              func() api.SecretManager
		mockFindSecretId  func(map[string]string, string) (string, bool)
		expectedError     bool
		expectedNewSecret bool
	}{
		{
			name:    "Empty Inserts (new team or empty secret)",
			teamObj: &organizationv1.Team{},
			zoneObj: zone,
			env:     "",
			mock: func() api.SecretManager {
				mockSecretManager := fake.NewMockSecretManager(t)
				mockSecretManager.EXPECT().
					UpsertTeam(mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
					Return(map[string]string{
						"clientSecret": "found",
						"teamToken":    "found",
					}, nil)
				return mockSecretManager
			},
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
			zoneObj: zone,
			mock: func() api.SecretManager {
				mockSecretManager := fake.NewMockSecretManager(t)
				mockSecretManager.EXPECT().
					UpsertTeam(mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
					Return(map[string]string{
						"clientSecret": "found",
						"teamToken":    "found",
					}, nil)
				return mockSecretManager
			},
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
			zoneObj: zone,
			mock: func() api.SecretManager {
				mockSecretManager := fake.NewMockSecretManager(t)
				mockSecretManager.EXPECT().
					UpsertTeam(mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
					Return(map[string]string{
						"clientSecret": "found",
						"teamToken":    "found",
					}, nil)
				return mockSecretManager
			},
			expectedError:     false,
			expectedNewSecret: false,
		},
		{
			name:    "Faulty Mock: Empty Inserts (new team or empty secret)",
			teamObj: &organizationv1.Team{},
			env:     "",
			zoneObj: zone,
			mock: func() api.SecretManager {
				mockSecretManager := fake.NewMockSecretManager(t)
				mockSecretManager.EXPECT().
					UpsertTeam(mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
					Return(nil, fmt.Errorf("faulty implementation"))
				return mockSecretManager
			},
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
			zoneObj: zone,
			mock: func() api.SecretManager {
				mockSecretManager := fake.NewMockSecretManager(t)
				mockSecretManager.EXPECT().
					UpsertTeam(mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
					Return(nil, fmt.Errorf("faulty implementation"))
				return mockSecretManager
			},
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
			zoneObj: zone,
			mock: func() api.SecretManager {
				mockSecretManager := fake.NewMockSecretManager(t)
				mockSecretManager.EXPECT().
					UpsertTeam(mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
					Return(nil, fmt.Errorf("faulty implementation"))
				return mockSecretManager
			},
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
			zoneObj: zone,
			mock: func() api.SecretManager {
				mockSecretManager := fake.NewMockSecretManager(t)
				mockSecretManager.EXPECT().
					UpsertTeam(mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
					Return(map[string]string{
						"clientSecret": "found",
						"teamToken":    "found",
					}, nil)
				return mockSecretManager
			},
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
			zoneObj: zone,
			mock: func() api.SecretManager {
				mockSecretManager := fake.NewMockSecretManager(t)
				mockSecretManager.EXPECT().
					UpsertTeam(mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
					Return(map[string]string{
						"clientSecret": "found",
						"teamToken":    "found",
					}, nil)
				return mockSecretManager
			},
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
			err := MutateSecret(context.Background(), tt.env, tt.teamObj, tt.zoneObj)
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

func emptyAvailableSecrets(_ map[string]string, _ string) (string, bool) {
	return "", false
}
