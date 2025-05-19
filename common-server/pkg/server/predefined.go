package server

import (
	"fmt"

	"golang.org/x/exp/slices"

	"github.com/bytedance/sonic"
	"github.com/go-logr/logr"
	"github.com/gofiber/fiber/v2"
	"github.com/telekom/controlplane/common-server/pkg/problems"
	"github.com/telekom/controlplane/common-server/pkg/server/middleware/security"
	"github.com/telekom/controlplane/common-server/pkg/server/template"
	"github.com/telekom/controlplane/common-server/pkg/store"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

var _ Controller = &PredefinedController{}

type PredefinedController struct {
	Name      string
	ApiPrefix string
	log       logr.Logger
	Store     store.ObjectStore[*unstructured.Unstructured]
	Filters   []store.Filter
	Patches   []store.Patch
}

func NewPredefinedController(name string, store store.ObjectStore[*unstructured.Unstructured], log logr.Logger) *PredefinedController {
	return &PredefinedController{
		Name:  name,
		log:   log.WithName(fmt.Sprintf("PredefinedController[%s]", name)),
		Store: store,
	}
}

func (r *PredefinedController) AddFilter(filter store.Filter) {
	r.Filters = append(r.Filters, filter)
}

func (r *PredefinedController) AddPatch(patch store.Patch) {
	r.Patches = append(r.Patches, patch)
}

func (r *PredefinedController) OnlyFilter() bool {
	return len(r.Patches) == 0 && len(r.Filters) > 0
}

func (r *PredefinedController) OnlyPatch() bool {
	return len(r.Patches) > 0 && len(r.Filters) == 0
}

func (r *PredefinedController) OnlyFindMatch() bool {
	return len(r.Patches) > 0 && len(r.Filters) > 0
}

func (r *PredefinedController) Register(router fiber.Router, opts ControllerOpts) {
	r.ApiPrefix = opts.Prefix
	prefix := "/" + r.Name
	checkAccess := security.ConfigureSecurity(router, opts.Security)

	if r.OnlyFilter() && opts.IsAllowed("GET") {
		r.log.V(1).Info("registering list handler", "prefix", prefix)
		router.Get(prefix, checkAccess, r.NewListHandler(r.Filters))
	}
	if r.OnlyPatch() && opts.IsAllowed("PATCH") {
		r.log.V(1).Info("registering patch handler", "prefix", prefix)
		router.Patch(prefix+"/:namespace/:name", checkAccess, r.NewPatchHandler(r.Patches))
	}
	if r.OnlyFindMatch() && opts.IsAllowed("PATCH") {
		r.log.V(1).Info("registering find match handler", "prefix", prefix)
		router.Patch(prefix, checkAccess, r.FindMatchHandler(r.Filters, r.Patches))
	}
}

func (r *PredefinedController) NewListHandler(ogFilters []store.Filter) func(c *fiber.Ctx) error {
	return func(c *fiber.Ctx) error {
		ctx := c.UserContext()

		opts := store.NewListOpts()

		// add any customer specified filters
		err := QueryParser(c, &opts)
		if err != nil {
			return ReturnWithError(c, problems.BadRequest(err.Error()))
		}

		// will replace any customer defined filter by the predefined one
		mergeWithPredefinedFilters(&opts, ogFilters)

		// finalize and add predefined filters
		if ok, err := r.finalizeFilters(c, opts.Filters); !ok {
			return err
		}

		store.EnforcePrefix(c.Locals("prefix"), &opts)
		list, err := r.Store.List(ctx, opts)
		if err != nil {
			return ReturnWithError(c, err)
		}
		opts.Cursor = list.Links.Self
		list.Links.Self = r.ApiPrefix + "?" + opts.UrlEncoded()
		if list.Links.Next != "" {
			opts.Cursor = list.Links.Next
			list.Links.Next = r.ApiPrefix + "?" + opts.UrlEncoded()
		}

		c.Set("X-Result-Count", fmt.Sprintf("%d", len(list.Items)))
		return Return(c, 200, list)
	}
}

func mergeWithPredefinedFilters(opts *store.ListOpts, predefinedFilters []store.Filter) {
	filtersToAppend := make([]store.Filter, len(predefinedFilters)+len(opts.Filters))
	copy(filtersToAppend, predefinedFilters)

	var isPredefinedFilter = func(filter store.Filter) bool {
		return slices.ContainsFunc(predefinedFilters, func(predFilter store.Filter) bool {
			return predFilter.Path == filter.Path
		})
	}
	var index = len(predefinedFilters)
	for _, optsFilter := range opts.Filters {
		if !isPredefinedFilter(optsFilter) {
			filtersToAppend[index] = optsFilter
			index++
		}
	}
	opts.Filters = filtersToAppend[:index]
}

func (r *PredefinedController) NewPatchHandler(ogPatches []store.Patch) func(c *fiber.Ctx) error {
	return func(c *fiber.Ctx) error {
		patches := make([]store.Patch, len(ogPatches))
		copy(patches, ogPatches)

		namespace := c.Params("namespace")
		name := c.Params("name")
		ctx := c.UserContext()

		if ok, err := r.finalizePatches(c, patches); !ok {
			return err
		}

		obj, err := r.Store.Patch(ctx, namespace, name, patches...)
		if err != nil {
			return ReturnWithError(c, err)
		}

		c.Location(fmt.Sprintf("%s/%s/%s", r.ApiPrefix, obj.GetNamespace(), obj.GetName()))
		return Return(c, 201, obj)
	}
}

func (r *PredefinedController) FindMatchHandler(ogFilters []store.Filter, ogPatches []store.Patch) func(c *fiber.Ctx) error {
	return func(c *fiber.Ctx) error {
		ctx := c.UserContext()
		opts := store.NewListOpts()

		// add any customer specified filters
		err := QueryParser(c, &opts)
		if err != nil {
			return ReturnWithError(c, problems.BadRequest(err.Error()))
		}

		// will replace any customer defined filter by the predefined one
		mergeWithPredefinedFilters(&opts, ogFilters)

		if ok, err := r.finalizeFilters(c, opts.Filters); !ok {
			return err
		}

		// handle patches
		patches := make([]store.Patch, len(ogPatches))
		copy(patches, ogPatches)

		if ok, err := r.finalizePatches(c, patches); !ok {
			return err
		}

		store.EnforcePrefix(c.Locals("prefix"), &opts)
		r.log.V(1).Info("finding match", "opts", opts)

		objList, err := r.Store.List(ctx, opts)
		if err != nil {
			return ReturnWithError(c, err)
		}

		if len(objList.Items) == 0 {
			return ReturnWithError(c, problems.NotFound("matching filters"))
		}
		if len(objList.Items) > 1 {
			return ReturnWithError(c, problems.BadRequest("found more than one object"))
		}

		foundObj := objList.Items[0]

		obj, err := r.Store.Patch(ctx, foundObj.GetNamespace(), foundObj.GetName(), patches...)
		if err != nil {
			return ReturnWithError(c, err)
		}

		c.Location(fmt.Sprintf("%s/%s/%s", r.ApiPrefix, obj.GetNamespace(), obj.GetName()))
		return Return(c, 201, obj)
	}
}

func (r *PredefinedController) finalizeFilters(c *fiber.Ctx, filters []store.Filter) (bool, error) {
	queries := c.Queries()

	for i, filter := range filters {
		if template.IsPlaceholder(filter.Value) {
			r.log.V(1).Info("found variable filter", "key", filter.Value)
			key := template.Trim(filter.Value)
			value, ok := queries[key]
			if ok {
				r.log.V(1).Info("resolved variable filter", "key", key, "value", value)
				filters[i].Value = value
			} else {
				return false, ReturnWithError(c, problems.BadRequest(fmt.Sprintf("missing value for %s", key)))
			}
		} else {
			r.log.V(1).Info("filter ok", "key", filter.Value)
		}
	}

	return true, nil
}

func (r *PredefinedController) finalizePatches(c *fiber.Ctx, patches []store.Patch) (bool, error) {
	lookUp := map[string]any{}
	rawBody := c.Body()
	if len(rawBody) > 0 {
		if err := sonic.Unmarshal(rawBody, &lookUp); err != nil {
			return false, ReturnWithError(c, problems.BadRequest("invalid body"))
		}
	}

	for i, patch := range patches {
		res, err := template.New(patch.Value).Apply(lookUp)
		if err != nil {
			return false, ReturnWithError(c, err)
		}
		patches[i].Value = res

	}

	return true, nil
}
