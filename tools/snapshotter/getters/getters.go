// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package getters

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
	kong "github.com/telekom/controlplane/gateway/pkg/kong/api"
	"github.com/telekom/controlplane/tools/snapshotter/util"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	KubeClient client.Client
	KongClient kong.ClientWithResponsesInterface
)

func GetRealm(ctx context.Context, environment, zone string) (*gatewayv1.Realm, error) {
	if environment == "" {
		return nil, fmt.Errorf("environment must be specified")
	}
	if zone == "" {
		return nil, fmt.Errorf("zone must be specified")
	}

	realm := &gatewayv1.Realm{
		ObjectMeta: metav1.ObjectMeta{
			Name:      environment,
			Namespace: fmt.Sprintf("%s--%s", environment, zone),
		},
	}

	err := KubeClient.Get(ctx, client.ObjectKeyFromObject(realm), realm)
	if err != nil {
		if client.IgnoreNotFound(err) != nil {
			return nil, errors.Wrapf(err, "failed to get realm %s", realm.Name)
		}
		return nil, fmt.Errorf("realm %s not found", realm.Name)
	}
	return realm, nil
}

func GetRealmGateway(ctx context.Context, realm *gatewayv1.Realm) (*gatewayv1.Gateway, error) {
	gateway := &gatewayv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Name:      realm.Spec.Gateway.Name,
			Namespace: realm.Spec.Gateway.Namespace,
		},
	}

	err := KubeClient.Get(ctx, client.ObjectKeyFromObject(gateway), gateway)
	if err != nil {
		if client.IgnoreNotFound(err) != nil {
			return nil, errors.Wrapf(err, "failed to get gateway %s in namespace %s", gateway.Name, gateway.Namespace)
		}
		return nil, fmt.Errorf("gateway %s not found", gateway.Name)
	}
	return gateway, nil
}

func ListRouteNames(ctx context.Context, environment, zone string, maxItems int) ([]string, error) {
	routes, err := KongClient.ListRouteWithResponse(ctx, &kong.ListRouteParams{
		Offset: nil,
		Size:   &maxItems,
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed to list routes")
	}
	util.MustBe2xx(routes, "Routes")
	routeNames := make([]string, 0, len(*routes.JSON200.Data))
	for _, route := range *routes.JSON200.Data {
		routeNames = append(routeNames, *route.Name)
	}

	return routeNames, nil
}
