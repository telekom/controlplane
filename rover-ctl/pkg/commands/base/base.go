// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package base

import (
	"context"
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

	cmd.Cmd.PersistentFlags().BoolVar(&cmd.FailFast, "fail-fast", true, "Stop processing on the first error encountered")

	return cmd
}

func (c *BaseCommand) Logger() logr.Logger {
	if c.logger.IsZero() {
		c.logger = log.L().WithName(fmt.Sprintf("%s-cmd", c.Cmd.Use))
	}
	return c.logger
}

func (c *BaseCommand) HandleError(err error, ctxInfo string) error {
	common.PrintTo(err, c.Cmd.ErrOrStderr(), viper.GetString("log.format"))
	if c.FailFast {
		return errors.Wrapf(err, "failed to %s", ctxInfo)
	}
	c.Logger().Info(fmt.Sprintf("⚠️ Failed to %s", ctxInfo))
	return nil
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
