// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package resetsecret

import (
	"context"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/telekom/controlplane/rover-ctl/pkg/commands/base"
	"github.com/telekom/controlplane/rover-ctl/pkg/handlers"
	"github.com/telekom/controlplane/rover-ctl/pkg/util"
)

type ResetSecretHandler interface {
	handlers.ResourceHandler
	ResetSecret(ctx context.Context, name string) (clientId, clientSecret string, err error)
}

type Command struct {
	*base.BaseCommand
	Name string
}

// NewCommand creates a new delete command
func NewCommand() *cobra.Command {
	baseCmd := base.NewCommand(
		"reset-secret",
		"Reset a secret",
		"Reset a secret for an application",
	)
	cmd := &Command{
		BaseCommand: baseCmd,
	}

	baseCmd.Cmd.Flags().StringVarP(&cmd.Name, "application", "a", "", "Name of the application to reset the secret for")
	baseCmd.Cmd.MarkFlagRequired("application")

	cmd.Cmd.RunE = cmd.Run

	return cmd.Cmd
}

func (c *Command) Run(cmd *cobra.Command, args []string) error {

	handler, err := handlers.GetHandler("Rover", "tcp.ei.telekom.de/v1")
	if err != nil {
		return errors.Wrap(err, "failed to get rover handler")
	}

	roverHandler, ok := handler.(ResetSecretHandler)
	if !ok {
		return errors.New("invalid rover handler type")
	}

	c.Logger().V(1).Info("Starting reset-secret command")

	clientId, clientSecret, err := roverHandler.ResetSecret(c.Cmd.Context(), c.Name)
	if err != nil {
		return c.HandleError(err, "reset secret")
	}

	prettyString, err := util.FormatOutput(map[string]string{
		"clientId":     clientId,
		"clientSecret": clientSecret,
	}, viper.GetString("output.format"))

	if err != nil {
		return errors.Wrap(err, "failed to format output")
	}

	_, err = c.Cmd.OutOrStdout().Write([]byte(prettyString))
	if err != nil {
		return errors.Wrap(err, "failed to write output")
	}

	c.Logger().V(1).Info("Successfully reset secret for application", "name", c.Name)

	return nil
}
