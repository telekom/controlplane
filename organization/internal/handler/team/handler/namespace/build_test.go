// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package namespace

import (
	"testing"

	"github.com/stretchr/testify/assert"
	organizationv1 "github.com/telekom/controlplane/organization/api/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestBuildNamespaceName(t *testing.T) {
	groupName := "this-is-a-group"
	teamName := "this-is-a-team"
	envName := "test-env"
	team := &organizationv1.Team{
		Spec: organizationv1.TeamSpec{
			Name:  teamName,
			Group: groupName,
		},
	}
	namespace := buildNamespaceName(envName, team)
	assert.Equal(t, "test-env--this-is-a-group--this-is-a-team", namespace)
}

func TestBuildNamespaceObj(t *testing.T) {
	groupName := "this-is-a-group"
	teamName := "this-is-a-team"
	envName := "test-env"
	team := &organizationv1.Team{
		Spec: organizationv1.TeamSpec{
			Name:  teamName,
			Group: groupName,
		},
	}
	expectedNsObject := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-env--this-is-a-group--this-is-a-team",
		},
	}
	namespaceObj := buildNamespaceObj(buildNamespaceName(envName, team))
	assert.Equal(t, expectedNsObject, namespaceObj)
}
