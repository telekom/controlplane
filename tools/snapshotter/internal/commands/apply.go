// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package commands

import (
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"slices"

	"github.com/spf13/cobra"
	kong "github.com/telekom/controlplane/gateway/pkg/kong/api"
	"github.com/telekom/controlplane/gateway/pkg/kongutil"
	"github.com/telekom/controlplane/tools/snapshotter/pkg/config"
	"github.com/telekom/controlplane/tools/snapshotter/pkg/rollback"
	"github.com/telekom/controlplane/tools/snapshotter/pkg/snapshot"
	"github.com/telekom/controlplane/tools/snapshotter/pkg/store"
	"go.uber.org/zap"
	"sigs.k8s.io/controller-runtime/pkg/manager/signals"
)

// applyOptions holds the flags of the apply command.
type applyOptions struct {
	SourceKey string
	RouteId   string
	File      string
	StorePath string
	Apply     bool
	Prune     bool
}

var (
	applyOpts applyOptions
	applyCmd  = &cobra.Command{
		Use:   "apply",
		Short: "Restore a route on the gateway from a raw snapshot",
		Long: `Restore a Kong route (service, route, route plugins, upstream, targets) 1:1 on the same
gateway from a raw snapshot (taken with 'snap --raw'). Runs as a dry run unless --apply is set.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.LoadConfig(configPath)
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}
			return runApply(signals.SetupSignalHandler(), cfg, applyOpts, newStandaloneClient, os.Stdout)
		},
	}
)

func init() {
	rootCmd.AddCommand(applyCmd)

	applyCmd.Flags().StringVar(&applyOpts.SourceKey, "source", "", "Source (target gateway) to apply to (only required if multiple sources are configured)")
	applyCmd.Flags().StringVar(&applyOpts.RouteId, "route", "", "ID or name of the route whose latest snapshot is loaded from the store")
	applyCmd.Flags().StringVar(&applyOpts.File, "file", "", "Path to a snapshot file to apply")
	applyCmd.Flags().StringVar(&applyOpts.StorePath, "store", "", "Snapshot store directory (overrides storePath from the config file)")
	applyCmd.Flags().BoolVar(&applyOpts.Apply, "apply", false, "Execute the plan (default is a dry run)")
	applyCmd.Flags().BoolVar(&applyOpts.Prune, "prune", false, "Delete route plugins, targets and the upstream that exist on the gateway but not in the snapshot")
}

// clientFactory creates a Kong admin client for the source and a func that releases it.
type clientFactory func(ctx context.Context, src config.SourceConfig) (kong.ClientWithResponsesInterface, func(), error)

// newStandaloneClient verifies TLS unless src.InsecureSkipVerify is set.
func newStandaloneClient(ctx context.Context, src config.SourceConfig) (kong.ClientWithResponsesInterface, func(), error) {
	return kongutil.NewStandaloneClient(ctx, src, src.InsecureSkipVerify)
}

func validateApplyOptions(opts applyOptions) error {
	if (opts.RouteId == "") == (opts.File == "") {
		return errors.New("exactly one of --route or --file must be provided")
	}
	return nil
}

func selectSource(cfg config.Config, key string) (config.SourceConfig, error) {
	sources := cfg.GetSourceConfigs()
	if key != "" {
		src, ok := sources[key]
		if !ok {
			return config.SourceConfig{}, fmt.Errorf("no source found for source key: %s", key)
		}
		return src, nil
	}
	if len(sources) != 1 {
		return config.SourceConfig{}, errors.New("multiple sources configured, but no source key provided")
	}
	keys := slices.Collect(maps.Keys(sources))
	return sources[keys[0]], nil
}

func loadApplySnapshot(ctx context.Context, opts applyOptions, src config.SourceConfig, storePath string) (*snapshot.Snapshot, error) {
	if opts.File != "" {
		return rollback.LoadFile(opts.File)
	}
	id := (&snapshot.Snapshot{Environment: src.Environment, Zone: src.Zone, Id: opts.RouteId}).ID()
	snap := &snapshot.Snapshot{}
	if err := store.NewFileStore[*snapshot.Snapshot](storePath).GetLatest(ctx, id, snap); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, fmt.Errorf("no snapshot %q found in store %s", id, storePath)
		}
		return nil, fmt.Errorf("loading snapshot %q: %w", id, err)
	}
	return snap, nil
}

func runApply(ctx context.Context, cfg config.Config, opts applyOptions, newClient clientFactory, out io.Writer) error {
	if err := validateApplyOptions(opts); err != nil {
		return err
	}
	src, err := selectSource(cfg, opts.SourceKey)
	if err != nil {
		return err
	}

	// The store path is needed to load by route and for the backup on --apply.
	needStore := opts.RouteId != "" || opts.Apply
	storePath, err := resolveStorePath(cfg, opts.StorePath, !needStore)
	if err != nil {
		return err
	}

	snap, err := loadApplySnapshot(ctx, opts, src, storePath)
	if err != nil {
		return err
	}
	if err := rollback.Validate(snap, src.Environment, src.Zone); err != nil {
		return fmt.Errorf("refusing to apply snapshot %q: %w", snap.ID(), err)
	}

	client, closeClient, err := newClient(ctx, src)
	if err != nil {
		return fmt.Errorf("creating kong client: %w", err)
	}
	if closeClient != nil {
		defer closeClient()
	}
	rb := &rollback.Rollback{Client: client, Prune: opts.Prune}
	prepared, err := rb.Prepare(ctx, snap)
	if err != nil {
		return err
	}

	if err := formatOutput(prepared.Diff); err != nil {
		return err
	}
	printPlan(out, prepared.Plan, opts.Apply)

	if !opts.Apply {
		zap.L().Info("dry run, nothing applied (use --apply to execute)")
		return nil
	}
	if !prepared.HasWrites() {
		zap.L().Info("gateway already matches the snapshot, nothing to apply")
		return nil
	}
	backupPath, err := rb.Apply(ctx, prepared, storePath)
	if err != nil {
		return err
	}
	zap.L().Info("snapshot applied and verified", zap.String("snapshotID", snap.ID()), zap.String("backup", backupPath))
	return nil
}

func printPlan(out io.Writer, plan []rollback.Operation, apply bool) {
	header := "Planned operations (dry run):"
	if apply {
		header = "Operations:"
	}
	_, _ = fmt.Fprintln(out, header)
	if len(plan) == 0 {
		_, _ = fmt.Fprintln(out, "  none")
		return
	}
	for _, op := range plan {
		_, _ = fmt.Fprintf(out, "  %s\n", op.String())
	}
}
