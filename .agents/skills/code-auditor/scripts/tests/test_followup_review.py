# Copyright 2026 Deutsche Telekom AG
#
# SPDX-License-Identifier: Apache-2.0

"""Regression tests for the second review of PR 850."""

import shutil
import subprocess
import sys
import tempfile
import unittest
from dataclasses import replace
from pathlib import Path

from test_config_root import (
    JUDGES,
    git, init_repo, run, SCRIPTS,
)
import test_config_root as fixtures
from auditlib import changeset, detect, rules
from auditlib.context import resolve
import create_audit_report as create
import process_audit_report as process

ROOT = SCRIPTS.parents[3]


class FollowupTest(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.repo = Path(self.tmp.name)
        init_repo(self.repo)
        agents = self.repo / ".agents"
        (agents / "rules").mkdir(parents=True)
        (agents / "judges.yaml").write_text(JUDGES)
        (agents / "rules/01-no-bad.md").write_text(
            "---\nid: 01-no-bad\ndescription: No BAD.\nseverity: HIGH\n"
            "complexity: 0\nscope: line\napplies_to:\n  - '**/*.txt'\n---\n"
            "\n## Detect\n\n```regex\nBAD\n```\n"
        )
        git(self.repo, "add", ".")
        git(self.repo, "commit", "-qm", "configuration")
        self.base = git(self.repo, "rev-parse", "HEAD").strip()
        git(self.repo, "update-ref", "refs/remotes/origin/main", self.base)
        self.context = resolve(None, start=self.repo)

    def build(self, mode="staged", **kwargs):
        return create.build(self.context, mode, **kwargs)

    def test_quoted_and_untracked_paths_in_every_scope(self):
        paths = ["café.txt", "a\tb.txt", 'a"b.txt', "a\\b.txt", "a\nb.txt", " a .txt"]
        for path in paths:
            (self.repo / path).write_text("BAD\n")
        self.assertEqual(
            {f.path for f in changeset.collect(self.repo, "local")}, set(paths)
        )
        git(self.repo, "add", ".")
        for mode in ("local", "staged"):
            self.assertEqual(
                {f["path"] for f in self.build(mode)["scope"]["files"]}, set(paths)
            )
        git(self.repo, "commit", "-qm", "quoted paths")
        self.assertEqual(
            {f["path"] for f in self.build("range")["scope"]["files"]}, set(paths)
        )

    def test_pure_rename_is_audited_as_destination_addition(self):
        (self.repo / "old.md").write_text("BAD\n")
        git(self.repo, "add", ".")
        git(self.repo, "commit", "-qm", "old path")
        git(self.repo, "update-ref", "refs/remotes/origin/main", "HEAD")
        git(self.repo, "mv", "old.md", "new.txt")
        for mode in ("local", "staged"):
            document = self.build(mode)
            self.assertEqual([i["verdict"] for i in document["items"]], ["FAIL"])
        git(self.repo, "commit", "-qm", "rename")
        self.assertEqual([i["verdict"] for i in self.build("range")["items"]], ["FAIL"])

    def test_unstaged_waiver_cannot_change_staged_or_committed_audit(self):
        (self.repo / "bad.txt").write_text("BAD\n")
        git(self.repo, "add", "bad.txt")
        ident = self.build()["items"][0]["item_id"]
        (self.repo / ".agents/auditignore").write_text(f"{ident}: human exception\n")
        self.assertEqual(self.build()["items"][0]["verdict"], "FAIL")
        gate = run("audit_gate.py", self.repo, "--fail-on", "HIGH")
        self.assertEqual(gate.returncode, 1, gate.stdout + gate.stderr)
        git(self.repo, "commit", "-qm", "bad committed")
        self.assertEqual(self.build("range")["items"][0]["verdict"], "FAIL")
        gate = run("audit_gate.py", self.repo, "--mode", "range", "--fail-on", "HIGH")
        self.assertEqual(gate.returncode, 1, gate.stdout + gate.stderr)
        self.assertEqual(self.build("local")["stats"]["files"], 1)

    def test_unstaged_rules_judges_and_tools_do_not_change_staged_audit(self):
        tool = self.repo / ".agents/tool.py"
        tool.write_text("import sys\nfor p in sys.argv[1:]: print(p + ':1: BAD')\n")
        rule = self.repo / ".agents/rules/01-no-bad.md"
        rule.write_text(rule.read_text().replace(
            "```regex\nBAD\n```",
            "```tool\ncommand: python3 .agents/tool.py\nviolation_when: output_lines\n```",
        ))
        git(self.repo, "add", ".agents")
        git(self.repo, "commit", "-qm", "tool configuration")
        (self.repo / "bad.txt").write_text("BAD\n")
        git(self.repo, "add", "bad.txt")
        tool.write_text("pass\n")
        rule.write_text(rule.read_text().replace("severity: HIGH", "severity: LOW"))
        (self.repo / ".agents/judges.yaml").write_text("invalid\n")
        document = self.build()
        self.assertEqual(document["items"][0]["verdict"], "FAIL")
        self.assertEqual(document["items"][0]["severity"], "HIGH")
        gate = run("audit_gate.py", self.repo, "--fail-on", "HIGH")
        self.assertEqual(gate.returncode, 1, gate.stdout + gate.stderr)

    def test_index_and_committed_exports_use_raw_blobs(self):
        (self.repo / "bad.txt").write_bytes(b"BAD\n")
        git(self.repo, "add", "bad.txt")
        git(self.repo, "config", "core.autocrlf", "true")
        self.assertEqual(self.build()["items"][0]["verdict"], "FAIL")
        git(self.repo, "commit", "-qm", "raw text")
        (self.repo / ".gitattributes").write_text("bad.txt export-ignore\n")
        git(self.repo, "add", ".gitattributes")
        git(self.repo, "commit", "-qm", "archive attributes")
        self.assertEqual(self.build("range")["items"][0]["verdict"], "FAIL")

    def test_configured_target_is_shared_by_report_and_push_gate(self):
        (self.repo / "bad.txt").write_text("BAD\n")
        git(self.repo, "add", "bad.txt")
        git(self.repo, "commit", "-qm", "release baseline")
        git(self.repo, "branch", "release")
        git(self.repo, "config", "audit.target", "release")
        self.assertEqual(self.build("range")["scope"]["files"], [])
        gate = run("audit_gate.py", self.repo, "--mode", "range", "--fail-on", "HIGH")
        self.assertEqual(gate.returncode, 0, gate.stdout + gate.stderr)
        gate = run(
            "audit_gate.py", self.repo, "--mode", "range",
            "--target", "origin/main", "--fail-on", "HIGH",
        )
        self.assertEqual(gate.returncode, 1, gate.stdout + gate.stderr)

    def test_file_waiver_identity_changes_with_content(self):
        rule = replace(rules.load_rules(self.repo)[0], detector=None)
        file = changeset.ChangedFile("guide.txt", "blob", (1,))
        candidates = [
            detect.run(rule, [file], self.repo, {"guide.txt": text})[0]
            for text in ("first version\n", "second version\n")
        ]
        self.assertNotEqual(candidates[0].normalized_anchor, candidates[1].normalized_anchor)

    def test_push_target_uses_pushed_branch_not_checkout(self):
        (self.repo / "bad.txt").write_text("BAD\n")
        git(self.repo, "add", "bad.txt")
        git(self.repo, "commit", "-qm", "release baseline")
        git(self.repo, "branch", "release")
        git(self.repo, "branch", "feature/topic")
        git(self.repo, "config", "branch.feature/topic.auditTarget", "release")
        tip = git(self.repo, "rev-parse", "HEAD").strip()
        git(self.repo, "switch", "-c", "other", self.base)
        git(self.repo, "config", "branch.other.auditTarget", "origin/main")
        env = {
            "PRE_COMMIT_TO_REF": tip,
            "PRE_COMMIT_LOCAL_BRANCH": "refs/heads/feature/topic",
        }
        gate = run("audit_gate.py", self.repo, "--mode", "range", "--fail-on", "HIGH", env=env)
        self.assertEqual(gate.returncode, 0, gate.stdout + gate.stderr)
        gate = run(
            "audit_gate.py", self.repo, "--mode", "range", "--fail-on", "HIGH",
            "--target", "origin/main", env=env,
        )
        self.assertEqual(gate.returncode, 1, gate.stdout + gate.stderr)
        gate = run(
            "audit_gate.py", self.repo, "--mode", "range", "--fail-on", "HIGH",
            env={**env, "AUDIT_TARGET": "origin/main"},
        )
        self.assertEqual(gate.returncode, 1, gate.stdout + gate.stderr)
        git(self.repo, "switch", "--detach")
        gate = run("audit_gate.py", self.repo, "--mode", "range", "--fail-on", "HIGH", env=env)
        self.assertEqual(gate.returncode, 0, gate.stdout + gate.stderr)

    def test_nonbranch_push_does_not_inherit_checkout_target(self):
        (self.repo / "bad.txt").write_text("BAD\n")
        git(self.repo, "add", "bad.txt")
        git(self.repo, "commit", "-qm", "release baseline")
        tip = git(self.repo, "rev-parse", "HEAD").strip()
        git(self.repo, "branch", "release")
        git(self.repo, "config", "branch.main.auditTarget", "release")
        for source in ("refs/tags/version", "HEAD"):
            gate = run(
                "audit_gate.py", self.repo, "--mode", "range", "--fail-on", "HIGH",
                env={"PRE_COMMIT_TO_REF": tip, "PRE_COMMIT_LOCAL_BRANCH": source},
            )
            self.assertEqual(gate.returncode, 1, gate.stdout + gate.stderr)
        git(self.repo, "config", "audit.target", "release")
        gate = run(
            "audit_gate.py", self.repo, "--mode", "range", "--fail-on", "HIGH",
            env={"PRE_COMMIT_TO_REF": tip, "PRE_COMMIT_LOCAL_BRANCH": "refs/tags/version"},
        )
        self.assertEqual(gate.returncode, 0, gate.stdout + gate.stderr)

    @unittest.skipUnless(shutil.which("pre-commit"), "pre-commit not installed")
    def test_installed_hook_uses_target_of_nonchecked_out_branch(self):
        remote = self.repo / "remote.git"
        git(self.repo, "init", "--bare", str(remote))
        git(self.repo, "remote", "add", "origin", str(remote))
        (self.repo / ".pre-commit-config.yaml").write_text(
            "repos:\n- repo: local\n  hooks:\n  - id: audit-gate-push\n"
            f"    name: audit\n    entry: {sys.executable} {SCRIPTS / 'audit_gate.py'} "
            "--mode range --fail-on HIGH\n"
            "    language: system\n    pass_filenames: false\n"
            "    always_run: true\n    stages: [pre-push]\n"
        )
        (self.repo / "bad.txt").write_text("BAD\n")
        git(self.repo, "add", ".pre-commit-config.yaml", "bad.txt")
        git(self.repo, "commit", "-qm", "release baseline")
        git(self.repo, "branch", "release")
        git(self.repo, "branch", "feature")
        git(self.repo, "config", "branch.feature.auditTarget", "release")
        git(self.repo, "switch", "-c", "other")
        git(self.repo, "config", "branch.other.auditTarget", "origin/main")
        installed = subprocess.run(
            ["pre-commit", "install", "--hook-type", "pre-push"], cwd=self.repo,
            capture_output=True, text=True, env=fixtures.hermetic_env(),
        )
        self.assertEqual(installed.returncode, 0, installed.stdout + installed.stderr)
        pushed = subprocess.run(
            ["git", "push", "origin", "feature:refs/heads/feature"], cwd=self.repo,
            capture_output=True, text=True, env=fixtures.hermetic_env(),
        )
        self.assertEqual(pushed.returncode, 0, pushed.stdout + pushed.stderr)
        self.assertIn("Passed", pushed.stdout + pushed.stderr)
        self.assertEqual(
            git(self.repo, "rev-parse", "feature").strip(),
            git(self.repo, "--git-dir", str(remote), "rev-parse", "refs/heads/feature").strip(),
        )

    def test_repeated_anchors_have_distinct_item_ids(self):
        rule = self.repo / ".agents/rules/01-no-bad.md"
        rule.write_text(rule.read_text().replace("complexity: 0", "complexity: 1") +
                        "\n## Checks\n\n- [bad] Is BAD wrong?\n\n## Examples\n"
                        "\n### Violation\n\n```text source=bad.txt:1@abc1234\nBAD\n```\n")
        (self.repo / "bad.txt").write_text("BAD\nsafe\nBAD\n")
        document = self.build("local")
        self.assertEqual(len(document["items"]), 2)
        self.assertEqual(len({i["item_id"] for i in document["items"]}), 2)
        settled = process.merge(self.repo, document, {"verdicts": [
            {"item_id": i["item_id"], "verdict": "PASS", "evidence": {"reason": "ok"}}
            for i in document["items"]
        ]})
        self.assertEqual(len(settled), 2)

    def test_one_deterministic_waiver_does_not_cover_repeated_anchor(self):
        (self.repo / "bad.txt").write_text("BAD\nsafe\nBAD\n")
        git(self.repo, "add", "bad.txt")
        document = self.build()
        ident = document["items"][0]["item_id"]
        (self.repo / ".agents/auditignore").write_text(f"{ident}: human exception\n")
        git(self.repo, "add", ".agents/auditignore")
        document = self.build()
        self.assertEqual([i["verdict"] for i in document["items"]], ["PASS", "FAIL"])
        gate = run("audit_gate.py", self.repo, "--fail-on", "HIGH")
        self.assertEqual(gate.returncode, 1, gate.stdout + gate.stderr)
        self.assertIn("bad.txt:3", gate.stderr)
        self.assertNotIn("bad.txt:1", gate.stderr)

    def test_evidence_keeps_the_cited_occurrence(self):
        (self.repo / "bad.txt").write_text("BAD\nsafe\nBAD\n")
        git(self.repo, "add", "bad.txt")
        blob = git(self.repo, "rev-parse", ":bad.txt").strip()
        self.assertEqual(process._anchor(self.repo, "bad.txt", 3, "BAD", blob), 3)

    def test_branch_target_overrides_repository_default_and_env_overrides_branch(self):
        git(self.repo, "config", "audit.target", "missing-target")
        git(self.repo, "config", "branch.main.auditTarget", "origin/main")
        self.assertEqual(self.build("range")["base"], self.base)
        gate = run(
            "audit_gate.py", self.repo, "--mode", "range",
            env={"AUDIT_TARGET": "does-not-exist"},
        )
        self.assertEqual(gate.returncode, 2, gate.stdout + gate.stderr)
        self.assertIn("does-not-exist", gate.stderr)

    def test_cached_advisory_remains_advisory(self):
        rule = self.repo / ".agents/rules/01-no-bad.md"
        rule.write_text(
            "---\nid: 01-no-bad\ndescription: No BAD.\nseverity: HIGH\n"
            "complexity: 2\nscope: file\ncontext_lines: 1\n"
            "applies_to:\n  - '*.txt'\n---\n\n## Checks\n\n- [bad] Is BAD wrong?\n"
            "\n## Examples\n\n### Violation\n\n```text source=bad.txt:1@abc1234\nBAD\n```\n"
        )
        (self.repo / "bad.txt").write_text("BAD\n" + "unchanged\n" * 40)
        git(self.repo, "add", ".")
        git(self.repo, "commit", "-qm", "old finding")
        with (self.repo / "bad.txt").open("a") as out:
            out.write("new edit\n")
        first = self.build("local")
        items = process.merge(self.repo, first, {"verdicts": [{
            "item_id": first["items"][0]["item_id"], "verdict": "FAIL",
            "evidence": {"line": 1, "quote": "BAD", "reason": "old violation"},
        }]})
        self.assertFalse(items[0]["in_scope"])
        process.persist(self.context.audit_dir, first, items)
        second = self.build("local")
        self.assertEqual(second["items"][0]["source"], "cache")
        items = process.merge(self.repo, second, {"verdicts": []})
        self.assertFalse(items[0]["in_scope"])


class SharedConfigBindingTest(unittest.TestCase):
    def test_shared_tool_or_data_change_invalidates_receipt(self):
        case = fixtures.SharedConfigTest()
        case.setUp()
        self.addCleanup(case.tearDown)
        git(case.sub, "add", "rover/internal/handler/rover.go")
        case.create("--mode", "staged", "--config-root", str(case.workspace))
        document = case.report()
        items = process.merge(case.sub, document, {"verdicts": [
            {"item_id": i["item_id"], "verdict": "PASS", "evidence": {"reason": "ok"}}
            for i in document["items"]
        ]})
        process.persist(case.state, document, items)
        (case.workspace / ".agents/detector-data.txt").write_text("changed input\n")
        gate = run("audit_gate.py", case.sub, "--config-root", str(case.workspace))
        self.assertEqual(gate.returncode, 1, gate.stdout + gate.stderr)


class ReceiverTest(unittest.TestCase):
    def test_client_operations_are_receiver_independent(self):
        rule = next(r for r in rules.load_rules(ROOT) if r.id == "13-environment-isolation")
        for receiver in ("k8sClient", "rawClient", "r.Client", "v.client"):
            for operation in ("List", "Get", "Create", "Update", "Patch", "Delete"):
                with self.subTest(receiver=receiver, operation=operation):
                    self.assertRegex(f"{receiver}.{operation}(ctx, obj)", rule.detector.pattern)

    def test_isolation_batches_all_receivers_once_per_file(self):
        rule = next(r for r in rules.load_rules(ROOT) if r.id == "13-environment-isolation")
        file = changeset.ChangedFile("organization/internal/example.go", "blob", (1, 2))
        found = detect.run(rule, [file], ROOT, {
            file.path: "k8sClient.List(ctx, list)\nrawClient.Delete(ctx, obj)\n",
        })
        self.assertEqual(len(found), 1)
        self.assertIn("k8sClient.List", found[0].context)
        self.assertIn("rawClient.Delete", found[0].context)
