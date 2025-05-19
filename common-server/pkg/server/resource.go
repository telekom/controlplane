package server

import (
	"fmt"

	"github.com/bytedance/sonic"
	"github.com/go-logr/logr"
	"github.com/gofiber/fiber/v2"
	"github.com/telekom/controlplane/common-server/pkg/problems"
	"github.com/telekom/controlplane/common-server/pkg/server/middleware/security"
	"github.com/telekom/controlplane/common-server/pkg/store"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func NewResourceController(store store.ObjectStore[*unstructured.Unstructured], log logr.Logger) *ResourceController {
	gvr, gvk := store.Info()
	ctrl := &ResourceController{
		log:   log.WithName(fmt.Sprintf("ResourceController[%s]", gvr.Resource)),
		Store: store,
		gvr:   gvr,
		gvk:   gvk,
	}
	return ctrl
}

type ResourceController struct {
	log       logr.Logger
	Store     store.ObjectStore[*unstructured.Unstructured]
	ApiPrefix string
	gvr       schema.GroupVersionResource
	gvk       schema.GroupVersionKind
}

func (r *ResourceController) SetXInfoHeaders(c *fiber.Ctx) {
	c.Set("X-ApiVersion", r.gvr.GroupVersion().String())
	c.Set("X-Resource", r.gvr.Resource)
}

func (r *ResourceController) Register(router fiber.Router, opts ControllerOpts) {
	r.ApiPrefix = opts.Prefix
	checkAccess := security.ConfigureSecurity(router, opts.Security)

	if opts.IsAllowed("GET") {
		router.Get("/:namespace/:name", checkAccess, r.Read)
		router.Get("/", checkAccess, r.List)
	}
	if opts.IsAllowed("PATCH") {
		router.Patch("/:namespace/:name", checkAccess, r.Patch)
	}
	if opts.IsAllowed("DELETE") {
		router.Delete("/:namespace/:name", checkAccess, r.Delete)
	}
	if opts.IsAllowed("POST") {
		router.Post("/", checkAccess, r.CreateOrUpdate)
	}
}

func (r *ResourceController) CreateOrUpdate(c *fiber.Ctx) error {
	ctx := c.UserContext()
	obj := &unstructured.Unstructured{}

	rawBody := c.Body()
	if len(rawBody) == 0 {
		return ReturnWithError(c, problems.BadRequest("invalid body"))
	}
	if err := obj.UnmarshalJSON(rawBody); err != nil {
		return ReturnWithError(c, problems.BadRequest("invalid body"))
	}

	if err := store.EqualGVK(r.gvk, obj.GroupVersionKind()); err != nil {
		return ReturnWithError(c, err)
	}

	err := r.Store.CreateOrReplace(ctx, obj)
	if err != nil {
		return ReturnWithError(c, err)
	}
	c.Location(fmt.Sprintf("%s/%s/%s", r.ApiPrefix, obj.GetNamespace(), obj.GetName()))
	r.SetXInfoHeaders(c)
	return Return(c, 201, obj)
}

func (r *ResourceController) Read(c *fiber.Ctx) error {
	namespace := c.Params("namespace")
	name := c.Params("name")
	ctx := c.UserContext()
	r.log.Info("Read", "namespace", namespace, "name", name)

	obj, err := r.Store.Get(ctx, namespace, name)
	if err != nil {
		return ReturnWithError(c, err)
	}
	r.SetXInfoHeaders(c)
	return Return(c, 200, obj)
}

func (r *ResourceController) Patch(c *fiber.Ctx) error {
	namespace := c.Params("namespace")
	name := c.Params("name")
	ctx := c.UserContext()

	patches := []store.Patch{}
	err := sonic.Unmarshal(c.Body(), &patches)
	if err != nil {
		return ReturnWithError(c, problems.BadRequest("invalid body"))
	}

	for i, patch := range patches {
		if !patch.Op.IsValid() {
			return ReturnWithError(c, problems.ValidationError(fmt.Sprintf("patches[%d].op", i), "invalid operation"))
		}
	}

	r.log.V(1).Info("Patch", "namespace", namespace, "name", name, "patches", patches)

	obj, err := r.Store.Patch(ctx, namespace, name, patches...)
	if err != nil {
		return ReturnWithError(c, err)
	}
	r.SetXInfoHeaders(c)
	return Return(c, 200, obj)
}

func (r *ResourceController) Delete(c *fiber.Ctx) error {
	namespace := c.Params("namespace")
	name := c.Params("name")
	ctx := c.UserContext()

	err := r.Store.Delete(ctx, namespace, name)
	if err != nil {
		return ReturnWithError(c, err)
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func (r *ResourceController) List(c *fiber.Ctx) error {
	ctx := c.UserContext()
	opts := store.NewListOpts()
	err := QueryParser(c, &opts)
	if err != nil {
		return ReturnWithError(c, problems.BadRequest(err.Error()))
	}

	store.EnforcePrefix(c.Locals("prefix"), &opts)

	list, err := r.Store.List(ctx, opts)
	if err != nil {
		return ReturnWithError(c, err)
	}
	opts.Cursor = list.Links.Self

	c.Set("X-Cursor-Self", list.Links.Self)
	c.Set("X-Cursor-Next", list.Links.Next)

	list.Links.Self = r.ApiPrefix + "?" + opts.UrlEncoded()
	if list.Links.Next != "" {
		opts.Cursor = list.Links.Next
		list.Links.Next = r.ApiPrefix + "?" + opts.UrlEncoded()
	}

	c.Set("X-Result-Count", fmt.Sprintf("%d", len(list.Items)))
	r.SetXInfoHeaders(c)
	return Return(c, 200, list)
}

func QueryParser(c *fiber.Ctx, opts *store.ListOpts) (err error) {
	c.Context().QueryArgs().VisitAll(func(key, b []byte) {
		value := string(b)
		switch string(key) {
		case "prefix":
			opts.Prefix = value
		case "cursor":
			opts.Cursor = value
		case "limit":
			opts.Limit = store.ParseLimit(value)
		case "filter":
			var filter store.Filter
			filter, err = store.ParseFilter(value)
			opts.Filters = append(opts.Filters, filter)
		case "sort":
			var sorter store.Sorter
			sorter, err = store.ParseSorter(value)
			opts.Sorters = append(opts.Sorters, sorter)
		}
		if err != nil {
			return
		}
	})
	return err
}
