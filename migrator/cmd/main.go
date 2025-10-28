// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"flag"
	"os"

	approvalv1 "github.com/telekom/controlplane/approval/api/v1"
	"github.com/telekom/controlplane/migrator/internal/migrators/approvalrequest"
	"github.com/telekom/controlplane/migrator/pkg/client"
	"github.com/telekom/controlplane/migrator/pkg/framework"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
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
	var secretName string
	var secretNamespace string

	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false, "Enable leader election for controller manager.")
	flag.StringVar(&secretName, "remote-cluster-secret-name", "remote-cluster-token", "Name of the secret containing remote cluster credentials")
	flag.StringVar(&secretNamespace, "remote-cluster-secret-namespace", "controlplane-system", "Namespace of the secret")

	opts := zap.Options{Development: true}
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
		LeaderElectionID:       "migrator.controlplane.telekom.de",
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	// Setup remote cluster client
	setupLog.Info("Loading remote cluster configuration",
		"secretName", secretName,
		"secretNamespace", secretNamespace)

	remoteClient, err := client.NewRemoteClusterClient(
		mgr.GetAPIReader(),
		secretName,
		secretNamespace,
		scheme,
	)
	if err != nil {
		setupLog.Error(err, "unable to create remote cluster client")
		os.Exit(1)
	}

	// Create migrator registry
	registry := framework.NewRegistry()

	// Register migrators
	setupLog.Info("Registering migrators")

	if err := registry.Register(approvalrequest.NewApprovalRequestMigrator()); err != nil {
		setupLog.Error(err, "failed to register ApprovalRequest migrator")
		os.Exit(1)
	}

	// TODO: Register more migrators here
	// Example:
	// if err := registry.Register(rover.NewRoverMigrator()); err != nil {
	//     setupLog.Error(err, "failed to register Rover migrator")
	//     os.Exit(1)
	// }

	setupLog.Info("Registered migrators",
		"count", registry.Count(),
		"migrators", registry.List())

	// Setup all migrators
	setupLog.Info("Setting up migrators with manager")
	if err := registry.SetupAll(mgr, remoteClient, setupLog); err != nil {
		setupLog.Error(err, "unable to setup migrators")
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
