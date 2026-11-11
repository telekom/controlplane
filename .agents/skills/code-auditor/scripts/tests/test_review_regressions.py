# Copyright 2026 Deutsche Telekom AG
#
# SPDX-License-Identifier: Apache-2.0

"""Regression coverage for the audit integrity review."""

from __future__ import annotations

import json
import re
import shutil
import subprocess
import sys
import tempfile
import unittest
from dataclasses import replace
from pathlib import Path

from test_config_root import (
    JUDGES, git, init_repo, run, SCRIPTS,
)
import test_config_root as fixtures

sys.path.insert(0, str(SCRIPTS))

from auditlib import changeset, detect, rules, store
from auditlib.judges import JudgeError
import process_audit_report as process
import selftest_rules as selftest

ROOT = SCRIPTS.parents[3]


class IntegrityTest(unittest.TestCase):
    def setUp(self):
        self.case = fixtures.SharedConfigTest()
        self.case.setUp()

    def tearDown(self):
        self.case.tearDown()

    def create(self, *args):
        return self.case.create("--config-root", str(self.case.workspace), *args)

    def test_detector_errors_reject_processing_without_receipts(self):
        self.case.add_spec_detector()
        (self.case.workspace / ".agents/tools/spec_field_docs.py").unlink()
        result = self.create()
        self.assertEqual(result.returncode, 2, result.stderr)
        for flag in ((), ("--json",)):
            result = run(
                "process_audit_report.py", self.case.sub,
                "--config-root", str(self.case.workspace), *flag,
            )
            self.assertEqual(result.returncode, 2, result.stdout + result.stderr)
            if flag:
                self.assertTrue(json.loads(result.stdout)["errors"])
        self.assertFalse((self.case.state / "receipts.json").exists())

    def test_missing_deterministic_tool_blocks_gate(self):
        self.case.add_spec_detector()
        (self.case.workspace / ".agents/tools/spec_field_docs.py").unlink()
        git(self.case.sub, "add", "admin/api/v1/widget_types.go")
        result = run(
            "audit_gate.py", self.case.sub, "--fail-on", "HIGH",
            "--config-root", str(self.case.workspace),
        )
        self.assertEqual(result.returncode, 2, result.stdout + result.stderr)

    def test_crashing_output_lines_tool_is_an_engine_error(self):
        self.case.add_spec_detector()
        tool = self.case.workspace / ".agents/tools/spec_field_docs.py"
        tool.write_text("raise RuntimeError('detector crashed')\n")
        result = self.create()
        self.assertEqual(result.returncode, 2, result.stdout + result.stderr)
        self.assertIn("detector crashed", " ".join(self.case.report()["errors"]))

    def test_staged_tool_cannot_see_unstaged_comment_fix(self):
        self.case.add_spec_detector()
        path = self.case.sub / "admin/api/v1/widget_types.go"
        git(self.case.sub, "add", "admin/api/v1/widget_types.go")
        staged = path.read_text()
        path.write_text(staged.replace("\tMode", "\t// Selects the operating mode.\n\tMode"))
        result = self.create("--mode", "staged")
        self.assertEqual(result.returncode, 0, result.stderr)
        items = self.case.report()["items"]
        self.assertTrue(any(i["rule_id"] == "09-spec-field-doc-comment" for i in items))

    def test_evidence_uses_recorded_blob_not_live_worktree(self):
        result = self.create()
        self.assertEqual(result.returncode, 0, result.stderr)
        report = self.case.report()
        pending = [i for i in report["items"] if i["status"] == "pending"]
        (self.case.sub / "rover/internal/handler/rover.go").write_text("package handler\n")
        answers = {"verdicts": [{
            "item_id": i["item_id"], "verdict": "FAIL",
            "evidence": {"line": 3, "quote": "// TODO fix later", "reason": "untracked"},
        } for i in pending]}
        settled = process.merge(self.case.sub, report, answers)
        self.assertTrue(any(i["source"] == "agent" for i in settled))

    def test_report_regeneration_resets_old_verdicts(self):
        self.create()
        path = self.case.state / "verdicts.json"
        path.write_text('{"verdicts":[{"item_id":"obsolete","verdict":"PASS"}]}')
        self.create()
        self.assertEqual(json.loads(path.read_text()), {"verdicts": []})

    def test_gate_honors_deterministic_waiver(self):
        self.create()
        item = next(i for i in self.case.report()["items"]
                    if i["rule_id"] == "05-docs-no-code-blocks")
        (self.case.workspace / ".agents/auditignore").write_text(
            f'{item["item_id"]}: accepted by human\n'
        )
        git(self.case.sub, "add", "docs/docs/user-journey/guide.md")
        result = run(
            "audit_gate.py", self.case.sub, "--fail-on", "HIGH",
            "--config-root", str(self.case.workspace),
        )
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)

    def test_current_judge_pin_invalidates_old_state(self):
        store.write_state(self.case.state, "github-copilot/retired#low", "index")
        with self.assertRaises(JudgeError):
            store.audited_model(self.case.state, self.case.workspace)

    def test_error_receipt_blocks_even_below_threshold(self):
        git(self.case.sub, "add", "rover/internal/handler/rover.go")
        self.create("--mode", "staged")
        report = self.case.report()
        file = report["scope"]["files"][0]
        rule = next(r for r in report["rules"] if r["id"] == "20-todo-tracked")
        receipts = store.ReceiptStore(self.case.state)
        receipts.put(
            store.receipt_key(
                file["blob_sha"], rule["content_hash"], report["model"], file["path"],
                tuple(file["changed_lines"]), report["tree_hash"], report["config_hash"],
            ), rule["id"], file["path"], "ERROR", "HIGH",
        )
        receipts.save()
        result = run(
            "audit_gate.py", self.case.sub, "--fail-on", "CRITICAL",
            "--config-root", str(self.case.workspace),
        )
        self.assertEqual(result.returncode, 2, result.stdout + result.stderr)

    def test_receipts_only_cover_applicable_paths(self):
        outside = self.case.sub / "outside.txt"
        outside.write_text("package handler\n\n// TODO fix later\n")
        self.create()
        report = self.case.report()
        items = [{**item, "source": "detector", "verdict": "FAIL"}
                 for item in report["items"]]
        process.persist(self.case.state, report, items)
        receipts = json.loads((self.case.state / "receipts.json").read_text())
        self.assertTrue(any(r["verdict"] == "FAIL" for r in receipts.values()))
        self.assertTrue(all(r["file"] != "outside.txt" for r in receipts.values()))

    def test_receipt_identity_binds_path_scope_and_dependency_tree(self):
        key = store.receipt_key("blob", "rule", "model", "a.go", (1,), "tree")
        for path, scope, tree in (
            ("b.go", (1,), "tree"), ("a.go", (2,), "tree"),
            ("a.go", (1,), "different-package-context"),
        ):
            self.assertNotEqual(
                key, store.receipt_key("blob", "rule", "model", path, scope, tree)
            )

    def test_context_edit_does_not_reuse_cached_pass(self):
        self.create()
        report = self.case.report()
        items = [{**item, "source": "agent", "verdict": "PASS",
                  "evidence": {"reason": "previous context"}}
                 for item in report["items"]]
        process.persist(self.case.state, report, items)
        path = self.case.sub / "rover/internal/handler/rover.go"
        path.write_text(path.read_text().replace("package handler", "package handler\n// changed guard"))
        self.create()
        item = next(i for i in self.case.report()["items"] if i["rule_id"] == "20-todo-tracked")
        self.assertEqual(item["status"], "pending")
        self.assertNotEqual(item["source"], "cache")

    def test_comment_only_edit_is_a_spec_detector_candidate(self):
        self.case.add_spec_detector()
        path = self.case.sub / "admin/api/v1/widget_types.go"
        text = path.read_text().replace("\tMode", "\t// Selects the operating mode.\n\tMode")
        path.write_text(text)
        git(self.case.sub, "add", "admin/api/v1/widget_types.go")
        git(self.case.sub, "commit", "-qm", "document field")
        git(self.case.sub, "update-ref", "refs/remotes/origin/main", "HEAD")
        # Replace only the prose comment; the field declaration is untouched.
        path.write_text(text.replace("// Selects the operating mode.", "// +optional"))
        self.create()
        items = [i for i in self.case.report()["items"]
                 if i["rule_id"] == "09-spec-field-doc-comment"]
        self.assertEqual(len(items), 1)
        self.assertEqual(items[0]["line"], 4)

    def test_removed_prose_is_a_spec_detector_candidate(self):
        self.case.add_spec_detector()
        path = self.case.sub / "admin/api/v1/widget_types.go"
        original = path.read_text()
        path.write_text(original.replace("\tMode", "\t// Selects the operating mode.\n\tMode"))
        git(self.case.sub, "add", "admin/api/v1/widget_types.go")
        git(self.case.sub, "commit", "-qm", "document field")
        git(self.case.sub, "update-ref", "refs/remotes/origin/main", "HEAD")
        path.write_text(original)
        self.create()
        self.assertTrue(any(
            i["rule_id"] == "09-spec-field-doc-comment"
            for i in self.case.report()["items"]
        ))

    def test_staged_detector_uses_staged_sibling_package_context(self):
        self.case.add_spec_detector()
        path = self.case.sub / "admin/api/v1/widget_types.go"
        path.write_text(
            'package v1\n\ntype WidgetSpec struct {\n'
            '\t// Selects nested options.\n\tNested Options `json:"nested"`\n}\n'
        )
        sibling = path.with_name("options.go")
        sibling.write_text('package v1\n\ntype Options struct {\n\tMode string `json:"mode"`\n}\n')
        # Include sibling Go files in this fixture's detector scope.
        rule = self.case.workspace / ".agents/rules/09-spec-field-doc-comment.md"
        rule.write_text(rule.read_text().replace("*_types.go", "*.go"))
        git(self.case.sub, "add", "admin/api/v1")
        sibling.write_text("package v1\n")
        result = self.create("--mode", "staged")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertTrue(any(
            i["file"].endswith("options.go") for i in self.case.report()["items"]
        ))

    def test_deletion_only_edit_is_discovered(self):
        path = self.case.sub / "rover/internal/handler/rover.go"
        path.write_text("package handler\n// label set here\n// TODO fix later\n")
        git(self.case.sub, "add", "rover/internal/handler/rover.go")
        git(self.case.sub, "commit", "-qm", "base")
        git(self.case.sub, "update-ref", "refs/remotes/origin/main", "HEAD")
        path.write_text("package handler\n// TODO fix later\n")
        self.create()
        self.assertTrue(any(
            i["rule_id"] == "20-todo-tracked" for i in self.case.report()["items"]
        ))

    def test_new_judge_pin_is_rejected_after_audit(self):
        self.create()
        (self.case.workspace / ".agents/judges.yaml").write_text(
            JUDGES.replace("gpt-6-luna", "new-model")
        )
        with self.assertRaises(JudgeError):
            store.audited_model(self.case.state, self.case.workspace)


class DiscoveryTest(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.corpus = {r.id: r for r in rules.load_rules(ROOT)}

    def test_reconcile_detector_covers_check_signals(self):
        rule = self.corpus["02-no-reconcile-loops"]
        for text in ("meta.SetStatusCondition(&obj.Status.Conditions, cond)",
                     "return ctrl.Result{Requeue: true}, nil"):
            self.assertRegex(text, rule.detector.pattern)

    def test_environment_detector_covers_webhook_receivers(self):
        rule = self.corpus["13-environment-isolation"]
        for text in ("r.client.Get(ctx, key, obj)", "v.Client.List(ctx, &items)"):
            self.assertRegex(text, rule.detector.pattern)

    def test_environment_delete_has_a_check(self):
        rule = self.corpus["13-environment-isolation"]
        self.assertTrue(any(
            "delete" in check.slug and check.applies_to_context("r.Delete(ctx, obj)")
            for check in rule.checks
        ))

    def test_file_judgment_sees_distant_matches(self):
        rule = self.corpus["06-rover-child-labels"]
        lines = ["// unchanged"] * 180
        lines[2] = "SetControllerReference(owner, first, scheme)"
        lines[160] = "SetControllerReference(owner, second, scheme)"
        file = changeset.ChangedFile("rover/internal/handler/example.go", "blob", (3, 161))
        found = detect.run(rule, [file], ROOT, {file.path: "\n".join(lines)})
        self.assertTrue(any("second" in c.context for c in found))

    def test_edit_near_existing_trigger_is_discovered(self):
        rule = self.corpus["06-rover-child-labels"]
        file = changeset.ChangedFile("rover/internal/handler/example.go", "blob", (2,))
        found = detect.run(rule, [file], ROOT, {
            file.path: "child := newChild()\n// label removed here\n"
                       "SetControllerReference(owner, child, scheme)\n",
        })
        self.assertTrue(found)

    def test_distant_edit_does_not_inherit_an_old_trigger(self):
        rule = self.corpus["06-rover-child-labels"]
        lines = ["// unchanged"] * 180
        lines[2] = "SetControllerReference(owner, child, scheme)"
        file = changeset.ChangedFile("rover/internal/handler/example.go", "blob", (180,))
        self.assertEqual(
            detect.run(rule, [file], ROOT, {file.path: "\n".join(lines)}), []
        )

    def test_context_changes_invalidate_cluster_identity(self):
        rule = self.corpus["06-rover-child-labels"]
        first = detect.Candidate(rule.id, "a.go", 1, "same call", "guard present")
        second = replace(first, context="guard absent")
        self.assertNotEqual(first.cluster_key(rule), second.cluster_key(rule))

    def test_selftest_requires_explicit_pass(self):
        plan = {"items": [{
            "item_id": "id", "rule_id": "fixture", "source_rule_id": "rule",
            "expects_fail": False, "sample": 0, "check_id": "check",
        }]}
        result, code = selftest.score(
            ROOT, plan, {"verdicts": [{"item_id": "id", "verdict": "UNCLEAR"}]}, .05
        )
        self.assertEqual(code, 1)
        self.assertEqual(result["rules"]["rule"]["accuracy"], 0)

    def test_selftest_units_are_blind_and_viewable(self):
        plan = selftest.plan(ROOT, [self.corpus["02-no-reconcile-loops"]], 1)
        with tempfile.TemporaryDirectory() as temp:
            path = Path(temp) / "plan.json"
            path.write_text(json.dumps(plan))
            for unit in plan["work_units"]:
                result = run(
                    "show_work_unit.py", ROOT, unit["unit_id"], "--report", str(path)
                )
                self.assertEqual(result.returncode, 0, result.stderr)
                self.assertNotRegex(result.stdout, r"fixture (violation|allowed)|@(violation|allowed)")

    def test_default_viewer_resolves_selftest_plan(self):
        with tempfile.TemporaryDirectory() as temp:
            repo = Path(temp)
            init_repo(repo)
            plan = selftest.plan(ROOT, [self.corpus["02-no-reconcile-loops"]], 1)
            state = repo / ".agents/.audit/selftest"
            state.mkdir(parents=True)
            (state / "plan.json").write_text(json.dumps(plan))
            result = run("show_work_unit.py", repo, plan["work_units"][0]["unit_id"])
            self.assertEqual(result.returncode, 0, result.stderr)


class RangeTest(unittest.TestCase):
    @unittest.skipUnless(shutil.which("pre-commit"), "pre-commit is not installed")
    def test_installed_git_pre_push_hook_blocks_a_terminal_push(self):
        with tempfile.TemporaryDirectory() as temp:
            repo = Path(temp) / "repo"
            init_repo(repo)
            remote = Path(temp) / "remote.git"
            git(repo, "init", "--bare", str(remote))
            git(repo, "remote", "add", "origin", str(remote))
            agents = repo / ".agents"
            (agents / "rules").mkdir(parents=True)
            (agents / "judges.yaml").write_text(JUDGES)
            (agents / "rules/01-no-bad.md").write_text(
                "---\nid: 01-no-bad\ndescription: No BAD text.\nseverity: HIGH\n"
                "complexity: 0\nscope: line\napplies_to:\n  - '*.txt'\n---\n"
                "\n## Detect\n\n```regex\nBAD\n```\n"
            )
            (repo / ".pre-commit-config.yaml").write_text(
                "repos:\n- repo: local\n  hooks:\n  - id: audit-gate-push\n"
                f"    name: audit\n    entry: {sys.executable} {SCRIPTS / 'audit_gate.py'} "
                "--mode range --fail-on HIGH\n"
                "    language: system\n    pass_filenames: false\n"
                "    always_run: true\n    stages: [pre-push]\n"
            )
            git(repo, "add", ".")
            git(repo, "commit", "-qm", "configure")
            git(repo, "update-ref", "refs/remotes/origin/main", "HEAD")
            (repo / "a.txt").write_text("BAD\n")
            git(repo, "add", ".")
            git(repo, "commit", "-qm", "violation")
            installed = subprocess.run(
                ["pre-commit", "install", "--hook-type", "pre-push"],
                cwd=repo, env=fixtures.hermetic_env(), capture_output=True, text=True,
            )
            self.assertEqual(installed.returncode, 0, installed.stderr)
            pushed = subprocess.run(
                ["git", "push", "origin", "HEAD:refs/heads/feature"],
                cwd=repo, env=fixtures.hermetic_env(), capture_output=True, text=True,
            )
            self.assertNotEqual(pushed.returncode, 0)
            self.assertIn("01-no-bad", pushed.stdout + pushed.stderr)
            self.assertIn("Deterministic rule violations", pushed.stdout + pushed.stderr)

    def test_feature_upstream_does_not_replace_target_base(self):
        with tempfile.TemporaryDirectory() as temp:
            repo = Path(temp)
            init_repo(repo)
            base = git(repo, "rev-parse", "HEAD").strip()
            git(repo, "update-ref", "refs/remotes/origin/main", base)
            git(repo, "switch", "-c", "feature")
            (repo / "a.txt").write_text("feature\n")
            git(repo, "add", ".")
            git(repo, "commit", "-qm", "feature")
            git(repo, "branch", "published-tip")
            git(repo, "branch", "--set-upstream-to=published-tip")
            self.assertEqual(changeset.merge_base(repo), base)

    def test_range_gate_reads_committed_blob_despite_unstaged_fix(self):
        with tempfile.TemporaryDirectory() as temp:
            repo = Path(temp)
            init_repo(repo)
            agents = repo / ".agents"
            (agents / "rules").mkdir(parents=True)
            (agents / "judges.yaml").write_text(JUDGES)
            source = ROOT / ".agents/rules/05-docs-no-code-blocks.md"
            (agents / "rules" / source.name).write_text(source.read_text())
            # Copy config-owned tools needed by the rule.
            if (ROOT / ".agents/tools/journey_fences.py").exists():
                (agents / "tools").mkdir()
                (agents / "tools/journey_fences.py").write_text(
                    (ROOT / ".agents/tools/journey_fences.py").read_text()
                )
            doc = repo / "docs/docs/user-journey/a.md"
            doc.parent.mkdir(parents=True)
            doc.write_text("# Guide\n")
            git(repo, "add", ".")
            git(repo, "commit", "-qm", "base")
            git(repo, "update-ref", "refs/remotes/origin/main", "HEAD")
            doc.write_text("# Guide\n\n```\nkind: Rover\n```\n")
            git(repo, "add", ".")
            git(repo, "commit", "-qm", "violation")
            doc.write_text("# Guide\n")
            result = run("audit_gate.py", repo, "--mode", "range", "--fail-on", "HIGH")
            self.assertEqual(result.returncode, 1, result.stdout + result.stderr)


if __name__ == "__main__":
    unittest.main()
