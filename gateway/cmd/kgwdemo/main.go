// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

// Command kgwdemo is a standalone local harness for the kgateway feature
// builder, analog of cmd/xdsdemo for the Envoy builder. It builds a single fake
// Route (no operator, no CRDs) and applies the resulting Gateway-API HTTPRoute
// plus kgateway Backend to a target cluster addressed by a kubeconfig. kgateway
// running in that cluster then translates the CRs into Envoy config.
//
// Unlike xdsdemo it does not run a server: it applies once and exits.
//
// ponytail: hardcoded fake route + upstream, basic routing only (no security
// features). Exercises the render -> apply path end to end.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/go-logr/logr"
	"github.com/go-logr/zapr"
	"go.uber.org/zap"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"

	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
	"github.com/telekom/controlplane/gateway/internal/features/kgateway"
	kongclient "github.com/telekom/controlplane/gateway/pkg/kong/client"
)

func main() {
	kubeconfig := flag.String("kubeconfig", envOr("KUBECONFIG", "config/samples/kgateway/kubeconfig.yaml"), "path to the target-cluster kubeconfig")
	gatewayName := flag.String("gateway", envOr("GATEWAY", "kgateway-poc"), "name of the kgateway Gateway the HTTPRoute binds to (parentRef)")
	namespace := flag.String("namespace", envOr("NAMESPACE", "kgateway-poc"), "namespace the HTTPRoute and Backend are created in")
	routeName := flag.String("route", envOr("ROUTE", "demo-route"), "name of the Route / emitted resources")
	upstreamURL := flag.String("upstream", envOr("UPSTREAM", "http://cosmoparrot.kgateway-poc.svc.cluster.local:8080/"), "upstream URL the route forwards to")
	hostnames := flag.String("hostnames", envOr("HOSTNAMES", "demo-route.local"), "comma-separated HTTPRoute hostnames (empty => match any)")
	paths := flag.String("paths", envOr("PATHS", "/get"), "comma-separated path prefixes (empty => \"/\")")
	flag.Parse()

	zl, _ := zap.NewDevelopment()
	log := zapr.NewLogger(zl)
	ctx := logr.NewContext(context.Background(), log)

	cl, err := newClient(*kubeconfig)
	if err != nil {
		log.Error(err, "building target-cluster client", "kubeconfig", *kubeconfig)
		os.Exit(1)
	}

	// Fake inputs: a named route in the target namespace, bound to the named
	// Gateway, forwarding matched traffic to the upstream.
	gateway := &gatewayv1.Gateway{}
	gateway.Name = *gatewayName
	gateway.Namespace = *namespace

	route := &gatewayv1.Route{}
	route.Name = *routeName
	route.Namespace = *namespace
	route.Spec.Type = gatewayv1.RouteTypePrimary
	fmt.Printf("ignoring hostnames %q", *hostnames)
	// route.Spec.Hostnames = splitCSV(*hostnames)
	route.Spec.Paths = splitCSV(*paths)
	route.Spec.Security.DefaultConsumers = []string{"eni--system--dev-luminary"}
	route.Spec.Security.TrustedIssuers = []string{"https://iris-distcp1-dataplane1.dev.dhei.telekom.de/auth/realms/rover"}

	builder := kgateway.NewFeatureBuilder(kgateway.NewClient(cl), route, nil, gateway)
	builder.EnableFeature(kgateway.InstanceAccessControlFeature)
	builder.SetUpstream(kongclient.NewUpstreamOrDie(*upstreamURL))

	if err := builder.Build(ctx); err != nil {
		log.Error(err, "building/applying kgateway resources")
		os.Exit(1)
	}
	log.Info("Applied kgateway resources",
		"gateway", *gatewayName, "namespace", *namespace, "route", *routeName, "upstream", *upstreamURL)
}

// newClient builds a controller-runtime client for the target cluster from the
// kubeconfig, with the Gateway-API + kgateway types registered.
func newClient(kubeconfig string) (client.Client, error) {
	restCfg, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		return nil, err
	}
	scheme := runtime.NewScheme()
	if err := kgateway.AddToScheme(scheme); err != nil {
		return nil, err
	}
	return client.New(restCfg, client.Options{Scheme: scheme})
}

// envOr returns the env var value if set (non-empty), else the fallback.
func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// splitCSV splits a comma-separated flag into trimmed, non-empty values.
// Returns nil for an empty string so the field stays unset.
func splitCSV(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
