// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package mapper

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	roverv1 "github.com/telekom/controlplane/rover/api/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

func TestLen_NilSlice_ReturnsZero(t *testing.T) {
	var slice *[]int

	result := Len(slice)

	assert.Equal(t, 0, result)
}

func TestLen_NonEmptySlice_ReturnsLength(t *testing.T) {
	slice := &[]int{1, 2, 3}

	result := Len(slice)

	assert.Equal(t, 3, result)
}

func TestToPtr_Value_ReturnsPointer(t *testing.T) {
	value := 42

	result := ToPtr(value)

	assert.NotNil(t, result)
	assert.Equal(t, 42, *result)
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
