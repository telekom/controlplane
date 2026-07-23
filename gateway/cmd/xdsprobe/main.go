// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

// Command xdsprobe opens the ADS stream against a running xDS server and prints
// the first response for one resource type, to confirm config is being served.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	discoveryv3 "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	resourcev3 "github.com/envoyproxy/go-control-plane/pkg/resource/v3"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func main() {
	addr := flag.String("addr", "localhost:18000", "xDS ADS gRPC address")
	node := flag.String("node", "poc-gateway-node", "xDS node id")
	flag.Parse()

	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()

	conn, err := grpc.NewClient(*addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		fmt.Fprintln(os.Stderr, "dial:", err)
		os.Exit(1)
	}
	defer conn.Close()

	stream, err := discoveryv3.NewAggregatedDiscoveryServiceClient(conn).StreamAggregatedResources(ctx)
	if err != nil {
		fmt.Fprintln(os.Stderr, "stream:", err)
		os.Exit(1)
	}

	if err := stream.Send(&discoveryv3.DiscoveryRequest{
		Node:    &corev3.Node{Id: *node},
		TypeUrl: resourcev3.ListenerType,
	}); err != nil {
		fmt.Fprintln(os.Stderr, "send:", err)
		os.Exit(1)
	}

	resp, err := stream.Recv()
	if err != nil {
		fmt.Fprintln(os.Stderr, "recv:", err)
		os.Exit(1)
	}

	fmt.Printf("OK: version=%q type=%s resources=%d\n", resp.VersionInfo, resp.TypeUrl, len(resp.Resources))
	for i, r := range resp.Resources {
		fmt.Printf("  [%d] %s\n", i, r.TypeUrl)
	}
}
