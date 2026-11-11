# Copyright 2026 Deutsche Telekom AG
#
# SPDX-License-Identifier: Apache-2.0

"""Run: python3 -m unittest discover -s .agents/tools/tests"""

from __future__ import annotations

import contextlib
import io
import sys
import tempfile
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent.parent))

import spec_field_docs  # noqa: E402

TYPES = """package v1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// WidgetSpec defines the desired state of Widget.
type WidgetSpec struct {
	// Size selects how many replicas serve the widget.
	// +kubebuilder:validation:Minimum=1
	Size int `json:"size"`
	// +kubebuilder:validation:Enum=a;b
	Mode string `json:"mode"`
	Inner InnerConfig `json:"inner"`
	Tags []string `json:"tags"` // Tags are copied onto every child object.
	// Timeout configuration
	Timeout string `json:"timeout"`
	// ClientID is the OAuth2 client ID
	ClientID string `json:"clientId"`
	Skipped string `json:"-"`
	hidden string
	Anon struct {
		Nested string `json:"nested"`
	} `json:"anon"`
}

type InnerConfig struct {
	Url string `json:"url"`
	Refs map[string]*Leaf `json:"refs"`
}

type Leaf struct {
	// Name is used to look up the leaf.
	Name string `json:"name"`
}

type WidgetStatus struct {
	Phase string `json:"phase"`
	Inner InnerConfig `json:"inner"`
}

type Widget struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   WidgetSpec   `json:"spec,omitempty"`
	Status WidgetStatus `json:"status,omitempty"`
}
"""


class SpecFieldDocsTest(unittest.TestCase):
    def setUp(self) -> None:
        self.tmp = tempfile.TemporaryDirectory()
        self.dir = Path(self.tmp.name)
        self.path = self.dir / "widget_types.go"
        self.path.write_text(TYPES, encoding="utf-8")
        (self.dir / "widget_types_test.go").write_text(
            'package v1\n\ntype ExtraSpec struct {\n\tX string `json:"x"`\n}\n',
            encoding="utf-8",
        )

    def tearDown(self) -> None:
        self.tmp.cleanup()

    def fields(self, thin: bool) -> list[str]:
        return [
            entry.split(": spec field ")[1].split()[0]
            for entry in spec_field_docs.report([str(self.path)], thin)
        ]

    def test_reports_spec_fields_without_prose(self) -> None:
        self.assertEqual(
            self.fields(thin=False),
            [
                "WidgetSpec.Mode",
                "WidgetSpec.Inner",
                "WidgetSpec.Anon",
                "WidgetSpec.Anon.Nested",
                "InnerConfig.Url",
                "InnerConfig.Refs",
            ],
        )

    def test_documented_anonymous_parent_still_checks_nested_fields(self) -> None:
        self.path.write_text(
            'package v1\n\ntype WidgetSpec struct {\n'
            '\t// Anon configures nested behavior.\n\tAnon struct {\n'
            '\t\t// +optional\n\t\tNested string `json:"nested"`\n'
            '\t} `json:"anon"`\n}\n',
            encoding="utf-8",
        )
        entries = spec_field_docs.report([str(self.path)], False)
        self.assertEqual(len(entries), 1)
        self.assertIn("WidgetSpec.Anon.Nested", entries[0])
        self.assertIn("[audit-span=6-7]", entries[0])

    def test_ignored_anonymous_struct_does_not_expose_children(self) -> None:
        self.path.write_text(
            'package v1\n\ntype WidgetSpec struct {\n'
            '\tAnon struct {\n\t\tNested string `json:"nested"`\n'
            '\t} `json:"-"`\n}\n',
            encoding="utf-8",
        )
        self.assertEqual(self.fields(False), [])

    def test_inlined_anonymous_struct_still_exposes_children(self) -> None:
        self.path.write_text(
            'package v1\n\ntype WidgetSpec struct {\n'
            '\tAnon struct {\n\t\tNested string `json:"nested"`\n'
            '\t} `json:",inline"`\n}\n',
            encoding="utf-8",
        )
        self.assertEqual(self.fields(False), ["WidgetSpec.Anon.Nested"])

    def test_thin_reports_name_restatements_only(self) -> None:
        self.assertEqual(
            self.fields(thin=True),
            ["WidgetSpec.Timeout", "WidgetSpec.ClientID"],
        )

    def test_novel_words_ignore_name_type_and_filler(self) -> None:
        item = spec_field_docs.Field(
            name="ClientID", line=1, type_names=[], documented=True,
            serialised=True, prose="ClientID is the OAuth2 client ID", type_expr="string",
        )
        self.assertEqual(spec_field_docs.novel_words(item), ["oauth2"])

    def test_status_only_types_and_test_files_are_ignored(self) -> None:
        both = self.fields(thin=False) + self.fields(thin=True)
        self.assertFalse(
            any(
                name.startswith(("WidgetStatus.", "Widget.", "ExtraSpec."))
                for name in both
            )
        )

    def test_output_is_path_line_message(self) -> None:
        entry = spec_field_docs.report([str(self.path)], thin=False)[0]
        path, line, message = entry.split(":", 2)
        self.assertEqual(path, str(self.path))
        self.assertEqual(
            TYPES.splitlines()[int(line) - 1].strip(), 'Mode string `json:"mode"`'
        )
        self.assertIn("has no purpose comment", message)

    def test_non_go_and_missing_files_are_skipped(self) -> None:
        self.assertEqual(
            spec_field_docs.report([str(self.dir / "absent.go"), "README.md"], False),
            [],
        )

    def test_main_prints_and_exits_zero(self) -> None:
        out = io.StringIO()
        with contextlib.redirect_stdout(out):
            code = spec_field_docs.main(["--thin", str(self.path)])
        self.assertEqual(code, 0)
        self.assertEqual(len(out.getvalue().splitlines()), 2)


if __name__ == "__main__":
    unittest.main()
