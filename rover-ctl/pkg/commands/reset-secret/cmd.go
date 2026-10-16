// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package resetsecret

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/telekom/controlplane/rover-ctl/pkg/commands/base"
	"github.com/telekom/controlplane/rover-ctl/pkg/handlers"
	v0 "github.com/telekom/controlplane/rover-ctl/pkg/handlers/v0"
	"github.com/telekom/controlplane/rover-ctl/pkg/util"
)

// DeprecationMessage is printed to stderr whenever reset-secret is used.
const DeprecationMessage = `Command "reset-secret" is deprecated and will be removed in June 2027, use "rotate-secret" instead.`

type ResetSecretHandler interface {
	handlers.ResourceHandler
	ResetSecret(ctx context.Context, name string) (*v0.SecretRotationStatusResponse, error)
}

type Command struct {
	*base.BaseCommand
	Name string
}

// NewCommand creates a new reset-secret command
func NewCommand() *cobra.Command {
	baseCmd := base.NewCommand(
		"reset-secret",
		"Reset a secret",
		"Reset a secret for an application and wait until the new secret has converged.\n\nDeprecated: will be removed in June 2027, use \"rotate-secret\" instead.",
	)
	cmd := &Command{
		BaseCommand: baseCmd,
	}

	cmd.Cmd.Flags().StringVarP(&cmd.Name, "application", "a", "", "Name of the application to reset the secret for")
	cmd.Cmd.Flags().StringVarP(&cmd.Name, "name", "n", "", "Name of the application to reset the secret for")
	cmd.Cmd.MarkFlagsMutuallyExclusive("application", "name")
	cmd.Cmd.MarkFlagsOneRequired("application", "name")

	cmd.Cmd.RunE = cmd.Run

	return cmd.Cmd
}

func (c *Command) Run(cmd *cobra.Command, args []string) error {
	// We cannot use the built-in deprecation mechanism of cobra, because it would print the message to stdout
	// potentially breaking pipes.
	fmt.Fprintln(cmd.ErrOrStderr(), DeprecationMessage)

	handler, err := handlers.GetHandler("Rover", "tcp.ei.telekom.de/v1")
	if err != nil {
		return errors.Wrap(err, "failed to get rover handler")
	}

	roverHandler, ok := handler.(ResetSecretHandler)
	if !ok {
		return errors.New("invalid rover handler type")
	}

	c.Logger().Info("Resetting secret", "name", c.Name)

	status, err := roverHandler.ResetSecret(cmd.Context(), c.Name)
	if err != nil {
		if status != nil {
			prettyString, fmtErr := util.FormatOutput(status, viper.GetString("output.format"))
			if fmtErr == nil {
				_, _ = c.Cmd.OutOrStdout().Write([]byte(prettyString))
			}
		}
		return c.HandleError(err, "reset secret")
	}

	prettyString, err := util.FormatOutput(status, viper.GetString("output.format"))
	if err != nil {
		return errors.Wrap(err, "failed to format output")
	}

	_, err = c.Cmd.OutOrStdout().Write([]byte(prettyString))
	if err != nil {
		return errors.Wrap(err, "failed to write output")
	}

	return nil
}
