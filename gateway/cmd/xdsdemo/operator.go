// SPDX-FileCopyrightText: 2026 Deutsche Telekom AG
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	ctrlzap "sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	commonconfig "github.com/telekom/controlplane/common/pkg/config"
	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
	"github.com/telekom/controlplane/gateway/internal/controller"
	"github.com/telekom/controlplane/gateway/internal/features/envoy"
	xdsapi "github.com/telekom/controlplane/gateway/internal/xds/api/v1"
	"github.com/telekom/controlplane/gateway/internal/xds/publication"
)

const (
	demoNamespace   = "xdsdemo"
	demoGatewayName = "demo-gateway"
	demoEnvironment = "demo"
	shutdownTimeout = 10 * time.Second
)

type operatorState struct {
	client      client.Client
	publisher   *publication.Publisher
	status      xdsapi.StatusServiceClient
	targetID    string
	ready       atomic.Bool
	targetReady atomic.Bool
	bundleMu    sync.RWMutex
	lastBundle  *xdsapi.Bundle
	management  string
}

func runOperator(args []string) error { //nolint:gocyclo // Demo wiring intentionally keeps one process lifecycle together.
	flags := flag.NewFlagSet("operator", flag.ContinueOnError)
	httpAddress := flags.String("http-address", ":18081", "control API listen address")
	managementAddress := flags.String("management-address", "management:18000", "management-server gRPC address")
	targetFile := flags.String("target-file", "/state/target-id", "target handoff file")
	assets := flags.String("envtest-assets", "/envtest", "envtest binary directory")
	crds := flags.String("crds", "/crds", "Gateway CRD directory")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if err := os.Remove(*targetFile); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("removing stale target file: %w", err)
	}

	ctrl.SetLogger(ctrlzap.New(ctrlzap.UseDevMode(true)))
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	scheme := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		return fmt.Errorf("adding Kubernetes scheme: %w", err)
	}
	if err := gatewayv1.AddToScheme(scheme); err != nil {
		return fmt.Errorf("adding Gateway scheme: %w", err)
	}
	testEnvironment := &envtest.Environment{
		BinaryAssetsDirectory: *assets,
		CRDDirectoryPaths:     []string{*crds},
		ErrorIfCRDPathMissing: true,
	}
	config, err := testEnvironment.Start()
	if err != nil {
		return fmt.Errorf("starting envtest: %w", err)
	}
	defer func() {
		if stopErr := testEnvironment.Stop(); stopErr != nil {
			ctrl.Log.Error(stopErr, "stopping envtest")
		}
	}()

	directClient, err := client.New(config, client.Options{Scheme: scheme})
	if err != nil {
		return fmt.Errorf("creating envtest client: %w", err)
	}
	if createErr := createDemoGateway(ctx, directClient); createErr != nil {
		return createErr
	}
	gateway := &gatewayv1.Gateway{}
	if getErr := directClient.Get(ctx, client.ObjectKey{Namespace: demoNamespace, Name: demoGatewayName}, gateway); getErr != nil {
		return fmt.Errorf("reading demo Gateway: %w", getErr)
	}
	targetID := fmt.Sprintf("%s/%s/%s/%s", demoEnvironment, demoNamespace, demoGatewayName, gateway.UID)
	if writeErr := writeTarget(*targetFile, targetID); writeErr != nil {
		return writeErr
	}

	connection, err := grpc.NewClient(*managementAddress,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithConnectParams(grpc.ConnectParams{MinConnectTimeout: time.Second}))
	if err != nil {
		return fmt.Errorf("creating management client: %w", err)
	}
	defer func() {
		if closeErr := connection.Close(); closeErr != nil {
			ctrl.Log.Error(closeErr, "closing management connection")
		}
	}()
	state := &operatorState{
		client: directClient, publisher: publication.New(xdsapi.NewPublicationServiceClient(connection)),
		status: xdsapi.NewStatusServiceClient(connection), targetID: targetID, management: *managementAddress,
	}
	state.targetReady.Store(true)

	httpServer := &http.Server{Addr: *httpAddress, Handler: state.routes(), ReadHeaderTimeout: 5 * time.Second}
	httpErrors := make(chan error, 1)
	go func() { httpErrors <- httpServer.ListenAndServe() }()

	if waitErr := waitForTCP(ctx, *managementAddress, targetWaitTimeout); waitErr != nil {
		return waitErr
	}
	manager, err := ctrl.NewManager(config, ctrl.Options{
		Scheme: scheme, Metrics: metricsserver.Options{BindAddress: "0"}, HealthProbeBindAddress: "0",
	})
	if err != nil {
		return fmt.Errorf("creating controller manager: %w", err)
	}
	controller.RegisterIndecesOrDie(ctx, manager)
	reconciler := &controller.GatewayReconciler{
		Client: manager.GetClient(), Scheme: manager.GetScheme(),
		OnCompiled: func(publishCtx context.Context, resources envoy.ResourceBundle) (controller.XDSPublicationResult, error) {
			bundle, bundleErr := publication.BundleFromResources(&resources)
			if bundleErr != nil {
				return controller.XDSPublicationResult{}, bundleErr
			}
			response, publishErr := state.publisher.Publish(publishCtx, bundle)
			if publishErr != nil {
				return controller.XDSPublicationResult{}, publishErr
			}
			state.bundleMu.Lock()
			state.lastBundle = bundle
			state.bundleMu.Unlock()
			state.ready.Store(true)
			return controller.XDSPublicationResult{
				PersistedGeneration: response.PersistedGeneration, Digest: response.Digest,
				Activated: response.Activated, Idempotent: response.Idempotent,
			}, nil
		},
	}
	if err := reconciler.SetupWithManager(manager); err != nil {
		return fmt.Errorf("setting up Gateway reconciler: %w", err)
	}
	managerErrors := make(chan error, 1)
	go func() { managerErrors <- manager.Start(ctx) }()

	select {
	case <-ctx.Done():
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer shutdownCancel()
		if err := httpServer.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("shutting down control API: %w", err)
		}
		return nil
	case err := <-managerErrors:
		return fmt.Errorf("running controller manager: %w", err)
	case err := <-httpErrors:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return fmt.Errorf("serving control API: %w", err)
	}
}

func createDemoGateway(ctx context.Context, k8sClient client.Client) error {
	namespace := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: demoNamespace}}
	if err := k8sClient.Create(ctx, namespace); err != nil && !apierrors.IsAlreadyExists(err) {
		return fmt.Errorf("creating demo namespace: %w", err)
	}
	gateway := &gatewayv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Name: demoGatewayName, Namespace: demoNamespace,
			Labels: map[string]string{commonconfig.EnvironmentLabelKey: demoEnvironment},
		},
		Spec: gatewayv1.GatewaySpec{Type: gatewayv1.GatewayTypeEnvoy, Admin: gatewayv1.AdminConfig{}},
	}
	if err := k8sClient.Create(ctx, gateway); err != nil && !apierrors.IsAlreadyExists(err) {
		return fmt.Errorf("creating demo Gateway: %w", err)
	}
	return nil
}

func writeTarget(path, targetID string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return fmt.Errorf("creating target directory: %w", err)
	}
	temporary := path + ".tmp"
	if err := os.WriteFile(temporary, []byte(targetID+"\n"), 0o600); err != nil {
		return fmt.Errorf("writing target file: %w", err)
	}
	if err := os.Rename(temporary, path); err != nil {
		return fmt.Errorf("activating target file: %w", err)
	}
	return nil
}

func waitForTCP(ctx context.Context, address string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		connection, err := (&net.Dialer{Timeout: time.Second}).DialContext(ctx, "tcp", address)
		if err == nil {
			if closeErr := connection.Close(); closeErr != nil {
				return fmt.Errorf("closing management probe connection: %w", closeErr)
			}
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("management server %q was unavailable within %s", address, timeout)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
}
