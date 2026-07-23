// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package envoy

import (
	"sort"
	"strings"

	routev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	matcherv3 "github.com/envoyproxy/go-control-plane/envoy/type/matcher/v3"
)

// routeDomains maps the route's hostnames to VirtualHost.domains (RT-02). With
// no hostnames configured the vhost matches any Host/:authority via "*",
// mirroring the Kong path leaving Route.Hosts unset.
func routeDomains(hostnames []string) []string {
	if len(hostnames) == 0 {
		return []string{"*"}
	}
	domains := append([]string(nil), hostnames...)
	sort.Strings(domains)
	return domains
}

// routeEntries emits one Route per configured path prefix, all pointing at the
// same cluster (RT-01). With no paths configured a single "/" prefix matches
// all requests, mirroring the Kong path leaving Route.Paths unset. When the
// upstream carries a non-trivial base path it is prepended to the forwarded
// path via regex_rewrite (RV-04).
func routeEntries(clusterName string, paths []string, upstreamPath string) []*routev3.Route {
	if len(paths) == 0 {
		paths = []string{"/"}
	} else {
		paths = append([]string(nil), paths...)
		sort.Strings(paths)
	}
	rewrite := basePathRewrite(upstreamPath)

	routes := make([]*routev3.Route, 0, len(paths))
	for _, p := range paths {
		action := &routev3.RouteAction{
			ClusterSpecifier: &routev3.RouteAction_Cluster{Cluster: clusterName},
		}
		if rewrite != nil {
			action.RegexRewrite = rewrite
		}
		routes = append(routes, &routev3.Route{
			Match:  &routev3.RouteMatch{PathSpecifier: &routev3.RouteMatch_Prefix{Prefix: p}},
			Action: &routev3.Route_Route{Route: action},
		})
	}
	return routes
}

// basePathRewrite prepends the upstream base path onto the request path,
// keeping the original path intact (RV-04). prefix_rewrite only *replaces* the
// matched prefix, so a prepend needs regex_rewrite: "^/" -> "/base/" turns
// "/foo" into "/base/foo".
//
// Identity guard: an empty or "/" base path is a no-op, so no rewrite is
// emitted and the original path is forwarded unchanged.
func basePathRewrite(upstreamPath string) *matcherv3.RegexMatchAndSubstitute {
	base := strings.Trim(upstreamPath, "/")
	if base == "" {
		return nil
	}
	return &matcherv3.RegexMatchAndSubstitute{
		Pattern: &matcherv3.RegexMatcher{
			EngineType: &matcherv3.RegexMatcher_GoogleRe2{
				GoogleRe2: &matcherv3.RegexMatcher_GoogleRE2{},
			},
			Regex: "^/",
		},
		Substitution: "/" + base + "/",
	}
}
