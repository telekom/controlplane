# Copyright 2026 Deutsche Telekom AG
#
# SPDX-License-Identifier: Apache-2.0

"""Regression coverage for remaining audit correctness findings."""

import json
import shutil
import subprocess
import sys
import tempfile
import unittest
from dataclasses import replace
from pathlib import Path

import test_config_root as fixtures
from auditlib import changeset, detect, provenance, rules, store
from auditlib.context import resolve
import create_audit_report as create
import process_audit_report as process

ROOT = fixtures.SCRIPTS.parents[3]


class RemainingReviewTest(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.repo = Path(self.tmp.name)
        fixtures.init_repo(self.repo)
        self.corpus = {r.id: r for r in rules.load_rules(ROOT)}

    def test_isolation_discovers_alternate_context_multiline_and_status(self):
        rule = self.corpus["13-environment-isolation"]
        for source, check in (
            ("raw.List(requestCtx, list)", "unscoped-list"),
            ("raw.List(\n requestCtx,\n list)", "unscoped-list"),
            ("raw.Status().Update(requestCtx, obj)", "unlabelled-write"),
            ("raw.Status().Patch(\n requestCtx, obj, patch)", "unlabelled-write"),
            ("raw.Get(otherCtx, key, obj)", "unscoped-get"),
        ):
            with self.subTest(source=source):
                file = changeset.ChangedFile(
                    "organization/internal/example.go", "blob",
                    tuple(range(1, len(source.splitlines()) + 1)),
                )
                found = detect.run(rule, [file], self.repo, {file.path: source})
                self.assertEqual(len(found), 1)
                self.assertTrue(next(c for c in rule.checks if c.slug == check)
                                .applies_to_context(found[0].context))

    def test_tool_paths_preserved_for_builtin_detectors(self):
        for rule_id, suffix, body in (
            ("05-docs-no-code-blocks", ".md", "```yaml\nkind: Bad\n```\n"),
            ("09-spec-field-doc-comment", "_types.go",
             'package v1\n\ntype WidgetSpec struct {\n\tMode string `json:"mode"`\n}\n'),
            ("10-spec-field-doc-purpose", "_types.go",
             'package v1\n\ntype WidgetSpec struct {\n'
             '\t// Mode configuration\n\tMode string `json:"mode"`\n}\n'),
        ):
            rule = self.corpus[rule_id]
            for name in ("example:notes", "example\nnotes", " example", 'a"b', "café"):
                with self.subTest(rule=rule_id, name=name):
                    folder = (
                        "docs/docs/user-journey" if suffix == ".md" else "admin/api/v1"
                    )
                    path = f"{folder}/{name}{suffix}"
                    target = self.repo / path
                    target.parent.mkdir(parents=True, exist_ok=True)
                    target.write_text(body)
                    file = changeset.ChangedFile(path, "blob", tuple(range(1, 6)))
                    found = detect.run(rule, [file], self.repo, {path: body})
                    self.assertEqual(len(found), 1)
                    self.assertEqual(found[0].file, path)
                    target.unlink()

    def test_malformed_tool_output_is_an_engine_error(self):
        tool = self.repo / "tool.py"
        rule = replace(
            self.corpus["05-docs-no-code-blocks"],
            detector=rules.Detector(
                kind="tool", command=f"{sys.executable} {tool}",
            ),
        )
        file = changeset.ChangedFile("docs/docs/user-journey/test.md", "blob", (1,))
        for output in ("not a finding", "unknown.md:1: bad", f"{file.path}:999: bad"):
            with self.subTest(output=output):
                tool.write_text(f"print({output!r})\n")
                with self.assertRaises(detect.ToolUnavailable):
                    detect.run(rule, [file], self.repo, {file.path: "bad\n"})

    def test_invalid_json_records_fail_closed(self):
        tool = self.repo / "tool.py"
        rule = replace(
            self.corpus["05-docs-no-code-blocks"],
            detector=rules.Detector(
                kind="tool", command=f"{sys.executable} {tool}", output_format="jsonl",
            ),
        )
        path = "docs/docs/user-journey/test.md"
        file = changeset.ChangedFile(path, "blob", (1,))
        valid = {"file": path, "line": 1, "detail": "bad"}
        for record in (
            [], {}, {**valid, "file": "unknown"},
            {**valid, "line": True}, {**valid, "line": 2},
            {**valid, "span": [0, 1]}, {**valid, "span": [1, 2]},
            {**valid, "detail": []},
        ):
            with self.subTest(record=record):
                tool.write_text(f"print({json.dumps(record)!r})\n")
                with self.assertRaises(detect.ToolUnavailable):
                    detect.run(rule, [file], self.repo, {path: "bad\n"})

    def test_unusual_filename_blocks_staged_and_committed_gates(self):
        agents = self.repo / ".agents"
        (agents / "rules").mkdir(parents=True)
        (agents / "tools").mkdir()
        shutil.copyfile(
            self.corpus["05-docs-no-code-blocks"].path,
            agents / "rules/05-docs-no-code-blocks.md",
        )
        shutil.copyfile(ROOT / ".agents/tools/journey_fences.py", agents / "tools/journey_fences.py")
        (agents / "judges.yaml").write_text(fixtures.JUDGES)
        fixtures.git(self.repo, "add", ".")
        fixtures.git(self.repo, "commit", "-qm", "config")
        fixtures.git(self.repo, "update-ref", "refs/remotes/origin/main", "HEAD")
        for name in ("example:notes.md", "example\nnotes.md", " example.md"):
            path = self.repo / "docs/docs/user-journey" / name
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_text("> ```yaml\n> bad\n> ```\n")
        fixtures.git(self.repo, "add", ".")
        gate = fixtures.run("audit_gate.py", self.repo, "--fail-on", "HIGH")
        self.assertEqual(gate.returncode, 1, gate.stdout + gate.stderr)
        self.assertEqual(gate.stderr.count("[HIGH]"), 3)
        fixtures.git(self.repo, "commit", "-qm", "governed docs")
        gate = fixtures.run(
            "audit_gate.py", self.repo, "--mode", "range", "--fail-on", "HIGH",
        )
        self.assertEqual(gate.returncode, 1, gate.stdout + gate.stderr)
        self.assertEqual(gate.stderr.count("[HIGH]"), 3)

    def test_whole_file_documentation_reports_only_marked_lines(self):
        rule = self.corpus["04-over-technical-docs"]
        prompt = {"context_lines": rule.context_lines}
        if hasattr(rule, "report_radius"):
            prompt["report_radius"] = rule.report_radius
        args = (
            {"rule_id": rule.id, "file": "guide.md"},
            {rule.id: rule.report_scope}, {"guide.md": {150}}, {rule.id: prompt},
        )
        self.assertFalse(process._finding_in_scope(args[0], 1, *args[1:]))
        self.assertTrue(process._finding_in_scope(args[0], 150, *args[1:]))

    def test_unrelated_documentation_finding_stays_advisory_through_cache_and_gate(self):
        agents = self.repo / ".agents"
        (agents / "rules").mkdir(parents=True)
        shutil.copyfile(
            self.corpus["04-over-technical-docs"].path,
            agents / "rules/04-over-technical-docs.md",
        )
        (agents / "judges.yaml").write_text(fixtures.JUDGES)
        path = self.repo / "docs/docs/user-journey/guide.md"
        path.parent.mkdir(parents=True)
        path.write_text("Old jargon\n" + "unchanged\n" * 149)
        fixtures.git(self.repo, "add", ".")
        fixtures.git(self.repo, "commit", "-qm", "old documentation")
        with path.open("a") as out:
            out.write("New paragraph\n")
        fixtures.git(self.repo, "add", ".")
        context = resolve(None, start=self.repo)
        document = create.build(context, "staged")
        self.assertIn("Old jargon", document["items"][0]["context"])
        items = process.merge(self.repo, document, {"verdicts": [
            {"item_id": item["item_id"], "verdict": "FAIL",
             "evidence": {"line": 1, "quote": "Old jargon", "reason": "old issue"}}
            for item in document["items"]
        ]})
        self.assertTrue(all(not item["in_scope"] for item in items))
        process.persist(context.audit_dir, document, items)
        document = create.build(context, "staged")
        self.assertTrue(all(item["source"] == "cache" for item in document["items"]))
        items = process.merge(self.repo, document, {"verdicts": []})
        self.assertTrue(all(not item["in_scope"] for item in items))
        process.persist(context.audit_dir, document, items)
        gate = fixtures.run("audit_gate.py", self.repo, "--fail-on", "HIGH")
        self.assertEqual(gate.returncode, 0, gate.stdout + gate.stderr)

    def test_provenance_requires_exact_normalized_lines(self):
        (self.repo / "example.go").write_text("return nil, err\n")
        fixtures.git(self.repo, "add", ".")
        fixtures.git(self.repo, "commit", "-qm", "source")
        rev = fixtures.git(self.repo, "rev-parse", "HEAD").strip()
        source = provenance.Provenance("example.go", 1, rev)
        self.assertIsNotNone(provenance.verify(self.repo, source, "return nil"))
        self.assertIsNone(provenance.verify(self.repo, source, "  return   nil, err"))

    def test_waivers_require_nonempty_string_reason(self):
        (self.repo / ".agents").mkdir()
        for value in ("", "[]", "false", "true", "123", '""', '"   "'):
            with self.subTest(value=value):
                (self.repo / ".agents/auditignore").write_text(f"abc123: {value}\n")
                waiver = store.Waivers(self.repo)
                self.assertTrue(waiver.errors)
                self.assertIsNone(waiver.get("abc123"))
        (self.repo / ".agents/auditignore").write_text('abc123: "approved exception"\n')
        self.assertFalse(store.Waivers(self.repo).errors)

    def test_shared_viewer_command_is_explicit_in_persona_and_skill(self):
        persona = (ROOT / ".agents/skills/code-auditor/persona/README.md").read_text()
        skill = (ROOT / ".agents/skills/code-auditor/SKILL.md").read_text()
        self.assertIn("$VIEWER_PATH", persona)
        self.assertIn('--config-root "$CONFIG_ROOT"', persona)
        self.assertIn("VIEWER_PATH", skill)
        self.assertIn("CONFIG_ROOT", skill)

    def test_resolved_viewer_runs_from_submodule(self):
        case = fixtures.SharedConfigTest()
        case.setUp()
        self.addCleanup(case.tearDown)
        viewer_dir = case.workspace / ".agents/skills/code-auditor/scripts"
        shutil.copytree(
            fixtures.SCRIPTS, viewer_dir,
            ignore=shutil.ignore_patterns("tests", "__pycache__"),
        )
        created = case.create("--config-root", str(case.workspace))
        self.assertEqual(created.returncode, 0, created.stderr)
        unit = case.report()["work_units"][0]["unit_id"]
        viewed = subprocess.run(
            [sys.executable, str(viewer_dir / "show_work_unit.py"), unit,
             "--config-root", str(case.workspace)],
            cwd=case.sub, capture_output=True, text=True, env=fixtures.hermetic_env(),
        )
        self.assertEqual(viewed.returncode, 0, viewed.stdout + viewed.stderr)
        self.assertIn(unit, viewed.stdout)

    @unittest.skipUnless(shutil.which("codespell"), "codespell not installed")
    def test_spelling_tool_preserves_unusual_filenames(self):
        rule = self.corpus["03-spelling"]
        for name in ("example:notes.md", "example\nnotes.md", " example.md"):
            with self.subTest(name=name):
                (self.repo / name).write_text("teh\n")
                file = changeset.ChangedFile(name, "blob", (1,))
                found = detect.run(rule, [file], self.repo, {name: "teh\n"})
                self.assertEqual(len(found), 1)
                self.assertEqual(found[0].file, name)
