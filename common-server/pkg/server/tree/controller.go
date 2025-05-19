package tree

import (
	"github.com/go-logr/logr"
	"github.com/gofiber/fiber/v2"
	"github.com/telekom/controlplane/common-server/pkg/server"
	"github.com/telekom/controlplane/common-server/pkg/server/middleware/security"
	"github.com/telekom/controlplane/common-server/pkg/store"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

var _ server.Controller = &ResourceTreeController{}

type ResourceTreeController struct {
	log   logr.Logger
	Store store.ObjectStore[*unstructured.Unstructured]
}

func NewResourceTreeController(store store.ObjectStore[*unstructured.Unstructured], log logr.Logger) *ResourceTreeController {
	return &ResourceTreeController{
		log:   log,
		Store: store,
	}
}

func (r *ResourceTreeController) Register(router fiber.Router, opts server.ControllerOpts) {
	checkAccess := security.ConfigureSecurity(router, opts.Security)

	router.Get("/:namespace/:name/tree", checkAccess, r.GetTree)
}

func (r *ResourceTreeController) GetTree(c *fiber.Ctx) error {
	namespace := c.Params("namespace")
	name := c.Params("name")

	tree, err := GetTree(c.UserContext(), r.Store, namespace, name, 10)
	if err != nil {
		return err
	}

	return c.JSON(tree)
}
