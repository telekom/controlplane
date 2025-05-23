// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package validator

import (
	"testing"

	"github.com/stretchr/testify/assert"
	organizationv1 "github.com/telekom/controlplane/organization/api/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestValidateTeamName(t *testing.T) {
	tests := []struct {
		name              string
		teamObj           *organizationv1.Team
		expectedError     bool
		expectedErrorType func(error) bool
	}{
		{
			name:              "Empty TeamName",
			teamObj:           &organizationv1.Team{},
			expectedError:     true,
			expectedErrorType: errors.IsInvalid,
		},
		{
			name: "Valid TeamName but missing group",
			teamObj: &organizationv1.Team{
				ObjectMeta: metav1.ObjectMeta{Name: "group--team"},
				Spec: organizationv1.TeamSpec{
					Name: "team",
				},
			},
			expectedError:     true,
			expectedErrorType: errors.IsInvalid,
		},
		{
			name: "Valid TeamName but missing team",
			teamObj: &organizationv1.Team{
				ObjectMeta: metav1.ObjectMeta{Name: "group--team"},
				Spec: organizationv1.TeamSpec{
					Group: "group",
				},
			},
			expectedError:     true,
			expectedErrorType: errors.IsInvalid,
		},
		{
			name: "Valid TeamName but team and group mixed up",
			teamObj: &organizationv1.Team{
				ObjectMeta: metav1.ObjectMeta{Name: "group--team"},
				Spec: organizationv1.TeamSpec{
					Name:  "group",
					Group: "team",
				},
			},
			expectedError:     true,
			expectedErrorType: errors.IsInvalid,
		},
		{
			name: "Valid TeamName",
			teamObj: &organizationv1.Team{
				ObjectMeta: metav1.ObjectMeta{Name: "group--team"},
				Spec: organizationv1.TeamSpec{
					Name:  "team",
					Group: "group",
				},
			},
			expectedError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateTeamName(tt.teamObj)
			if tt.expectedError {
				assert.NotNil(t, err, "expected error but got nil")
				assert.True(t, tt.expectedErrorType(err), "expected error type does not match")
			} else {
				assert.Nil(t, err, "expected no error but got one")
			}
		})
	}
}

func TestValidateAndGetEnv(t *testing.T) {
	tests := []struct {
		name              string
		teamObj           *organizationv1.Team
		expectedEnv       string
		expectedError     bool
		expectedErrorType func(error) bool
	}{
		{
			name:              "No labels",
			teamObj:           &organizationv1.Team{},
			expectedEnv:       "",
			expectedError:     true,
			expectedErrorType: errors.IsInvalid,
		},
		{
			name: "No env label",
			teamObj: &organizationv1.Team{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"foo": "bar"},
				},
			},
			expectedEnv:       "",
			expectedError:     true,
			expectedErrorType: errors.IsInvalid,
		},
		{
			name: "Wrong env label",
			teamObj: &organizationv1.Team{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"environment": "bar"},
				},
			},
			expectedEnv:       "",
			expectedError:     true,
			expectedErrorType: errors.IsInvalid,
		},
		{
			name: "right env label",
			teamObj: &organizationv1.Team{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"cp.ei.telekom.de/environment": "bar"},
				},
			},
			expectedEnv:   "bar",
			expectedError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env, err := ValidateAndGetEnv(tt.teamObj)
			if tt.expectedError {
				assert.NotNil(t, err, "expected error but got nil")
				assert.True(t, tt.expectedErrorType(err), "expected error type does not match")
			} else {
				assert.Nil(t, err, "expected no error but got one")
			}
			assert.Equal(t, tt.expectedEnv, env)
		})
	}
}
