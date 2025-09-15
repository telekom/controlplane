// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/pkg/errors"
	"golang.org/x/oauth2/clientcredentials"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	kconfig "sigs.k8s.io/controller-runtime/pkg/client/config"

	applicationv1 "github.com/telekom/controlplane/application/api/v1"
	accesstoken "github.com/telekom/controlplane/common-server/pkg/client/token"
	"github.com/telekom/controlplane/common/pkg/util/labelutil"
	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
	secretsapi "github.com/telekom/controlplane/secret-manager/api"
)

var (
	// flags
	kubecontext string
	environment string
	team        string
	basePath    string
	application string
	secretsApi  secretsapi.SecretsApi
)

func init() {
	// Initialize flags, configurations, or any other setup needed for the application
	flag.StringVar(&kubecontext, "kubecontext", "", "Kubernetes context to use")
	flag.StringVar(&environment, "environment", "", "Environment to use")
	flag.StringVar(&environment, "env", "", "Environment to use (alias for --environment)")
	flag.StringVar(&team, "team", "", "Team name")
	flag.StringVar(&basePath, "basepath", "", "Base path for the API requests")
	flag.StringVar(&application, "application", "", "Application name")
	flag.StringVar(&application, "app", "", "Application name (alias for --application)")
}

func main() {
	ctx := context.Background()
	flag.Parse()

	secretsApi = secretsapi.NewSecrets(
		secretsapi.WithURL("https://localhost:9090/api"),
		secretsapi.WithAccessToken(accesstoken.NewStaticAccessToken(os.Getenv("SECRET_MANAGER_TOKEN"))),
		secretsapi.WithSkipTLSVerify(),
	)

	kubeCfg, err := kconfig.GetConfigWithContext(kubecontext)
	if err != nil {
		panic(err)
	}
	k8sClient, err := newClient(kubeCfg)
	if err != nil {
		panic(errors.Wrap(err, "failed to create Kubernetes client"))
	}

	teamNamespace := environment + "--" + team

	application := &applicationv1.Application{
		ObjectMeta: metav1.ObjectMeta{
			Name:      application,
			Namespace: teamNamespace,
		},
	}

	err = k8sClient.Get(ctx, client.ObjectKeyFromObject(application), application)
	if err != nil {
		panic(errors.Wrapf(err, "failed to get Application %s in namespace %s", application.Name, teamNamespace))
	}

	zoneName := application.Spec.Zone.Name
	realmName := environment // per convention, the realm name is the same as the environment name

	route := &gatewayv1.Route{
		ObjectMeta: metav1.ObjectMeta{
			Name:      realmName + "--" + labelutil.NormalizeValue(basePath),
			Namespace: environment + "--" + zoneName, // zone namespace
		},
	}

	err = k8sClient.Get(ctx, client.ObjectKeyFromObject(route), route)
	if err != nil {
		panic(errors.Wrapf(err, "failed to get Route %s in namespace %s", route.Name, teamNamespace))
	}

	clientId := application.Status.ClientId
	clientSecret := application.Status.ClientSecret
	tokenUrl := route.Spec.Downstreams[0].IssuerUrl + "/protocol/openid-connect/token"
	url := route.Spec.Downstreams[0].Url() + "/anything"

	makeRequest(ctx, url, tokenUrl, clientId, clientSecret)

}

func newClient(cfg *rest.Config) (client.Client, error) {
	scheme := runtime.NewScheme()
	err := gatewayv1.AddToScheme(scheme)
	if err != nil {
		return nil, errors.Wrap(err, "failed to add gateway scheme")
	}
	err = applicationv1.AddToScheme(scheme)
	if err != nil {
		return nil, errors.Wrap(err, "failed to add application scheme")
	}

	k8sClient, err := client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		return nil, errors.Wrap(err, "failed to create Kubernetes client")
	}
	return k8sClient, nil
}

func newHttpClient(ctx context.Context, tokenUrl, clientId, clientSecret string) (*http.Client, error) {
	clientSecret, err := secretsApi.Get(ctx, clientSecret)
	if err != nil {
		return nil, errors.Wrap(err, "failed to retrieve client secret from secret manager")
	}
	tokenCfg := clientcredentials.Config{
		ClientID:     clientId,
		ClientSecret: clientSecret,
		TokenURL:     tokenUrl,
	}

	httpClient := tokenCfg.Client(ctx)
	if httpClient == nil {
		return nil, errors.New("failed to create HTTP client")
	}
	return httpClient, nil
}

func makeRequest(ctx context.Context, url, tokenUrl, clientId, clientSecret string) {
	httpClient, err := newHttpClient(ctx, tokenUrl, clientId, clientSecret)
	if err != nil {
		panic(errors.Wrap(err, "failed to create HTTP client"))
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		panic(errors.Wrapf(err, "failed to create HTTP request to %s", url))
	}
	req.Header.Set("Accept", "application/json")

	Logf("Making request to %s with token URL %s\n", url, tokenUrl)

	resp, err := httpClient.Do(req)
	if err != nil {
		panic(errors.Wrapf(err, "failed to make HTTP request to %s", url))
	}
	defer resp.Body.Close()

	Logf("Response status: %s\n", resp.Status)
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		panic(errors.Wrapf(err, "failed to read response body from %s", url))
	}
	fmt.Fprint(os.Stdout, string(body))

	if resp.StatusCode >= 400 {
		os.Exit(1)
	}
}

func Logf(msg string, args ...any) {
	if len(args) > 0 {
		fmt.Fprintf(os.Stderr, msg, args...)
	} else {
		fmt.Println(msg)
	}
}
