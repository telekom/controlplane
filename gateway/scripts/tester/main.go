package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/go-logr/logr"
	"github.com/go-logr/zapr"
	"github.com/telekom/controlplane/common/pkg/types"
	"github.com/telekom/controlplane/common/pkg/util/contextutil"
	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
	"github.com/telekom/controlplane/gateway/pkg/kong/client"
	"github.com/telekom/controlplane/gateway/pkg/kong/client/plugin"
	"github.com/telekom/controlplane/gateway/pkg/kongutil"
	"go.uber.org/zap"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func main() {
	log := zapr.NewLogger(zap.Must(zap.NewDevelopment()))
	ctx := context.Background()
	ctx = contextutil.WithEnv(ctx, "poc-rgu")
	ctx = logr.NewContext(ctx, log)

	gatewayCfg := kongutil.NewGatewayConfig(
		"https://stargate-admin-distcp1-dataplane1.dev.dhei.telekom.de/admin-api",
		"rover",
		"XJyMENQI7HbZheaH0p7AALEyeKqGiesX",
		"https://iris-distcp1-dataplane1.dev.dhei.telekom.de/auth/realms/rover",
	)

	httpClient, err := kongutil.NewClientFor(gatewayCfg)
	if err != nil {
		panic(err)
	}

	kongClient := client.NewKongClient(httpClient)

	route := &gatewayv1.Route{
		ObjectMeta: metav1.ObjectMeta{
			Name: "poc-rgu--eni-basicauth-v1",
		},
	}

	consumeRoute := &gatewayv1.ConsumeRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name: "poc-rgu--eni-basicauth-v1",
		},
		Spec: gatewayv1.ConsumeRouteSpec{
			Route: types.ObjectRef{
				Name: route.Name,
			},
			ConsumerName: "eni--hyper--rover-basicauth-example",
		},
	}

	routeRateLimiter := plugin.RateLimitPluginFromRoute(route)

	consumerRateLimiter := plugin.RateLimitPluginFromConsumeRoute(consumeRoute)

	routeRateLimiter.Config.Limits = plugin.Limits{
		Service: &plugin.LimitConfig{
			Minute: 10,
		},
	}

	consumerRateLimiter.Config.Limits = plugin.Limits{
		Consumer: &plugin.LimitConfig{
			Minute: 2,
		},
	}

	kongPlugin, err := kongClient.CreateOrReplacePlugin(ctx, routeRateLimiter)
	if err != nil {
		panic(err)
	}
	prettyPrint(kongPlugin)
	prettyPrint(route)

	kongPlugin, err = kongClient.CreateOrReplacePlugin(ctx, consumerRateLimiter)
	if err != nil {
		panic(err)
	}
	prettyPrint(kongPlugin)
	prettyPrint(consumeRoute)

}

func prettyPrint(o any) {
	jsonData, err := json.MarshalIndent(o, "", "  ")
	if err != nil {
		panic(err)
	}
	fmt.Println(string(jsonData))
}
