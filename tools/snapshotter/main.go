// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/goccy/go-yaml"
	"github.com/joho/godotenv"
	"github.com/pkg/errors"
	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
	kong "github.com/telekom/controlplane/gateway/pkg/kong/api"
	"github.com/telekom/controlplane/gateway/pkg/kongutil"
	secrets "github.com/telekom/controlplane/secret-manager/api"
	"github.com/telekom/controlplane/secret-manager/api/accesstoken"
	"github.com/telekom/controlplane/tools/snapshotter/getters"
	"github.com/telekom/controlplane/tools/snapshotter/state"
	"github.com/telekom/controlplane/tools/snapshotter/util"
	authv1 "k8s.io/api/authentication/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	kconfig "sigs.k8s.io/controller-runtime/pkg/client/config"

	"github.com/gkampitakis/go-diff/diffmatchpatch"
)

var (
	ctx = context.Background()

	// flags
	kubecontext string
	environment string
	zone        string
	routeName   string
	clean       bool
	outputDir   string
	fromEnv     bool
	allRoutes   bool
	failFast    bool
	parallel    bool

	waitGroup               sync.WaitGroup
	kubeCfg                 *rest.Config
	secretsApi              secrets.SecretsApi
	serviceAccountNamespace = "gateway-system"
	serviceAccountName      = "gateway-controller-manager"

	obfuscationTargets = []state.ObfuscationTarget{
		{
			Pattern: `[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`,
			Replace: `00000000-0000-0000-0000-000000000000`,
		}, {
			Pattern: `[0-9]{10}`,
			Replace: `0`,
		},
		{
			Path:    "route.tags",
			Replace: "[]",
		},
		{
			Path:    "service.tags",
			Replace: "[]",
		},
		{
			Path:    "plugins[].tags",
			Replace: "[]",
		},
	}

	base64ContentPatterns = []string{
		`jumper_config:([A-Za-z0-9=]+)`,
	}

	diffDetected = false // used to indicate if a diff was detected in the route state
	maxRoutes    = 1000
)

func init() {
	flag.StringVar(&kubecontext, "kubecontext", "", "Kubernetes context to use")
	flag.StringVar(&environment, "env", "", "Environment to use")
	flag.StringVar(&zone, "zone", "", "Zone to use")
	flag.StringVar(&routeName, "route", "", "Route to use")
	flag.BoolVar(&clean, "clean", false, "Clean output directory before writing snapshots")
	flag.StringVar(&outputDir, "output-dir", "snapshots", "Output directory for snapshots")
	flag.BoolVar(&fromEnv, "from-env", false, "Use environment variables to configure gateway client")
	flag.BoolVar(&allRoutes, "all-routes", false, "Collect state of all routes in the gateway instead of a single route")
	flag.BoolVar(&failFast, "fail-fast", false, "Fail fast on first difference found")
	flag.BoolVar(&parallel, "parallel", false, "Run snapshotting in parallel")
}

func setupSecretManager(ctx context.Context) error {
	clientset := kubernetes.NewForConfigOrDie(kubeCfg)
	tokenReq := &authv1.TokenRequest{}
	tokenRes, err := clientset.CoreV1().ServiceAccounts(serviceAccountNamespace).CreateToken(ctx, serviceAccountName, tokenReq, metav1.CreateOptions{})
	if err != nil {
		return errors.Wrap(err, "failed to create token for secret manager")
	}

	secretsApi = secrets.NewSecrets(
		secrets.WithURL("https://localhost:8443/api"), // kubectl -n secret-manager-system port-forward svc/secret-manager 8443:443
		secrets.WithAccessToken(accesstoken.NewStaticAccessToken(tokenRes.Status.Token)),
	)

	return nil
}

func newClient(cfg *rest.Config) (client.Client, error) {
	scheme := runtime.NewScheme()
	util.Must(gatewayv1.AddToScheme(scheme))

	k8sClient, err := client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		return nil, errors.Wrap(err, "failed to create Kubernetes client")
	}
	return k8sClient, nil
}

func automaticSetup(ctx context.Context) (kong.ClientWithResponsesInterface, error) {
	err := setupSecretManager(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to setup secret manager")
	}

	realm, err := getters.GetRealm(ctx, environment, zone)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get realm")
	}
	fmt.Fprintf(os.Stderr, "Using realm: %s/%s\n", realm.Namespace, realm.Name)

	gateway, err := getters.GetRealmGateway(ctx, realm)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get gateway for realm")
	}
	fmt.Fprintf(os.Stderr, "Using gateway: %s/%s\n", gateway.Namespace, gateway.Name)

	secretId, isSecret := secrets.FromRef(gateway.Spec.Admin.ClientSecret)
	if isSecret {
		gateway.Spec.Admin.ClientSecret, err = secretsApi.Get(ctx, secretId)
		if err != nil {
			return nil, errors.Wrap(err, "failed to get gateway admin client secret")
		}
	}

	kongClient, err := kongutil.NewClientFor(gateway)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create Kong client for gateway")
	}

	return kongClient, nil
}

func setupFromEnv() (kong.ClientWithResponsesInterface, error) {
	gwCfg := kongutil.NewGatewayConfig(
		util.EnvOrFail("GATEWAY_ADMIN_URL"),
		util.EnvOrFail("GATEWAY_ADMIN_CLIENT_ID"),
		util.EnvOrFail("GATEWAY_ADMIN_CLIENT_SECRET"),
		util.EnvOrFail("GATEWAY_ADMIN_ISSUER"),
	)

	kongClient, err := kongutil.NewClientFor(gwCfg)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create Kong client for gateway")
	}

	return kongClient, nil
}

func collectStateOfRoute(ctx context.Context, state *state.RouteState) error {
	routeRes, err := getters.KongClient.GetRouteWithResponse(ctx, state.RouteName)
	if err != nil {
		return errors.Wrap(err, "failed to get route")
	}
	util.MustBe2xx(routeRes, "Route")
	state.Route = routeRes.JSON200

	serviceRes, err := getters.KongClient.GetServiceWithResponse(ctx, *routeRes.JSON200.Service.Id)
	if err != nil {
		return errors.Wrap(err, "failed to get service for route")
	}
	util.MustBe2xx(serviceRes, "Service")
	state.Service = serviceRes.JSON200

	plugins, err := getters.KongClient.ListPluginsForRouteWithResponse(ctx, state.RouteName, &kong.ListPluginsForRouteParams{})
	if err != nil {
		return errors.Wrap(err, "failed to list plugins for route")
	}
	util.MustBe2xx(plugins, "Plugins for Route")
	state.Plugins = *plugins.JSON200.Data

	return nil
}

func main() {
	var err error
	flag.Parse()

	kubeCfg, err = kconfig.GetConfigWithContext(kubecontext)
	if err != nil {
		panic(err)
	}

	getters.KubeClient, err = newClient(kubeCfg)
	if err != nil {
		panic(errors.Wrap(err, "failed to create Kubernetes client"))
	}

	godotenv.Load() //nolint:errcheck

	zone = util.IfEmptyLoadEnvOrFail(zone, "GATEWAY_ZONE")
	environment = util.IfEmptyLoadEnvOrFail(environment, "GATEWAY_ENV")
	routeName = util.IfEmptyLoadEnv(routeName, "GATEWAY_ROUTE")
	if routeName == "" && !allRoutes {
		fmt.Fprintf(os.Stderr, "No route specified, use -route or GATEWAY_ROUTE environment variable to specify a route.\n")
		os.Exit(1)
	}

	if fromEnv {
		getters.KongClient, err = setupFromEnv()
		if err != nil {
			panic(errors.Wrap(err, "failed to setup Kong client from environment variables"))
		}
	} else {
		getters.KongClient, err = automaticSetup(ctx)
		if err != nil {
			panic(errors.Wrap(err, "failed to setup Kong client automatically"))
		}
	}

	// setup directories
	outputDir = filepath.Join(outputDir, environment, zone)
	if clean {
		err = os.RemoveAll(outputDir)
		if err != nil {
			panic(errors.Wrap(err, "failed to clean output directory"))
		}
	}
	err = os.MkdirAll(outputDir, 0755)
	if err != nil {
		panic(errors.Wrap(err, "failed to create output directory"))
	}

	if allRoutes {
		routeNames, err := getters.ListRouteNames(ctx, environment, zone, maxRoutes)
		if err != nil {
			panic(errors.Wrap(err, "failed to list routes"))
		}

		for _, routeName := range routeNames {
			if parallel {
				waitGroup.Add(1)
				go forRoute(routeName)
			} else {
				forRoute(routeName)
			}
		}
		if parallel {
			waitGroup.Wait()
		}

	} else {
		forRoute(routeName)
	}

	if diffDetected {
		os.Exit(1)
	}
}

func forRoute(routeName string) {
	if parallel {
		defer waitGroup.Done()
	}

	// collect route state
	currentState := &state.RouteState{
		Environment: environment,
		Zone:        zone,
		RouteName:   routeName,
	}

	err := collectStateOfRoute(ctx, currentState)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to collect state of route: %v\n", err)
		return
	}

	filename := fmt.Sprintf("%s-route-state.yaml", routeName)
	filepath := filepath.Join(outputDir, filename)

	// decode base64 content in plugins
	err = state.DecodeBase64Content(currentState, base64ContentPatterns...)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to decode base64 content in route state: %v\n", err)
		return
	}

	// setup initial snapshot
	if !util.FileExists(filepath) || clean {
		fmt.Fprintf(os.Stderr, "Creating initial snapshot for route %s/%s/%s\n", environment, zone, routeName)
		util.Must(currentState.Write(filepath))
		return
	}

	// compare current state with snapshot
	snapshottedStateBytes, err := os.ReadFile(filepath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to read snapshot file %s: %v\n", filepath, err)
		return
	}
	snapshottedState := &state.RouteState{}
	err = yaml.Unmarshal(snapshottedStateBytes, snapshottedState)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to unmarshal snapshotted route state from %s: %v\n", filepath, err)
		return
	}

	// obfuscate sensitive or dynamic data
	err = state.Obfuscate(currentState, obfuscationTargets...)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to obfuscate route state: %v\n", err)
		return
	}

	err = state.Obfuscate(snapshottedState, obfuscationTargets...)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to obfuscate snapshotted route state: %v\n", err)
		return
	}

	dmp := diffmatchpatch.New()
	diffs := dmp.DiffMain(snapshottedState.String(), currentState.String(), false)
	diffs = dmp.DiffCleanupSemantic(diffs)
	if len(diffs) <= 1 {
		fmt.Fprintf(os.Stderr, "✅ State of route is unchanged, no diff to show.\n")
		return
	} else {
		fmt.Fprintf(os.Stderr, "⚠️  State of route has changed, showing diff:\n")
	}
	fmt.Println(dmp.DiffPrettyText(diffs))
	if failFast {
		os.Exit(1)
	}
	diffDetected = true
}
