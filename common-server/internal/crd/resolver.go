// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package crd

import (
	"context"
	"encoding/json"
	"regexp"

	"github.com/pkg/errors"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apiextensions "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/rest"
)

type CRD struct {
	GVR              schema.GroupVersionResource
	GVK              schema.GroupVersionKind
	SpecDefinition   string
	StatusDefinition string
}

var Instance Resolver

type Resolver interface {
	ResolveCrd(gvr schema.GroupVersionResource) (crd *CRD, err error)
	ResolveCrds(gvr schema.GroupVersionResource, limit int) (crds []*CRD, err error)
}

type resolver struct {
	client    apiextensions.Interface
	crdsCache *apiextensionsv1.CustomResourceDefinitionList
}

func NewResolver(client apiextensions.Interface) Resolver {
	return &resolver{
		client: client,
	}
}

func InitCrdResolver(cfg *rest.Config) {
	InitCrdResolverWithClient(apiextensions.NewForConfigOrDie(cfg))
}

func InitCrdResolverWithClient(apiextensionsClient apiextensions.Interface) {
	if Instance != nil {
		panic("CRD resolver already initialized")
	}
	Instance = NewResolver(apiextensionsClient)
}

func (r *resolver) ResolveCrd(gvr schema.GroupVersionResource) (crd *CRD, err error) {
	crds, err := r.ResolveCrds(gvr, 1)
	if err != nil {
		return nil, err
	}
	return crds[0], nil
}

func (r *resolver) ResolveCrds(gvr schema.GroupVersionResource, limit int) (crds []*CRD, err error) {
	if r.client == nil {
		return nil, errors.New("CRD resolver not initialized")
	}

	if r.crdsCache == nil {
		r.crdsCache, err = r.client.ApiextensionsV1().CustomResourceDefinitions().List(context.Background(), metav1.ListOptions{})
		if err != nil {
			return nil, errors.Wrap(err, "failed to list CRDs")
		}
	}

	resourceRE, err := regexp.Compile(gvr.Resource)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to compile regexp %s", gvr.Resource)
	}

	grpRE, err := regexp.Compile(gvr.Group)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to compile regexp %s", gvr.Group)
	}

	for _, item := range r.crdsCache.Items {
		for _, version := range item.Spec.Versions {
			if grpRE.MatchString(item.Spec.Group) && resourceRE.MatchString(item.Spec.Names.Plural) && version.Name == gvr.Version {
				crd := &CRD{
					GVR: schema.GroupVersionResource{
						Group:    item.Spec.Group,
						Version:  version.Name,
						Resource: item.Spec.Names.Plural,
					},
					GVK: schema.GroupVersionKind{
						Group:   item.Spec.Group,
						Version: version.Name,
						Kind:    item.Spec.Names.Kind,
					},
				}

				crd.SpecDefinition = "{}"
				crd.StatusDefinition = "{}"
				if version.Schema != nil {
					var b []byte
					spec, ok := version.Schema.OpenAPIV3Schema.Properties["spec"]
					if ok {
						b, _ := json.Marshal(spec)
						crd.SpecDefinition = string(b)
					}

					status, ok := version.Schema.OpenAPIV3Schema.Properties["status"]
					if ok {
						b, _ = json.Marshal(status)
						crd.StatusDefinition = string(b)
					}
				}

				crds = append(crds, crd)
				if limit > 0 && len(crds) >= limit {
					return
				}
			}
		}
	}

	if len(crds) == 0 {
		return nil, errors.Errorf("CRD for %s not found", gvr.String())
	}
	return crds, nil
}
