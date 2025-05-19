// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package server

import (
	"github.com/gofiber/fiber/v2"
	"github.com/telekom/controlplane/common-server/pkg/problems"
)

var _ Controller = &ProbesController{}

var ErrServiceUnavailable = problems.Builder().Status(fiber.StatusServiceUnavailable).Title("Service Unavailable").Build()
var OkResponse = map[string]string{"status": "ok"}

type CheckerFunc func(c *fiber.Ctx) error

var nop = func(_ *fiber.Ctx) error {
	return nil
}

type SimpleChecker func() bool

var CustomCheck = func(check SimpleChecker) CheckerFunc {
	return func(c *fiber.Ctx) error {
		if check() {
			return nil
		}
		return ErrServiceUnavailable
	}
}

type ProbesController struct {
	ReadyChecks   []CheckerFunc
	HealthyChecks []CheckerFunc
}

func NewProbesController() *ProbesController {
	return &ProbesController{
		ReadyChecks:   []CheckerFunc{nop},
		HealthyChecks: []CheckerFunc{nop},
	}
}

func (h *ProbesController) AddReadyCheck(checker CheckerFunc) {
	h.ReadyChecks = append(h.ReadyChecks, checker)
}

func (h *ProbesController) AddHealthyCheck(checker CheckerFunc) {
	h.HealthyChecks = append(h.HealthyChecks, checker)
}

func (h *ProbesController) Register(router fiber.Router, opts ControllerOpts) {
	router.Get("/healthz", h.HealthyCheck)
	router.Get("/readyz", h.ReadyCheck)

}

func (h *ProbesController) ReadyCheck(c *fiber.Ctx) error {
	for _, check := range h.ReadyChecks {
		if err := check(c); err != nil {
			return ReturnWithError(c, err)
		}
	}
	return Return(c, fiber.StatusOK, OkResponse)
}

func (h *ProbesController) HealthyCheck(c *fiber.Ctx) error {
	for _, check := range h.HealthyChecks {
		if err := check(c); err != nil {
			return ReturnWithError(c, err)
		}
	}
	return Return(c, fiber.StatusOK, OkResponse)
}
