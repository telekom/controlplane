# Copyright 2026 Deutsche Telekom AG
#
# SPDX-License-Identifier: Apache-2.0

"""Inline audit ignores: comment markers that excuse a finding in place."""

import copy
import tempfile
import unittest
from pathlib import Path

from test_config_root import JUDGES, git, init_repo, run
from auditlib.context import resolve
from auditlib.inline import InlineIgnores, hint
import create_audit_report as create
import process_audit_report as process

DETERMINISTIC_RULE = (
    "---\nid: 01-no-bad\ndescription: No BAD.\nseverity: HIGH\n"
    "complexity: 0\nscope: line\napplies_to:\n  - '*.go'\n---\n"
    "\n## Detect\n\n```regex\nBAD\n```\n"
)
JUDGED_RULES = {
    "whole file": (
        "---\nid: 01-no-bad\ndescription: No BAD.\nseverity: HIGH\n"
        "complexity: 2\nscope: file\napplies_to:\n  - '*.go'\n---\n"
        "\n## Checks\n\n- [bad] Is BAD wrong?\n"
        "\n## Examples\n\n### Violation\n\n```text source=a.go:1@abc1234\nBAD\n```\n"
    ),
    "file-scoped detector": (
        "---\nid: 01-no-bad\ndescription: No BAD.\nseverity: HIGH\n"
        "complexity: 1\nscope: file\napplies_to:\n  - '*.go'\n---\n"
        "\n## Detect\n\n```regex\nBAD\n```\n"
        "\n## Checks\n\n- [bad] Is BAD wrong?\n"
        "\n## Examples\n\n### Violation\n\n```text source=a.go:1@abc1234\nBAD\n```\n"
    ),
}
SPAN_RULE = (
    "---\nid: 01-no-bad\ndescription: No BAD.\nseverity: HIGH\n"
    "complexity: 0\nscope: line\napplies_to:\n  - '*.go'\n---\n"
    "\n## Detect\n\n```tool\ncommand: python3 .agents/tool.py\n"
    "violation_when: output_lines\n```\n"
)


class MarkerParsingTest(unittest.TestCase):
    def test_placement_targets_and_comment_syntaxes(self):
        markers = InlineIgnores(
            "a() // audit:ignore 01-x trailing reason\n"
            "// audit:ignore 02-y/check standalone reason\n"
            "# audit:ignore 03-z,04-w stacked reason\n"
            "b()\n"
            "c() // mentions audit:ignore 01-x in prose\n"
            "<!-- audit:ignore 05-v html reason -->\n"
            "d()\n"
        )
        self.assertEqual(markers.find("01-x", "violation", [1]).line, 1)
        self.assertIsNone(markers.find("01-x", "violation", [2]))
        self.assertEqual(markers.find("02-y", "check", [4]).line, 2)
        self.assertIsNone(markers.find("02-y", "other", [4]))
        self.assertEqual(markers.find("03-z", "violation", [4]).line, 3)
        self.assertEqual(markers.find("04-w", "any", [4]).line, 3)
        self.assertIsNone(markers.find("01-x", "violation", [5]))
        self.assertEqual(markers.find("05-v", "violation", [7]).reason, "html reason")

    def test_marker_above_reaches_past_its_comment_block(self):
        markers = InlineIgnores(
            "\t// audit:ignore 01-x a reason that is long enough\n"
            "\t// to wrap onto a second comment line\n"
            "\tcode()\n"
            "\tmore()\n"
            "# audit:ignore 02-y yaml reason\n"
            "// not the same opener\n"
            "\n"
            "after_blank()\n"
        )
        self.assertEqual(markers.find("01-x", "violation", [3]).line, 1)
        self.assertEqual(markers.find("01-x", "violation", [2]).line, 1)
        self.assertIsNone(markers.find("01-x", "violation", [4]))
        self.assertEqual(markers.find("02-y", "violation", [6]).line, 5)
        self.assertIsNone(markers.find("02-y", "violation", [7]))
        self.assertIsNone(markers.find("02-y", "violation", [8]))

    def test_malformed_marker_never_ignores(self):
        markers = InlineIgnores(
            "a() // audit:ignore 01-x\n"
            "b() // audit:ignore not-a-rule reason\n"
        )
        missing = markers.find("01-x", "violation", [1])
        self.assertEqual(missing.problem, "audit:ignore 01-x needs a reason")
        self.assertFalse(missing.ignores("01-x", "violation"))
        self.assertIsNone(markers.find("02-y", "violation", [1]))
        self.assertIn("'not-a-rule'", markers.find("02-y", "violation", [2]).problem)

    def test_printed_hint_is_a_working_marker(self):
        for path in ("a.go", "a.yaml", "a.md"):
            with self.subTest(path=path):
                text = hint(path, "01-x", "check").replace("<reason>", "why") + "\ncode\n"
                marker = InlineIgnores(text).find("01-x", "check", [2])
                self.assertTrue(marker.ignores("01-x", "check"), text)


class InlineIgnoreAuditTest(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.repo = Path(self.tmp.name)
        init_repo(self.repo)
        (self.repo / ".agents/rules").mkdir(parents=True)
        (self.repo / ".agents/judges.yaml").write_text(JUDGES)
        self.configure(DETERMINISTIC_RULE)
        self.context = resolve(None, start=self.repo)

    def configure(self, rule: str, **files: str) -> None:
        (self.repo / ".agents/rules/01-no-bad.md").write_text(rule)
        for name, text in files.items():
            (self.repo / ".agents" / name).write_text(text)
        git(self.repo, "add", ".agents")
        git(self.repo, "commit", "-qm", "configuration", "--", ".agents")

    def stage(self, text: str) -> None:
        (self.repo / "a.go").write_text(text)
        git(self.repo, "add", "a.go")

    def test_marker_settles_only_its_own_deterministic_finding(self):
        self.stage("BAD // audit:ignore 01-no-bad upstream fixture name\nBAD\n")
        document = create.build(self.context, "staged")
        self.assertEqual(
            [(i["line"], i["verdict"], i["source"]) for i in document["items"]],
            [(1, "PASS", "inline_ignore"), (2, "FAIL", "tool")],
        )
        gate = run("audit_gate.py", self.repo, "--fail-on", "HIGH")
        self.assertEqual(gate.returncode, 1, gate.stdout + gate.stderr)
        self.assertIn("a.go:2", gate.stderr)
        self.assertNotIn("a.go:1", gate.stderr)

        self.stage("BAD // audit:ignore 01-no-bad upstream fixture name\n")
        gate = run("audit_gate.py", self.repo, "--fail-on", "HIGH")
        self.assertEqual(gate.returncode, 0, gate.stdout + gate.stderr)

    def test_report_lists_ignores_and_explains_malformed_markers(self):
        self.stage("BAD // audit:ignore 01-no-bad upstream fixture name\nBAD // audit:ignore 01-no-bad\n")
        created = run("create_audit_report.py", self.repo, "--mode", "staged")
        self.assertEqual(created.returncode, 0, created.stdout + created.stderr)
        settled = run("process_audit_report.py", self.repo, "--fail-on", "HIGH")
        self.assertEqual(settled.returncode, 1, settled.stdout + settled.stderr)
        finding, ignored = settled.stdout.split("item(s) ignored inline")
        self.assertIn("a.go:2", finding)
        self.assertIn(
            "inline ignore at line 2 not applied: audit:ignore 01-no-bad needs a reason",
            finding,
        )
        self.assertIn("ignore inline (line above): // audit:ignore 01-no-bad <reason>", finding)
        self.assertIn("a.go:1 (marker on line 1)", ignored)
        self.assertIn("upstream fixture name", ignored)

    def test_marker_ignores_judged_finding_where_it_lands(self):
        for name, rule in JUDGED_RULES.items():
            with self.subTest(rule=name):
                self.configure(rule)
                self.check_judged_finding()

    def check_judged_finding(self):
        self.stage(
            "// audit:ignore 01-no-bad/bad fixture name, reviewed\n"
            "BAD\n"
            "BAD again\n"
        )
        document = create.build(self.context, "staged")
        item = document["items"][0]
        # A file-wide candidate is never skipped because of a marker near it.
        self.assertEqual(item["status"], "pending")

        def answer(report, line, quote):
            return process.merge(self.repo, copy.deepcopy(report), {"verdicts": [{
                "item_id": item["item_id"], "verdict": "FAIL",
                "evidence": {"line": line, "quote": quote, "reason": "BAD is wrong"},
            }]})

        self.assertEqual(answer(document, 3, "BAD again")[0]["verdict"], "FAIL")
        items = answer(document, 2, "BAD")
        self.assertEqual(items[0]["verdict"], "PASS")
        self.assertEqual(
            items[0]["inline_ignore"],
            {"line": 1, "reason": "fixture name, reviewed", "verdict": "FAIL"},
        )
        process.persist(self.context.audit_dir, document, items)
        gate = run("audit_gate.py", self.repo, "--fail-on", "HIGH")
        self.assertEqual(gate.returncode, 0, gate.stdout + gate.stderr)

        cached = create.build(self.context, "staged")
        self.assertEqual(
            (cached["items"][0]["source"], cached["items"][0]["verdict"]), ("cache", "FAIL")
        )
        self.assertEqual(process.merge(self.repo, cached, {"verdicts": []})[0]["verdict"], "PASS")

    def test_marker_anywhere_in_a_reported_span_covers_the_finding(self):
        self.configure(
            SPAN_RULE,
            **{"tool.py": "import sys\nfor p in sys.argv[1:]: print(p + ':3: undocumented [audit-span=1-3]')\n"},
        )
        (self.repo / "a.go").write_text(
            "// Field docs.\n// More docs.\nField string // audit:ignore 01-no-bad documented upstream\n"
        )
        git(self.repo, "add", "a.go")
        git(self.repo, "commit", "-qm", "field")
        self.stage(
            "// Field docs, edited.\n// More docs.\nField string // audit:ignore 01-no-bad documented upstream\n"
        )
        document = create.build(self.context, "staged")
        self.assertEqual(
            [(i["line"], i["source"]) for i in document["items"]], [(1, "inline_ignore")]
        )
        gate = run("audit_gate.py", self.repo, "--fail-on", "HIGH")
        self.assertEqual(gate.returncode, 0, gate.stdout + gate.stderr)


if __name__ == "__main__":
    unittest.main()
