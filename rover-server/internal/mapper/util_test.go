// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package mapper

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/telekom/controlplane/common-server/pkg/server/middleware/security"
	roverv1 "github.com/telekom/controlplane/rover/api/v1"
)

func TestCopyFromTo_InvalidSource_ReturnsError(t *testing.T) {
	type Source struct {
		Field string
	}
	type Target struct {
		Field int
	}
	source := Source{Field: "invalid"}
	var target Target

	err := CopyFromTo(source, &target)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to unmarshal")
}

func TestCopyFromTo_ValidSource_CopiesData(t *testing.T) {
	type Source struct {
		Field string
	}
	type Target struct {
		Field string
	}
	source := Source{Field: "value"}
	var target Target

	err := CopyFromTo(source, &target)

	assert.NoError(t, err)
	assert.Equal(t, "value", target.Field)
}

func TestMakeResourceId_ValidNamespace_ReturnsResourceId(t *testing.T) {
	obj := &roverv1.Rover{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "env--group--team",
			Name:      "resource",
		},
	}

	result := MakeResourceId(obj)

	assert.Equal(t, "group--team--resource", result)
}

func TestMakeResourceId_InvalidNamespace_ReturnsResourceIdWithNamespace(t *testing.T) {
	obj := &roverv1.Rover{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "invalid-namespace",
			Name:      "resource",
		},
	}

	result := MakeResourceId(obj)

	assert.Equal(t, "invalid-namespace--resource", result)
}

func TestParseResourceId_InvalidContext_ReturnsError(t *testing.T) {
	ctx := context.Background()

	_, err := ParseResourceId(ctx, "namespace--resource")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Security context not found")
}

// --- New tests for ParseResourceId and ValidateResourceIdInfo ---

func securityContext(env string) context.Context {
	bCtx := &security.BusinessContext{
		Environment: env,
	}
	return security.ToContext(context.Background(), bCtx)
}

func TestParseResourceId_ValidResourceId(t *testing.T) {
	ctx := securityContext("prod")

	info, err := ParseResourceId(ctx, "group--team--my-resource")

	assert.NoError(t, err)
	assert.Equal(t, "group--team--my-resource", info.ResourceId)
	assert.Equal(t, "prod", info.Environment)
	assert.Equal(t, "group--team", info.Namespace)
	assert.Equal(t, "my-resource", info.Name)
}

func TestParseResourceId_InvalidFormat(t *testing.T) {
	ctx := securityContext("prod")

	_, err := ParseResourceId(ctx, "invalid-no-separator")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Invalid resourceId format")
}

func TestParseResourceId_NameTooShort(t *testing.T) {
	ctx := securityContext("prod")

	_, err := ParseResourceId(ctx, "group--team--x")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "ResourceId must be in format")
}

func TestParseResourceId_NameTooLong(t *testing.T) {
	ctx := securityContext("prod")
	longName := strings.Repeat("a", MaxNameLength+1)

	_, err := ParseResourceId(ctx, "group--team--"+longName)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "ResourceId must be in format")
}

func TestValidateResourceIdInfo_ValidName(t *testing.T) {
	info := ResourceIdInfo{
		ResourceId:  "group--team--my-resource",
		Environment: "prod",
		Namespace:   "group--team",
		Name:        "my-resource",
	}

	err := ValidateResourceIdInfo(info)

	assert.NoError(t, err)
}

func TestValidateResourceIdInfo_NameTooShort(t *testing.T) {
	info := ResourceIdInfo{
		Name: "x",
	}

	err := ValidateResourceIdInfo(info)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "ResourceId must be in format")
}

func TestValidateResourceIdInfo_NameTooLong(t *testing.T) {
	info := ResourceIdInfo{
		Name: strings.Repeat("a", MaxNameLength+1),
	}

	err := ValidateResourceIdInfo(info)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "ResourceId must be in format")
}

func TestValidateResourceIdInfo_ExactMinLength(t *testing.T) {
	info := ResourceIdInfo{Name: "ab"}

	err := ValidateResourceIdInfo(info)

	assert.NoError(t, err)
}

func TestValidateResourceIdInfo_ExactMaxLength(t *testing.T) {
	info := ResourceIdInfo{Name: strings.Repeat("a", MaxNameLength)}

	err := ValidateResourceIdInfo(info)

	assert.NoError(t, err)
}
