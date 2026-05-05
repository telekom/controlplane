// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package mutator

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/mock"

	applicationv1 "github.com/telekom/controlplane/application/api/v1"
	"github.com/telekom/controlplane/application/internal/secret"
	"github.com/telekom/controlplane/secret-manager/api"
	"github.com/telekom/controlplane/secret-manager/api/fake"

	. "github.com/onsi/gomega"
)

func TestMutateSecret(t *testing.T) {
	RegisterTestingT(t)

	tests := []struct {
		name              string
		app               *applicationv1.Application
		env               string
		mock              func() api.SecretManager
		mockFindSecretId  func(map[string]string, string) (string, bool)
		expectedError     bool
		expectedNewSecret bool
	}{
		{
			name: "Empty secret generates new UUID",
			app: &applicationv1.Application{
				Spec: applicationv1.ApplicationSpec{
					Team:   "my-team",
					Secret: "",
				},
			},
			env: "dev",
			mock: func() api.SecretManager {
				m := fake.NewMockSecretManager(t)
				m.EXPECT().
					UpsertApplication(mock.Anything, "dev", "my-team", mock.Anything, mock.Anything).
					Return(map[string]string{"clientSecret": "$<new-ref>"}, nil)
				return m
			},
			expectedError:     false,
			expectedNewSecret: true,
		},
		{
			name: "Rotate keyword generates new UUID",
			app: &applicationv1.Application{
				Spec: applicationv1.ApplicationSpec{
					Team:   "my-team",
					Secret: "rotate",
				},
			},
			env: "dev",
			mock: func() api.SecretManager {
				m := fake.NewMockSecretManager(t)
				m.EXPECT().
					UpsertApplication(mock.Anything, "dev", "my-team", mock.Anything, mock.Anything).
					Return(map[string]string{"clientSecret": "$<new-ref>"}, nil)
				return m
			},
			expectedError:     false,
			expectedNewSecret: true,
		},
		{
			name: "Custom secret value is passed through",
			app: &applicationv1.Application{
				Spec: applicationv1.ApplicationSpec{
					Team:   "my-team",
					Secret: "my-custom-secret",
				},
			},
			env: "dev",
			mock: func() api.SecretManager {
				m := fake.NewMockSecretManager(t)
				m.EXPECT().
					UpsertApplication(mock.Anything, "dev", "my-team", mock.Anything, mock.Anything).
					Return(map[string]string{"clientSecret": "$<custom-ref>"}, nil)
				return m
			},
			expectedError:     false,
			expectedNewSecret: true,
		},
		{
			name: "Already a reference is a no-op",
			app: &applicationv1.Application{
				Spec: applicationv1.ApplicationSpec{
					Team:   "my-team",
					Secret: "$<existing-ref>",
				},
			},
			env: "dev",
			mock: func() api.SecretManager {
				return nil
			},
			expectedError:     false,
			expectedNewSecret: false,
		},
		{
			name: "Secret-manager error is propagated",
			app: &applicationv1.Application{
				Spec: applicationv1.ApplicationSpec{
					Team:   "my-team",
					Secret: "rotate",
				},
			},
			env: "dev",
			mock: func() api.SecretManager {
				m := fake.NewMockSecretManager(t)
				m.EXPECT().
					UpsertApplication(mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
					Return(nil, fmt.Errorf("connection refused"))
				return m
			},
			expectedError:     true,
			expectedNewSecret: false,
		},
		{
			name: "Missing client secret in response is an error",
			app: &applicationv1.Application{
				Spec: applicationv1.ApplicationSpec{
					Team:   "my-team",
					Secret: "rotate",
				},
			},
			env: "dev",
			mock: func() api.SecretManager {
				m := fake.NewMockSecretManager(t)
				m.EXPECT().
					UpsertApplication(mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
					Return(map[string]string{}, nil)
				return m
			},
			mockFindSecretId:  emptyAvailableSecrets,
			expectedError:     true,
			expectedNewSecret: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			previousSecret := tt.app.Spec.Secret
			secret.GetSecretManager = tt.mock
			if tt.mockFindSecretId != nil {
				secret.FindSecretId = tt.mockFindSecretId
			}
			err := MutateSecret(context.Background(), tt.env, tt.app)
			if tt.expectedError {
				Expect(err).To(HaveOccurred())
			} else {
				Expect(err).NotTo(HaveOccurred())
			}
			if tt.expectedNewSecret {
				Expect(tt.app.Spec.Secret).NotTo(Equal(previousSecret))
			} else {
				Expect(tt.app.Spec.Secret).To(Equal(previousSecret))
			}
		})
	}
}

func emptyAvailableSecrets(_ map[string]string, _ string) (string, bool) {
	return "", false
}
