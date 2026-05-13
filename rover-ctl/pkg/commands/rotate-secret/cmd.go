// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package rotatesecret

import (
	"context"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/telekom/controlplane/rover-ctl/pkg/commands/base"
	"github.com/telekom/controlplane/rover-ctl/pkg/handlers"
	v0 "github.com/telekom/controlplane/rover-ctl/pkg/handlers/v0"
	"github.com/telekom/controlplane/rover-ctl/pkg/util"
)

type RotateSecretHandler interface {
	handlers.ResourceHandler
	ResetSecret(ctx context.Context, name string) (*v0.SecretRotationStatusResponse, error)
}

type Command struct {
	*base.BaseCommand
	Name string
}

// NewCommand creates a new rotate-secret command
func NewCommand() *cobra.Command {
	baseCmd := base.NewCommand(
		"rotate-secret",
		"Rotate a secret",
		"Rotate a secret for an application and wait until the new secret has converged.",
	)
	cmd := &Command{
		BaseCommand: baseCmd,
	}

	cmd.Cmd.Flags().StringVarP(&cmd.Name, "application", "a", "", "Name of the application to rotate the secret for")
	cmd.Cmd.Flags().StringVarP(&cmd.Name, "name", "n", "", "Name of the application to rotate the secret for")
	cmd.Cmd.MarkFlagsMutuallyExclusive("application", "name")
	cmd.Cmd.MarkFlagsOneRequired("application", "name")

	cmd.Cmd.RunE = cmd.Run

	return cmd.Cmd
}

func (c *Command) Run(cmd *cobra.Command, args []string) error {
	handler, err := handlers.GetHandler("Rover", "tcp.ei.telekom.de/v1")
	if err != nil {
		return errors.Wrap(err, "failed to get rover handler")
	}

	roverHandler, ok := handler.(RotateSecretHandler)
	if !ok {
		return errors.New("invalid rover handler type")
	}

	c.Logger().Info("Rotating secret", "name", c.Name)

	status, err := roverHandler.ResetSecret(cmd.Context(), c.Name)
	if err != nil {
		return c.HandleError(err, "rotate secret")
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
