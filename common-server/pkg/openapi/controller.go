package openapi

import (
	"github.com/gofiber/fiber/v2"
	"github.com/telekom/controlplane/common-server/pkg/server"
)

var _ server.Controller = &OpenAPIController{}

type OpenAPIController struct {
	Document *Document
}

func NewOpenAPIController(doc *Document) *OpenAPIController {
	return &OpenAPIController{
		Document: doc,
	}
}

func (r *OpenAPIController) Register(router fiber.Router, opts server.ControllerOpts) {
	router.Get("/openapi.json", r.GetOpenAPI)
}

func (r *OpenAPIController) GetOpenAPI(c *fiber.Ctx) error {
	return c.JSON(r.Document)
}
