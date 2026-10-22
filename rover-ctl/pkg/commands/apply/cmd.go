// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package apply

import (
	stderrors "errors"
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

	cmd.Cmd.MarkFlagRequired("file")

	// Set the run function
	cmd.Cmd.RunE = cmd.Run

	return cmd.Cmd
}

// Run executes the apply command
func (c *Command) Run(cmd *cobra.Command, args []string) error {
	c.Logger().V(1).Info("Starting apply command")

	if err := c.ParseFiles(); err != nil {
		return err
	}

	return base.ForEach(c.BaseCommand, "apply", handlers.Sort(c.Parser.Objects()), func(obj types.Object) error {
		c.Logger().V(1).Info("Processing object", "kind", obj.GetKind(), "name", obj.GetName())
		if err := c.applyObject(obj); err != nil {
			return errors.Wrapf(err, "%s %q", obj.GetKind(), obj.GetName())
		}
		return nil
	})
}

// applyObject processes a single object from the parser
func (c *Command) applyObject(obj types.Object) error {
	// Get the appropriate handler based on the object kind and API version
	handler, err := handlers.GetHandler(obj.GetKind(), obj.GetApiVersion())
	if err != nil {
		if errors.Is(err, handlers.ErrNoHandlerFound) {
			c.Logger().Info("Handler not found. Ignoring...",
				"kind", obj.GetKind(),
				"apiVersion", obj.GetApiVersion())
			return nil
		}
		return err
	}

	c.Logger().Info(fmt.Sprintf("🚀 Applying %s",
		obj.GetKind()),
		"name", obj.GetName())

	// Apply the object using the handler
	if err = handler.Apply(c.Cmd.Context(), obj); err != nil {
		return errors.Wrap(err, "failed to apply")
	}

	status, err := handler.WaitForReady(c.Cmd.Context(), obj.GetName())
	if err != nil {
		waitErr := errors.Wrap(err, "failed to wait for readiness")
		if status == nil {
			return waitErr
		}
		return stderrors.Join(waitErr, c.printStatus(obj, status))
	}

	switch status.GetOverallStatus() {
	case types.OverallStatusComplete:
		c.Logger().Info(fmt.Sprintf("✅ Successfully applied %s",
			obj.GetKind()),
			"name", obj.GetName())
		return nil
	case types.OverallStatusBlocked:
		// Blocked means waiting for an external action (e.g. an approval).
		// It is not a failure, but the resource is not necessarily ready.
		if err := c.printStatus(obj, status); err != nil {
			return err
		}
		c.Logger().Info(fmt.Sprintf("⏸️  Applied %s, but it is blocked and not necessarily ready",
			obj.GetKind()),
			"name", obj.GetName())
		return nil
	case types.OverallStatusDone:
		if err := c.printStatus(obj, status); err != nil {
			return err
		}
		c.Logger().Info(fmt.Sprintf("☑️  Applied %s, processing is done but it is not necessarily ready",
			obj.GetKind()),
			"name", obj.GetName())
		return nil
	}

	statusErr := fmt.Errorf("finished with status %q", status.GetOverallStatus())
	return stderrors.Join(statusErr, c.printStatus(obj, status))
}

// printStatus prints the status of the object.
func (c *Command) printStatus(obj types.Object, status types.ObjectStatus) error {
	if err := common.NewStatusEval(obj, status).PrettyPrint(c.Cmd.OutOrStdout(), viper.GetString("log.format")); err != nil {
		return errors.Wrap(err, "failed to print status")
	}
	return nil
}
