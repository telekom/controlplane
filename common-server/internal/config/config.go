// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"context"
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	"github.com/telekom/controlplane/common-server/internal/crd"
	"github.com/telekom/controlplane/common-server/pkg/openapi"
	"github.com/telekom/controlplane/common-server/pkg/server"
	"github.com/telekom/controlplane/common-server/pkg/server/config"
	"github.com/telekom/controlplane/common-server/pkg/server/middleware/security"
	"github.com/telekom/controlplane/common-server/pkg/server/tree"
	"github.com/telekom/controlplane/common-server/pkg/store"
	"github.com/telekom/controlplane/common-server/pkg/store/inmemory"
	"github.com/telekom/controlplane/common-server/pkg/store/secrets"
	"gopkg.in/yaml.v3"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"

	"net/http"
	_ "net/http/pprof"
)

type OpenapiConfig struct {
	Title       string          `json:"name"`
	Description string          `json:"description"`
	Version     string          `json:"version"`
	Servers     []OpenapiServer `json:"servers"`
}

type OpenapiServer struct {
	URL         string `json:"url"`
	Description string `json:"description"`
}

type LMSConfig struct {
	Enabled  bool   `json:"enabled"`
	BasePath string `json:"basePath"`
}

type SecurityConfig struct {
	Enabled        bool `json:"enabled"`
	LMS            LMSConfig
	TrustedIssuers []string `yaml:"trustedIssuers" json:"trustedIssuers"`
	DefaultScope   string   `yaml:"defaultScope" json:"defaultScope"`
	ScopePrefix    string   `yaml:"scopePrefix" json:"scopePrefix"`
}

type ServerConfig struct {
	Address  string `json:"address"`
	BasePath string `json:"basepath"`

	AddGroupToPath bool               `yaml:"addGroupToPath" json:"addGroupToPath"`
	Resources      []ResourceConfig   `json:"resources"`
	Predefined     []PredefinedConfig `json:"predefined"`

	Openapi  OpenapiConfig  `json:"openapi"`
	Security SecurityConfig `json:"security"`

	Tree  TreeConfig  `json:"tree"`
	Pprof PProfConfig `json:"pprof"`
}

type StoreOpts struct {
	// DisableInformerCache if true, the informer will not use a local cache and will directly forward events to the event handler.
	DisableInformerCache bool `json:"disableInformerCache" yaml:"disableInformerCache"`
	// DatabaseFilepath is the filepath where the badger database will be stored. If empty, the database will be in-memory only.
	DatabaseFilepath string `json:"databaseFilepath" yaml:"databaseFilepath"`
	// OptimizeMemoryUsage if true, the database will be optimized for memory usage.
	OptimizeMemoryUsage bool `json:"optimizeMemoryUsage" yaml:"optimizeMemoryUsage"`
}

type PProfConfig struct {
	Enabled bool `json:"enabled"`
	Port    int  `json:"port"`
}

type TreeConfig struct {
	Enabled bool `json:"enabled"`
}

// The Allowed Http-Methods for this resource. If empty, all methods are allowed.
// Allowed aliases are "read-only" (GET, HEAD) and "read-change" (GET, HEAD, PATCH).
type Actions []string

func (a Actions) GetAllowedList() []string {
	if len(a) == 0 {
		// Will result in the default
		return nil
	}
	if slices.Contains(a, "read-only") {
		return []string{"HEAD", "GET"}
	}
	if slices.Contains(a, "read-change") {
		return []string{"HEAD", "GET", "PATCH"}
	}
	return a
}

type ResourceConfig struct {
	Id           string    `json:"id"`
	Group        string    `json:"group"`
	Version      string    `json:"version"`
	Resource     string    `json:"resource"`
	AllowedSorts []string  `yaml:"allowedSorts" json:"allowedSorts"`
	Owns         []string  `json:"owns"`
	References   []string  `json:"references"`
	Secrets      []string  `json:"secrets"`
	Actions      Actions   `json:"actions"`
	Store        StoreOpts `json:"store"`
}

type PredefinedConfig struct {
	Ref     string         `json:"ref"`
	Name    string         `json:"name"`
	Filters []store.Filter `json:"filters,omitempty"`
	Patches []store.Patch  `json:"patches,omitempty"`
}

func ReadConfig(filepath string) (*ServerConfig, error) {
	content, err := os.ReadFile(filepath) // #nosec G304
	if err != nil {
		return nil, errors.Wrap(err, "failed to read config file")
	}

	config := &ServerConfig{}
	err = yaml.Unmarshal(content, config)
	if err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal config file")
	}

	return config, nil
}

func (c *ServerConfig) BuildServer(ctx context.Context, dynamicClient dynamic.Interface, log logr.Logger) (*server.Server, error) {
	securityOpts := security.SecurityOpts{
		Enabled: c.Security.Enabled,
		Log:     log.WithName("security"),
		JWTOpts: []security.Option[*security.JWTOpts]{
			security.WithLmsCheck(c.Security.LMS.BasePath),
			security.WithTrustedIssuers(c.Security.TrustedIssuers),
		},
		BusinessContextOpts: []security.Option[*security.BusinessContextOpts]{
			security.WithLog(log.WithName("security")),
			security.WithDefaultScope(c.Security.DefaultScope),
			security.WithScopePrefix(c.Security.ScopePrefix),
		},
	}

	appCfg := server.NewAppConfig()
	appCfg.CtxLog = &log
	s := server.NewServerWithApp(server.NewAppWithConfig(appCfg))
	openapiBuilder := openapi.NewDocumentBuilder()
	openapiBuilder.NewInfo(c.Openapi.Title, c.Openapi.Description, c.Openapi.Version)
	for _, server := range c.Openapi.Servers {
		openapiBuilder.AddServer(server.URL, server.Description)
	}

	resurces := make(map[string]ResourceConfig, len(c.Resources))
	stores := make(map[string]store.ObjectStore[*unstructured.Unstructured], len(c.Resources))
	for _, resource := range c.Resources {
		gvr := schema.GroupVersionResource{
			Group:    resource.Group,
			Version:  resource.Version,
			Resource: resource.Resource,
		}
		crds, err := crd.Instance.ResolveCrds(gvr, -1)
		if err != nil {
			return nil, errors.Wrap(err, "failed to resolve crd")
		}

		for _, crd := range crds {
			storeOpts := inmemory.StoreOpts{
				Client:       dynamicClient,
				GVR:          crd.GVR,
				GVK:          crd.GVK,
				AllowedSorts: resource.AllowedSorts,
				Database: inmemory.DatabaseOpts{
					Filepath:     resource.Store.DatabaseFilepath,
					ReduceMemory: resource.Store.OptimizeMemoryUsage,
				},
				Informer: inmemory.InformerOpts{
					DisableCache: resource.Store.DisableInformerCache,
				},
			}

			resourceId := strings.ToLower(crd.GVR.Resource)
			if _, exists := stores[crd.GVR.Resource]; exists {
				log.V(1).Info("Store already exists", "id", resourceId)
				if resource.Id != "" {
					log.V(1).Info("Overwriting resource id", "id", resourceId)
					resourceId = resource.Id

				} else {
					if c.AddGroupToPath {
						resourceId = fmt.Sprintf("%s/%s.%s", crd.GVR.Group, crd.GVR.Version, crd.GVR.Resource)
					} else {
						log.Info("Duplicate resource id", "id", resourceId)
						resourceId = strings.ToLower(crd.GVR.Resource)
					}
				}
			} else {
				resourceId = strings.ToLower(crd.GVR.Resource)
			}
			log.V(1).Info("Creating store", "id", resourceId, "gvr", crd.GVR)
			resourceStore := inmemory.NewSortableOrDie[*unstructured.Unstructured](ctx, storeOpts)
			if len(resource.Secrets) > 0 {
				log.V(1).Info("Wrapping store with secret resolver", "id", resourceId, "secrets", resource.Secrets)
				resourceStore = secrets.WrapStore(resourceStore, resource.Secrets, secrets.NewDefaultSecretManagerResolver())
			}

			stores[resourceId] = resourceStore
			resurces[resourceId] = resource
		}
	}

	for id, store := range stores {
		gvr, _ := store.Info()
		opts := server.ControllerOpts{Prefix: c.BasePath + server.CalculatePrefix(gvr, c.AddGroupToPath), Security: securityOpts}
		opts.AllowedMethods = resurces[id].Actions.GetAllowedList()
		ctrl := server.NewResourceController(store, log)

		s.RegisterController(ctrl, opts)
		openapi.AddResourceController(openapiBuilder, ctrl, opts)
		log.Info("Registered controller", "prefix", opts.Prefix, "resource", id, "allowedMethods", opts.AllowedMethods)
	}

	for _, predefined := range c.Predefined {
		objectStore, ok := stores[predefined.Ref]
		if !ok {
			return nil, errors.Errorf("store with id %s not found", predefined.Ref)
		}

		ctrl := server.NewPredefinedController(predefined.Name, objectStore, log)
		for _, filter := range predefined.Filters {
			ctrl.AddFilter(filter)
		}
		for _, patch := range predefined.Patches {
			ctrl.AddPatch(patch)
		}
		gvr, _ := objectStore.Info()
		opts := server.ControllerOpts{Prefix: c.BasePath + server.CalculatePrefix(gvr, c.AddGroupToPath), Security: securityOpts}
		s.RegisterController(ctrl, opts)
		openapi.AddPredefinedController(openapiBuilder, ctrl, opts)
		log.Info("Registered predefined controller", "prefix", opts.Prefix, "resource", predefined.Ref, "filters", len(predefined.Filters), "patches", len(predefined.Patches))
	}

	if c.Tree.Enabled {
		for _, objectStore := range stores {
			gvr, _ := objectStore.Info()
			tree.LookupStores.AddStore(objectStore)
			ctrl := tree.NewResourceTreeController(objectStore, log)
			ctrlOpts := server.ControllerOpts{Prefix: c.BasePath + server.CalculatePrefix(gvr, c.AddGroupToPath), Security: securityOpts}
			s.RegisterController(ctrl, ctrlOpts)
		}
	}

	if c.Pprof.Enabled {
		go func() {
			addr := fmt.Sprintf(":%d", c.Pprof.Port)
			log.Info("Starting pprof server", "address", addr)
			err := http.ListenAndServe(addr, nil)
			if err != nil {
				log.Error(err, "pprof server failed", "address", addr)
			}
		}()
	}

	s.RegisterController(config.NewConfigController(log, storesToStoreInfos(stores)...), server.ControllerOpts{Prefix: c.BasePath})

	s.RegisterController(openapi.NewOpenAPIController(openapiBuilder.Build()), server.ControllerOpts{Prefix: c.BasePath})
	log.Info("Registered openapi controller", "prefix", c.BasePath)

	probesCtrl := server.NewProbesController()
	for _, objectStore := range stores {
		probesCtrl.AddReadyCheck(server.CustomCheck(objectStore.Ready))
	}
	s.RegisterController(probesCtrl, server.ControllerOpts{})

	return s, nil
}

func storesToStoreInfos(stores map[string]store.ObjectStore[*unstructured.Unstructured]) []config.StoreInfo {
	storeInfos := make([]config.StoreInfo, 0, len(stores))
	for _, objectStore := range stores {
		storeInfos = append(storeInfos, objectStore)
	}
	return storeInfos
}
