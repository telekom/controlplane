// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package base

import (
	"context"
	stderrors "errors"
	"fmt"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/telekom/controlplane/rover-ctl/pkg/config"
	"github.com/telekom/controlplane/rover-ctl/pkg/handlers/common"
	"github.com/telekom/controlplane/rover-ctl/pkg/log"
)

// BaseCommand is the base structure for all commands
type BaseCommand struct {
	Cmd      *cobra.Command
	logger   logr.Logger
	FailFast bool
}

// NewCommand creates a new base command with common flags
func NewCommand(use, short, long string) *BaseCommand {
	cmd := &BaseCommand{
		Cmd: &cobra.Command{
			Use:   use,
			Short: short,
			Long:  long,
		},
	}

	cmd.Cmd.PersistentFlags().BoolVar(&cmd.FailFast, "fail-fast", false, "Stop processing further resources on the first failure (failures always cause a non-zero exit code)")

	cmd.Cmd.PreRunE = func(_ *cobra.Command, args []string) error {
		PrintBanner(cmd.Cmd)
		return cmd.SetupToken()
	}

	return cmd
}

func (c *BaseCommand) Logger() logr.Logger {
	if c.logger.IsZero() {
		c.logger = log.L().WithName(fmt.Sprintf("%s-cmd", c.Cmd.Use))
	}
	return c.logger
}

// HandleError prints the error diagnostics of a single operation and returns
// the wrapped error. The --fail-fast flag has no effect here: every failure
// results in a non-zero exit code.
func (c *BaseCommand) HandleError(err error, ctxInfo string) error {
	common.PrintTo(err, c.Cmd.ErrOrStderr(), viper.GetString("log.format"))
	return errors.Wrapf(err, "failed to %s", ctxInfo)
}

// ForEach runs op for every item. Without --fail-fast all items are processed;
// with --fail-fast processing stops at the first failure. The collected
// failures are reported once at the end via ReportFailures.
func ForEach[T any](c *BaseCommand, action string, items []T, op func(T) error) error {
	var failures []error
	for _, item := range items {
		if err := op(item); err != nil {
			failures = append(failures, err)
			if c.FailFast {
				break
			}
		}
	}
	return c.ReportFailures(action, failures)
}

// ReportFailures prints structured API error diagnostics for each failure and
// returns a single error joining all failures, or nil if there are none.
func (c *BaseCommand) ReportFailures(action string, failures []error) error {
	if len(failures) == 0 {
		return nil
	}
	format := viper.GetString("log.format")
	for _, err := range failures {
		if apiErr, ok := common.AsApiError(err); ok && apiErr != nil {
			common.PrintTo(err, c.Cmd.ErrOrStderr(), format)
		}
	}
	return errors.Wrapf(stderrors.Join(failures...), "failed to %s %d resource(s)", action, len(failures))
}

// SetupToken sets up the authorization token for the command
func (c *BaseCommand) SetupToken() error {
	ctx, err := SetupTokenInContext(c.Cmd.Context())
	if err != nil {
		return err
	}
	c.Cmd.SetContext(ctx)
	return nil
}
func SetupTokenInContext(ctx context.Context) (context.Context, error) {
	token, err := config.GetToken()
	if err != nil {
		return nil, err
	}
	ctx = config.NewContext(ctx, token)
	return ctx, nil
}
