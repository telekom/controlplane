// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/log"

	"github.com/telekom/controlplane/rover-server/internal/api"
	"github.com/telekom/controlplane/rover-server/internal/server"
)

var _ server.EventSpecificationController = &EventSpecificationController{}

type EventSpecificationController struct{}

func NewEventSpecificationController() *EventSpecificationController {
	return &EventSpecificationController{}
}

// TODO EventSpecificationController: Implement the EventSpecificationController interface.
// Currently this is not in focus of the development, but it is already included in the openapi specification.

// Create implements server.EventSpecificationController.
func (e *EventSpecificationController) Create(ctx context.Context, req api.EventSpecificationCreateRequest) (api.EventSpecificationResponse, error) {
	log.Infof("EventSpecification: Create not implemented. EventSpecification is: %+v", req)
	return api.EventSpecificationResponse{},
		fiber.NewError(fiber.StatusNotImplemented, "Create not implemented")
}

// Delete implements server.EventSpecificationController.
func (e *EventSpecificationController) Delete(ctx context.Context, resourceId string) error {
	log.Infof("EventSpecification: Delete not implemented. ResourceId is: %s.", resourceId)
	return fiber.NewError(fiber.StatusNotImplemented, "Delete not implemented")
}

// Get implements server.EventSpecificationController.
func (e *EventSpecificationController) Get(ctx context.Context, resourceId string) (api.EventSpecificationResponse, error) {
	log.Infof("EventSpecification: Get not implemented. ResourceId is: %s.", resourceId)
	return api.EventSpecificationResponse{},
		fiber.NewError(fiber.StatusNotImplemented, "Get not implemented")
}

// GetAll implements server.EventSpecificationController.
func (e *EventSpecificationController) GetAll(ctx context.Context, params api.GetAllEventSpecificationsParams) (*api.EventSpecificationListResponse, error) {
	log.Info("EventSpecification: GetAll not implemented")
	return nil, fiber.NewError(fiber.StatusNotImplemented, "GetAll not implemented")
}

// GetStatus implements server.EventSpecificationController.
func (e *EventSpecificationController) GetStatus(ctx context.Context, resourceId string) (api.ResourceStatusResponse, error) {
	log.Infof("EventSpecification: GetStatus not implemented. ResourceId is: %s.", resourceId)
	return api.ResourceStatusResponse{},
		fiber.NewError(fiber.StatusNotImplemented, "GetStatus not implemented")

}

// Update implements server.EventSpecificationController.
func (e *EventSpecificationController) Update(ctx context.Context, resourceId string, req api.EventSpecification) (api.EventSpecificationResponse, error) {
	log.Infof("EventSpecification: Update not implemented. ResourceId is: %s. EventSpecification is: %+v", resourceId, req)
	return api.EventSpecificationResponse{},
		fiber.NewError(fiber.StatusNotImplemented, "Update not implemented")
}
