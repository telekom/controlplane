// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package base

import (
	"path/filepath"

	"github.com/pkg/errors"
	"github.com/telekom/controlplane/rover-ctl/pkg/config"
	"github.com/telekom/controlplane/rover-ctl/pkg/parser"
)

// FileCommand extends the base Command for file operations
type FileCommand struct {
	*BaseCommand
	FilePath string
	Parser   *parser.ObjectParser
}

// NewFileCommand creates a new file command with file-related flags
func NewFileCommand(use, short, long string) *FileCommand {
	baseCmd := NewCommand(use, short, long)

	cmd := &FileCommand{
		BaseCommand: baseCmd,
	}

	baseCmd.Cmd.Flags().StringVarP(&cmd.FilePath, "file", "f", "", "Path to the file or directory containing resource definitions")

	return cmd
}

// InitParser initializes the parser for processing files
func (c *FileCommand) InitParser() error {
	c.Parser = parser.NewObjectParser(parser.Opts...)

	c.Logger().V(1).Info("Initialized parser")

	return nil
}

// ParseFiles parses files using the configured parser
func (c *FileCommand) ParseFiles() error {
	if c.Parser == nil {
		c.Logger().V(1).Info("Parser not initialized, initializing now")
		if err := c.InitParser(); err != nil {
			return err
		}
	}

	path := resolveFilePath(c.FilePath)
	c.Logger().V(1).Info("Parsing files", "path", path)

	if err := c.Parser.Parse(path); err != nil {
		return errors.Wrap(err, "failed to parse files")
	}

	c.Logger().V(1).Info("Successfully parsed files", "count", len(c.Parser.Objects()))

	return nil
}

// resolveFilePath joins a nonempty relative path with the configured resource path if set.
// Empty and absolute paths are returned unchanged.
func resolveFilePath(path string) string {
	base := config.ResourcePath()
	if base == "" || path == "" || filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(base, path)
}
