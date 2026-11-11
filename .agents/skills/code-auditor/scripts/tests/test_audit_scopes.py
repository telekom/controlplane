# Copyright 2026 Deutsche Telekom AG
#
# SPDX-License-Identifier: Apache-2.0

"""CLI contracts for explicit local, staged and branch/PR audit scopes."""

import json
import tempfile
import unittest
from pathlib import Path

from test_config_root import JUDGES, git, init_repo, run


class AuditScopesTest(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.repo = Path(self.tmp.name)
        init_repo(self.repo)
        agents = self.repo / ".agents"
        (agents / "rules").mkdir(parents=True)
        (agents / "judges.yaml").write_text(JUDGES)
        (agents / "rules/01-no-bad.md").write_text(
            "---\nid: 01-no-bad\ndescription: No BAD text.\nseverity: HIGH\n"
            "complexity: 0\nscope: line\napplies_to:\n  - '*.txt'\n---\n"
            "\n## Detect\n\n```regex\nBAD\n```\n"
        )
        git(self.repo, "add", ".")
        git(self.repo, "commit", "-qm", "base configuration")
        self.base = git(self.repo, "rev-parse", "HEAD").strip()
        git(self.repo, "update-ref", "refs/remotes/origin/main", self.base)
        git(self.repo, "switch", "-c", "feature")
        (self.repo / "committed.txt").write_text("BAD committed\n")
        git(self.repo, "add", ".")
        git(self.repo, "commit", "-qm", "committed feature")
        self.tip = git(self.repo, "rev-parse", "HEAD").strip()
        (self.repo / "staged.txt").write_text("BAD staged\n")
        git(self.repo, "add", "staged.txt")
        (self.repo / "staged.txt").write_text("BAD unstaged version\n")
        (self.repo / "untracked.txt").write_text("BAD untracked\n")

    def create(self, *args):
        return run("create_audit_report.py", self.repo, *args)

    def report(self):
        return json.loads((self.repo / ".agents/.audit/report.json").read_text())

    def paths(self):
        return {f["path"] for f in self.report()["scope"]["files"]}

    def test_missing_scope_fails_before_writing_state(self):
        result = self.create()
        self.assertEqual(result.returncode, 2)
        self.assertIn("--mode", result.stderr)
        self.assertFalse((self.repo / ".agents/.audit").exists())

    def test_local_is_against_head_and_includes_untracked(self):
        result = self.create("--mode", "local")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(self.paths(), {"staged.txt", "untracked.txt"})
        self.assertEqual(self.report()["base"], self.tip)
        self.assertEqual(self.report()["mode"], "local")
        self.assertIn("BAD unstaged version", [i["anchor"] for i in self.report()["items"]])
        self.assertIn("Scope: local", result.stdout)
        self.assertIn(self.tip, result.stdout)

    def test_staged_uses_index_and_records_head_base(self):
        result = self.create("--mode", "staged")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(self.paths(), {"staged.txt"})
        self.assertEqual(self.report()["base"], self.tip)
        self.assertEqual([i["anchor"] for i in self.report()["items"]], ["BAD staged"])
        self.assertIn("Scope: staged", result.stdout)

    def test_pr_range_ignores_local_edits_and_untracked(self):
        (self.repo / "committed.txt").write_text("local fix must not hide committed issue\n")
        result = self.create("--mode", "range", "--base", "origin/main")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(self.paths(), {"committed.txt"})
        report = self.report()
        self.assertEqual(report["base"], self.base)
        self.assertEqual(report["comparison"]["revision"], self.tip)
        self.assertEqual(report["comparison"]["target"], "origin/main")
        self.assertEqual([i["anchor"] for i in report["items"]], ["BAD committed"])
        self.assertIn("Scope: range", result.stdout)
        self.assertIn(self.base, result.stdout)
        self.assertIn(self.tip, result.stdout)

    def test_range_defaults_to_target_not_feature_upstream(self):
        git(self.repo, "branch", "published-tip")
        git(self.repo, "branch", "--set-upstream-to=published-tip")
        result = self.create("--mode", "range")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(self.report()["base"], self.base)
        self.assertEqual(self.paths(), {"committed.txt"})

    def test_explicit_target_uses_merge_base_not_target_tip(self):
        git(self.repo, "update-ref", "refs/heads/release", self.base)
        # A divergent target has content which must not appear as feature deletions.
        with tempfile.TemporaryDirectory() as temp:
            git(self.repo, "worktree", "add", temp, "release")
            target = Path(temp)
            (target / "target-only.txt").write_text("BAD target only\n")
            git(target, "add", ".")
            git(target, "commit", "-qm", "target branch advanced")
            git(self.repo, "worktree", "remove", temp)
        result = self.create("--mode", "range", "--base", "release")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(self.report()["base"], self.base)
        self.assertEqual(self.paths(), {"committed.txt"})

    def test_range_can_review_a_revision_other_than_head(self):
        result = self.create(
            "--mode", "range", "--base", "origin/main", "--revision", self.base
        )
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(self.paths(), set())
        self.assertEqual(self.report()["comparison"]["revision"], self.base)

    def test_combined_worktree_scope_is_only_explicit(self):
        result = self.create("--mode", "worktree", "--base", "origin/main")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(self.paths(), {"committed.txt", "staged.txt", "untracked.txt"})

    def test_invalid_ref_is_an_explicit_error_without_report(self):
        for option in ("--base", "--revision"):
            result = self.create("--mode", "range", option, "missing-reference")
            self.assertEqual(result.returncode, 2, result.stdout + result.stderr)
            self.assertIn("missing-reference", result.stderr)
            self.assertFalse((self.repo / ".agents/.audit/report.json").exists())

    def test_scope_options_cannot_be_silently_ignored(self):
        for mode, option in (
            ("local", "--base"), ("staged", "--base"),
            ("local", "--revision"), ("staged", "--revision"),
            ("worktree", "--revision"),
        ):
            result = self.create("--mode", mode, option, "HEAD")
            self.assertEqual(result.returncode, 2, result.stdout + result.stderr)
            self.assertIn(option, result.stderr)


if __name__ == "__main__":
    unittest.main()
