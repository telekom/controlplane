// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package apply

import (
	"fmt"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/telekom/controlplane/rover-ctl/pkg/commands/base"
	"github.com/telekom/controlplane/rover-ctl/pkg/handlers"
	"github.com/telekom/controlplane/rover-ctl/pkg/handlers/common"
	"github.com/telekom/controlplane/rover-ctl/pkg/types"
)

// Command represents the apply command
type Command struct {
	*base.FileCommand
}

// NewCommand creates a new apply command
func NewCommand() *cobra.Command {
	cmd := &Command{
		FileCommand: base.NewFileCommand(
			"apply",
			"Apply a resource configuration",
			"Apply a resource configuration from file or directory to the server",
		),
	}

	// Set the run function
	cmd.Cmd.RunE = cmd.Run
	cmd.Cmd.PreRunE = func(_ *cobra.Command, args []string) error {
		return cmd.SetupToken()
	}

	return cmd.Cmd
}

// Run executes the apply command
func (c *Command) Run(cmd *cobra.Command, args []string) error {
	c.Logger().V(1).Info("Starting apply command")

	if err := c.ParseFiles(); err != nil {
		return err
	}

	// Process objects
	for _, obj := range handlers.Sort(c.Parser.Objects()) {
		c.Logger().V(1).Info("Processing object", "kind", obj.GetKind(), "name", obj.GetName())

		if err := c.applyObject(obj); err != nil {
			return errors.Wrapf(err, "failed to apply object %s", obj.GetName())
		}

	}

	return nil
}

// applyObject processes a single object from the parser
func (c *Command) applyObject(obj types.Object) error {
	// Get the appropriate handler based on the object kind and API version
	handler, err := handlers.GetHandler(obj.GetKind(), obj.GetApiVersion())
	if err != nil {
		return errors.Wrapf(err, "no handler found for %s/%s",
			obj.GetApiVersion(), obj.GetKind())
	}

	c.Logger().Info(fmt.Sprintf("🚀 Applying %s",
		obj.GetKind()),
		"name", obj.GetName())

	// Apply the object using the handler
	if err := handler.Apply(c.Cmd.Context(), obj); err != nil {
		if c.FailFast {
			return errors.Wrap(err, "failed to apply object")
		}
		c.Logger().Error(err, "Failed to apply object, continuing due to fail-fast setting")
		return nil
	}

	status, err := handler.WaitForReady(c.Cmd.Context(), obj.GetName())
	if err != nil {
		if c.FailFast {
			return errors.Wrap(err, "failed to get status")
		}
		c.Logger().Error(err, "Failed to get status, continuing due to fail-fast setting")
		return nil
	}

	statusEval := common.NewStatusEval(obj, status)
	if statusEval.IsSuccess() {
		c.Logger().Info(fmt.Sprintf("✅ Successfully applied %s",
			obj.GetKind()),
			"name", obj.GetName())
	} else {
		if err := statusEval.PrettyPrint(c.Cmd.OutOrStdout(), viper.GetString("log.format")); err != nil {
			if c.FailFast {
				return errors.Wrap(err, "failed to print status")
			}
		}
	}

	return nil
}
