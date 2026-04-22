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
	applicationv1 "github.com/telekom/controlplane/application/api/v1"
	"github.com/telekom/controlplane/application/internal/secret"
	commontypes "github.com/telekom/controlplane/common/pkg/types"
	"github.com/telekom/controlplane/secret-manager/api"
	"github.com/telekom/controlplane/secret-manager/api/fake"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	fakeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestMutateSecret(t *testing.T) {
	RegisterTestingT(t)

	scheme := runtime.NewScheme()
	_ = adminv1.AddToScheme(scheme)

	zoneRef := commontypes.ObjectRef{Name: "test-zone", Namespace: "test-ns"}

	// Zone with SecretRotation feature enabled
	zoneWithRotation := &adminv1.Zone{
		ObjectMeta: metav1.ObjectMeta{Name: "test-zone", Namespace: "test-ns"},
		Status:     adminv1.ZoneStatus{Features: []adminv1.Feature{{Name: adminv1.FeatureSecretRotation, Enabled: true}}},
	}

	// Zone without SecretRotation feature
	zoneWithoutRotation := &adminv1.Zone{
		ObjectMeta: metav1.ObjectMeta{Name: "test-zone", Namespace: "test-ns"},
	}

	newReader := func(objs ...client.Object) client.Reader {
		return fakeclient.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).Build()
	}

	tests := []struct {
		name                  string
		app                   *applicationv1.Application
		env                   string
		reader                client.Reader
		mock                  func(t *testing.T) api.SecretManager
		mockFindSecretId      func(map[string]string, string) (string, bool)
		expectedError         bool
		expectedForbidden     bool
		expectedNewSecret     bool
		expectedRotatedSecret string
	}{
		{
			name: "Empty secret generates new UUID",
			app: &applicationv1.Application{
				Spec: applicationv1.ApplicationSpec{
					Team:   "my-team",
					Secret: "",
				},
			},
			env:    "dev",
			reader: newReader(),
			mock: func(t *testing.T) api.SecretManager {
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
			name: "Rotate keyword with graceful rotation stores rotated secret",
			app: &applicationv1.Application{
				Spec: applicationv1.ApplicationSpec{
					Team:   "my-team",
					Secret: "rotate",
					Zone:   zoneRef,
				},
				Status: applicationv1.ApplicationStatus{
					ClientSecret: "$<old-ref>",
				},
			},
			env:    "dev",
			reader: newReader(zoneWithRotation.DeepCopy()),
			mock: func(t *testing.T) api.SecretManager {
				m := fake.NewMockSecretManager(t)
				m.EXPECT().
					Get(mock.Anything, "$<old-ref>").
					Return("old-secret-value", nil)
				m.EXPECT().
					UpsertApplication(mock.Anything, "dev", "my-team", mock.Anything, mock.Anything, mock.Anything).
					Return(map[string]string{
						"clientSecret":        "$<new-ref>",
						"rotatedClientSecret": "$<rotated-ref>",
					}, nil)
				return m
			},
			expectedError:         false,
			expectedNewSecret:     true,
			expectedRotatedSecret: "$<rotated-ref>",
		},
		{
			name: "Rotate keyword without graceful rotation does not store rotated secret",
			app: &applicationv1.Application{
				Spec: applicationv1.ApplicationSpec{
					Team:   "my-team",
					Secret: "rotate",
					Zone:   zoneRef,
				},
				Status: applicationv1.ApplicationStatus{
					ClientSecret: "$<old-ref>",
				},
			},
			env:    "dev",
			reader: newReader(zoneWithoutRotation.DeepCopy()),
			mock: func(t *testing.T) api.SecretManager {
				m := fake.NewMockSecretManager(t)
				m.EXPECT().
					UpsertApplication(mock.Anything, "dev", "my-team", mock.Anything, mock.Anything).
					Return(map[string]string{
						"clientSecret": "$<new-ref>",
					}, nil)
				return m
			},
			expectedError:         false,
			expectedNewSecret:     true,
			expectedRotatedSecret: "",
		},
		{
			name: "Rotate without existing status.clientSecret only sets new secret",
			app: &applicationv1.Application{
				Spec: applicationv1.ApplicationSpec{
					Team:   "my-team",
					Secret: "rotate",
					Zone:   zoneRef,
				},
				Status: applicationv1.ApplicationStatus{
					ClientSecret: "",
				},
			},
			env:    "dev",
			reader: newReader(zoneWithRotation.DeepCopy()),
			mock: func(t *testing.T) api.SecretManager {
				m := fake.NewMockSecretManager(t)
				m.EXPECT().
					UpsertApplication(mock.Anything, "dev", "my-team", mock.Anything, mock.Anything).
					Return(map[string]string{
						"clientSecret": "$<new-ref>",
					}, nil)
				return m
			},
			mockFindSecretId: func(secrets map[string]string, name string) (string, bool) {
				v, ok := secrets[name]
				return v, ok
			},
			expectedError: true,
		},
		{
			name: "Rotate denied when rotation already in progress (heavy-clicker guard)",
			app: &applicationv1.Application{
				Spec: applicationv1.ApplicationSpec{
					Team:          "my-team",
					Secret:        "rotate",
					RotatedSecret: "$<existing-rotated-ref>",
					Zone:          zoneRef,
				},
				Status: applicationv1.ApplicationStatus{
					Conditions: []metav1.Condition{
						{
							Type:   secret.SecretRotationConditionType,
							Status: metav1.ConditionFalse,
							Reason: secret.SecretRotationReasonInProgress,
						},
					},
				},
			},
			env:    "dev",
			reader: newReader(zoneWithRotation.DeepCopy()),
			mock: func(t *testing.T) api.SecretManager {
				return nil
			},
			expectedError:     true,
			expectedForbidden: true,
		},
		{
			name: "Custom secret value is passed through",
			app: &applicationv1.Application{
				Spec: applicationv1.ApplicationSpec{
					Team:   "my-team",
					Secret: "my-custom-secret",
				},
			},
			env:    "dev",
			reader: newReader(),
			mock: func(t *testing.T) api.SecretManager {
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
			env:    "dev",
			reader: newReader(),
			mock: func(t *testing.T) api.SecretManager {
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
					Zone:   zoneRef,
				},
				Status: applicationv1.ApplicationStatus{
					ClientSecret: "$<old-ref>",
				},
			},
			env:    "dev",
			reader: newReader(zoneWithRotation.DeepCopy()),
			mock: func(t *testing.T) api.SecretManager {
				m := fake.NewMockSecretManager(t)
				m.EXPECT().
					Get(mock.Anything, "$<old-ref>").
					Return("old-secret-value", nil)
				m.EXPECT().
					UpsertApplication(mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
					Return(nil, fmt.Errorf("connection refused"))
				return m
			},
			expectedError:     true,
			expectedNewSecret: false,
		},
		{
			name: "Secret-manager Get error during rotation is propagated",
			app: &applicationv1.Application{
				Spec: applicationv1.ApplicationSpec{
					Team:   "my-team",
					Secret: "rotate",
					Zone:   zoneRef,
				},
				Status: applicationv1.ApplicationStatus{
					ClientSecret: "$<old-ref>",
				},
			},
			env:    "dev",
			reader: newReader(zoneWithRotation.DeepCopy()),
			mock: func(t *testing.T) api.SecretManager {
				m := fake.NewMockSecretManager(t)
				m.EXPECT().
					Get(mock.Anything, "$<old-ref>").
					Return("", fmt.Errorf("not found"))
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
					Zone:   zoneRef,
				},
				Status: applicationv1.ApplicationStatus{
					ClientSecret: "$<old-ref>",
				},
			},
			env:    "dev",
			reader: newReader(zoneWithRotation.DeepCopy()),
			mock: func(t *testing.T) api.SecretManager {
				m := fake.NewMockSecretManager(t)
				m.EXPECT().
					Get(mock.Anything, "$<old-ref>").
					Return("old-secret-value", nil)
				m.EXPECT().
					UpsertApplication(mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
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
			RegisterTestingT(t)
			t.Cleanup(func() {
				secret.FindSecretId = api.FindSecretId
			})

			previousSecret := tt.app.Spec.Secret
			sm := tt.mock(t)
			secret.GetSecretManager = func() api.SecretManager { return sm }
			if tt.mockFindSecretId != nil {
				secret.FindSecretId = tt.mockFindSecretId
			}
			err := MutateSecret(context.Background(), tt.env, tt.app, tt.reader)
			if tt.expectedError {
				Expect(err).To(HaveOccurred())
				if tt.expectedForbidden {
					Expect(err.Error()).To(ContainSubstring("forbidden"))
				}
			} else {
				Expect(err).NotTo(HaveOccurred())
			}
			if tt.expectedNewSecret {
				Expect(tt.app.Spec.Secret).NotTo(Equal(previousSecret))
			} else if !tt.expectedError {
				Expect(tt.app.Spec.Secret).To(Equal(previousSecret))
			}
			if tt.expectedRotatedSecret != "" {
				Expect(tt.app.Spec.RotatedSecret).To(Equal(tt.expectedRotatedSecret))
			}
		})
	}
}

func emptyAvailableSecrets(_ map[string]string, _ string) (string, bool) {
	return "", false
}
