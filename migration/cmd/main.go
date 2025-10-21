// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"flag"
	"os"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	approvalv1 "github.com/telekom/controlplane/approval/api/v1"
	"github.com/telekom/controlplane/migration/internal/client"
	"github.com/telekom/controlplane/migration/internal/controller"
	"github.com/telekom/controlplane/migration/internal/handler/approvalrequest"
	"github.com/telekom/controlplane/migration/internal/mapper"
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(approvalv1.AddToScheme(scheme))
}

func main() {
	var metricsAddr string
	var enableLeaderElection bool
	var probeAddr string
	var remoteClusterSecretName string
	var remoteClusterSecretNamespace string

	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flag.StringVar(&remoteClusterSecretName, "remote-cluster-secret-name", "remote-cluster-token",
		"Name of the secret containing remote cluster credentials")
	flag.StringVar(&remoteClusterSecretNamespace, "remote-cluster-secret-namespace", "controlplane-system",
		"Namespace of the secret containing remote cluster credentials")

	opts := zap.Options{
		Development: true,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme: scheme,
		Metrics: metricsserver.Options{
			BindAddress: metricsAddr,
		},
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "migration-operator.controlplane.telekom.de",
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	// Create remote cluster client from secret
	setupLog.Info("Loading remote cluster configuration",
		"secretName", remoteClusterSecretName,
		"secretNamespace", remoteClusterSecretNamespace)

	remoteClient, err := createRemoteClient(mgr, remoteClusterSecretName, remoteClusterSecretNamespace)
	if err != nil {
		setupLog.Error(err, "unable to create remote cluster client")
		os.Exit(1)
	}

	// Create mapper and handler
	approvalMapper := mapper.NewApprovalMapper()
	migrationHandler := approvalrequest.NewMigrationHandler(
		mgr.GetClient(),
		remoteClient,
		approvalMapper,
		ctrl.Log.WithName("handler").WithName("ApprovalRequest"),
	)

	// Setup controller
	if err = controller.NewApprovalRequestMigrationReconciler(
		mgr.GetClient(),
		mgr.GetScheme(),
		migrationHandler,
		ctrl.Log.WithName("controller").WithName("ApprovalRequest"),
	).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "ApprovalRequest")
		os.Exit(1)
	}

	// Add health checks
	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}

func createRemoteClient(mgr ctrl.Manager, secretName, secretNamespace string) (*client.RemoteClusterClient, error) {
	ctx := context.Background()

	// Fetch the secret containing remote cluster credentials
	secret := &corev1.Secret{}
	if err := mgr.GetAPIReader().Get(ctx, types.NamespacedName{
		Name:      secretName,
		Namespace: secretNamespace,
	}, secret); err != nil {
		return nil, err
	}

	// Extract credentials from secret
	config := &client.RemoteClusterConfig{
		APIServer: string(secret.Data["server"]),
		Token:     string(secret.Data["token"]),
		CAData:    secret.Data["ca.crt"],
	}

	// Check if kubeconfig is provided
	if kubeconfig, ok := secret.Data["kubeconfig"]; ok {
		config.Kubeconfig = kubeconfig
	}

	return client.NewRemoteClusterClient(config)
}
