// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"github.com/go-logr/logr"
	"github.com/gofiber/fiber/v2"
	"github.com/telekom/controlplane/common-server/pkg/server"
	"github.com/telekom/controlplane/common-server/pkg/server/middleware/security"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

type Item struct {
	Kind     string `json:"kind"`
	Resource string `json:"resource"`
}

type GroupedItems struct {
	ApiVersion string `json:"apiVersion"`
	Items      []Item `json:"items"`
}

type ConfigController struct {
	log     logr.Logger
	configs []GroupedItems
}

type StoreInfo interface {
	Info() (schema.GroupVersionResource, schema.GroupVersionKind)
}

func BuildConfigs(stores ...StoreInfo) []GroupedItems {
	tmp := make(map[string][]Item, 0)
	for _, store := range stores {
		gvr, gvk := store.Info()
		apiVersion := gvr.Group + "/" + gvr.Version
		if _, ok := tmp[apiVersion]; !ok {
			tmp[apiVersion] = make([]Item, 0)
		}
		tmp[apiVersion] = append(tmp[apiVersion], Item{
			Kind:     gvk.Kind,
			Resource: gvr.Resource,
		})
	}

	configs := make([]GroupedItems, 0)
	for apiVersion, resources := range tmp {
		configs = append(configs, GroupedItems{
			ApiVersion: apiVersion,
			Items:      resources,
		})
	}
	return configs
}

func NewConfigController(log logr.Logger, stores ...StoreInfo) *ConfigController {
	c := &ConfigController{
		log: log,
	}
	c.configs = BuildConfigs(stores...)
	return c
}

func (r *ConfigController) Register(router fiber.Router, opts server.ControllerOpts) {
	checkAccess := security.ConfigureSecurity(router, opts.Security)

	router.Get("/config", checkAccess, r.GetConfig)
}

func (r *ConfigController) GetConfig(c *fiber.Ctx) error {
	return c.JSON(r.configs)
}
