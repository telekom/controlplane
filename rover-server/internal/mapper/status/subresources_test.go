// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package status

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	apiv1 "github.com/telekom/controlplane/api/api/v1"
	v1 "github.com/telekom/controlplane/rover/api/v1"
)

func TestGetAllSubResources_NilRover(t *testing.T) {
	// given
	ctx := context.Background()
	mockStore := new(MockObjectStore[*apiv1.ApiSubscription])
	var rover *v1.Rover = nil

	// when
	result, err := getAllSubResources(ctx, rover, mockStore)

	// then
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "rover resource is not processed and does not contain an application")
	mockStore.AssertExpectations(t)
}

func TestGetAllSubResources_NilApplication(t *testing.T) {
	// given
	ctx := context.Background()
	mockStore := new(MockObjectStore[*apiv1.ApiSubscription])
	rover := &v1.Rover{
		Status: v1.RoverStatus{
			// Application is nil
		},
	}

	// when
	result, err := getAllSubResources(ctx, rover, mockStore)

	// then
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "rover resource is not processed and does not contain an application")
	mockStore.AssertExpectations(t)
}
