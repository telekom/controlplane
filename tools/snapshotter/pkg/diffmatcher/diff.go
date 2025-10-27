// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package diffmatcher

import (
	"slices"

	"github.com/gkampitakis/go-diff/diffmatchpatch"
)

type Snapshot interface {
	String() string
}
type Result struct {
	Changed         bool   `yaml:"changed" json:"changed"`
	NumberOfChanges int    `yaml:"number_of_changes" json:"number_of_changes"`
	Text            string `yaml:"text" json:"text"`
}

func Compare(a, b Snapshot) Result {
	dmp := diffmatchpatch.New()
	var aStr, bStr string
	if a != nil {
		aStr = a.String()
	}
	if b != nil {
		bStr = b.String()
	}

	diffs := dmp.DiffMain(aStr, bStr, false)
	diffs = dmp.DiffCleanupSemantic(diffs)

	result := Result{Text: dmp.DiffPrettyText(diffs), NumberOfChanges: len(diffs)}
	result.Changed = slices.ContainsFunc(diffs, func(diff diffmatchpatch.Diff) bool {
		return diff.Type != diffmatchpatch.DiffEqual
	})
	return result
}
