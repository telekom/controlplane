// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package changeset

import "flag"

// Flags are the command-line flags of the change pipeline.
type Flags struct {
	Mode         string
	IgnoreConfig string
}

// Bind registers the flags on fs.
func (f *Flags) Bind(fs *flag.FlagSet) {
	fs.StringVar(&f.Mode, "migration-mode", string(ModeOff),
		"Migration mode: off, on or strict. With on, a backend change with a diff that is neither ignored nor "+
			"approved is blocked. With strict, every backend change needs an approval.")
	fs.StringVar(&f.IgnoreConfig, "migration-ignore-config", "",
		"Path to the optional ignore list of expected backend diffs in migration mode.")
}

// Gate returns the Gate configured by the flags for the operator and its entity kinds.
func (f *Flags) Gate(operator string, kinds []string) (*Gate, error) {
	mode, err := ParseMode(f.Mode)
	if err != nil {
		return nil, err
	}
	return Load(Options{Operator: operator, Mode: mode, Kinds: kinds}, f.IgnoreConfig)
}
