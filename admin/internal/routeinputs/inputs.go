// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package routeinputs

import (
	"fmt"
	"path"
	"slices"

	"k8s.io/apimachinery/pkg/util/validation/field"

	adminv1 "github.com/telekom/controlplane/admin/api/v1"
)

const (
	MaxHostnames   = 20
	MaxPaths       = 10
	ZoneHealthPath = "/zone-health"
)

// HostnamesAndBasePaths returns the sorted union of all presets using a gateway.
func HostnamesAndBasePaths(spec *adminv1.ZoneSpec, gatewayName string) (hostnames, basePaths []string) {
	for i := range spec.Presets {
		preset := &spec.Presets[i]
		if preset.GatewayRef != gatewayName {
			continue
		}
		for _, u := range preset.Urls {
			if !slices.Contains(hostnames, u.Hostname) {
				hostnames = append(hostnames, u.Hostname)
			}
			if !slices.Contains(basePaths, u.BasePath) {
				basePaths = append(basePaths, u.BasePath)
			}
		}
	}
	slices.Sort(hostnames)
	slices.Sort(basePaths)
	return hostnames, basePaths
}

func Paths(basePaths []string, downstreamPath string) []string {
	paths := make([]string, 0, len(basePaths))
	for _, basePath := range basePaths {
		joined := path.Join(basePath, downstreamPath)
		if !slices.Contains(paths, joined) {
			paths = append(paths, joined)
		}
	}
	slices.Sort(paths)
	return paths
}

// Validate checks the cardinality of generated health and identity routes.
// Their fixed suffixes contain no dot segments, so joining any of them yields
// the same number of distinct paths, independently of realm and visibility.
func Validate(spec *adminv1.ZoneSpec) field.ErrorList {
	var errs field.ErrorList
	for i, gateway := range spec.Gateways {
		hostnames, basePaths := HostnamesAndBasePaths(spec, gateway.Name)
		gatewayPath := field.NewPath("spec", "gateways").Index(i).Child("name")
		if len(hostnames) == 0 || len(hostnames) > MaxHostnames {
			errs = append(errs, field.Invalid(gatewayPath, gateway.Name,
				fmt.Sprintf("gateway has %d distinct preset hostnames, but a route requires 1 to %d", len(hostnames), MaxHostnames)))
		}
		paths := Paths(basePaths, ZoneHealthPath)
		if len(paths) == 0 || len(paths) > MaxPaths {
			errs = append(errs, field.Invalid(gatewayPath, gateway.Name,
				fmt.Sprintf("gateway has %d distinct joined preset paths, but a route requires 1 to %d", len(paths), MaxPaths)))
		}
	}
	return errs
}
