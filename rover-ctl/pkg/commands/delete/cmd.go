// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package delete

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

// Command represents the delete command
type Command struct {
	*base.FileCommand
}

// NewCommand creates a new delete command
func NewCommand() *cobra.Command {
	cmd := &Command{
		FileCommand: base.NewFileCommand(
			"delete",
			"Delete a resource",
			"Delete a resource defined in a file or directory from the server",
		),
	}

	cmd.Cmd.MarkFlagRequired("file")

	// Set the run function
	cmd.Cmd.RunE = cmd.Run

	return cmd.Cmd
}

// Run executes the delete command
func (c *Command) Run(cmd *cobra.Command, args []string) error {
	c.Logger().V(1).Info("Starting delete command")

	if err := c.ParseFiles(); err != nil {
		return err
	}

	return base.ForEach(c.BaseCommand, "delete", handlers.Sort(c.Parser.Objects()), func(obj types.Object) error {
		c.Logger().V(1).Info("Processing object", "kind", obj.GetKind(), "name", obj.GetName())
		if err := c.deleteObject(obj); err != nil {
			return errors.Wrapf(err, "%s %q", obj.GetKind(), obj.GetName())
		}
		return nil
	})
}

// deleteObject processes a single object from the parser
func (c *Command) deleteObject(obj types.Object) error {
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

	c.Logger().Info(fmt.Sprintf("🧹 Deleting %s",
		obj.GetKind()),
		"apiVersion", obj.GetApiVersion(),
		"name", obj.GetName())

	// Delete the object using the handler
	if err = handler.Delete(c.Cmd.Context(), obj); err != nil {
		return errors.Wrap(err, "failed to delete")
	}

	status, err := handler.WaitForDeleted(c.Cmd.Context(), obj.GetName())
	if err != nil {
		waitErr := errors.Wrap(err, "failed to wait for deletion")
		if status == nil {
			return waitErr
		}
		if printErr := common.NewStatusEval(obj, status).PrettyPrint(c.Cmd.OutOrStdout(), viper.GetString("log.format")); printErr != nil {
			return stderrors.Join(waitErr, errors.Wrap(printErr, "failed to print status"))
		}
		return waitErr
	}

	if status.IsGone() {
		c.Logger().Info(fmt.Sprintf("✅ Successfully deleted %s",
			obj.GetKind()),
			"name", obj.GetName())
	}

	return nil
}
