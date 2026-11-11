# Copyright 2026 Deutsche Telekom AG
#
# SPDX-License-Identifier: Apache-2.0

"""Run: python3 -m unittest discover -s .agents/skills/code-auditor/scripts/tests"""

from __future__ import annotations

import json
import os
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

SCRIPTS = Path(__file__).resolve().parent.parent
sys.path.insert(0, str(SCRIPTS))

from auditlib import context as contextlib_  # noqa: E402
from auditlib import rules as rulelib  # noqa: E402
from auditlib.store import Waivers  # noqa: E402

JUDGES = "default: fast\nfast: github-copilot/gpt-6-luna#low\n"

COMPLEXITY0_RULE = """---
id: 05-docs-no-code-blocks
description: Journey docs must not hold code blocks.
severity: HIGH
complexity: 0
scope: line
applies_to:
  - "docs/docs/user-journey/**/*.md"
---

# No code blocks

## Detect

```regex
^```(?!mermaid\\s*$)(?!\\s*$).*$
```
"""

COMPLEXITY1_RULE = """---
id: 20-todo-tracked
description: TODOs in Go code must be tracked.
severity: HIGH
complexity: 1
scope: line
applies_to:
  - "**/internal/handler/**/*.go"
---

# TODOs must be tracked

## Detect

```regex
TODO
```

## Checks

- [tracked] Does the TODO reference a ticket?

## Examples

### Violation

```go source=rover/internal/handler/rover.go:3@abc1234
// TODO fix later
```
"""


def hermetic_env(extra: dict | None = None) -> dict:
    """Ignore user git config: signing and trace daemons must not leak in."""
    environment = {k: v for k, v in os.environ.items() if not k.startswith("AUDIT_")}
    environment.update({"GIT_CONFIG_GLOBAL": os.devnull, "GIT_CONFIG_NOSYSTEM": "1"})
    environment.update(extra or {})
    return environment


def git(cwd: Path, *args: str) -> str:
    return subprocess.run(
        ["git", *args],
        cwd=cwd,
        check=True,
        capture_output=True,
        text=True,
        env=hermetic_env(),
    ).stdout


def init_repo(path: Path) -> None:
    path.mkdir(parents=True, exist_ok=True)
    git(path, "init", "-q", "-b", "main")
    git(path, "config", "user.email", "test@example.com")
    git(path, "config", "user.name", "test")
    (path / "README.md").write_text("base\n", encoding="utf-8")
    git(path, "add", "README.md")
    git(path, "commit", "-q", "-m", "base")


def run(script: str, cwd: Path, *args: str, env: dict | None = None):
    return subprocess.run(
        [sys.executable, str(SCRIPTS / script), *args],
        check=False,
        cwd=cwd,
        capture_output=True,
        text=True,
        env=hermetic_env(env),
    )


class SharedConfigTest(unittest.TestCase):
    """A sub-repository audited with the rules of its enclosing workspace."""

    def setUp(self) -> None:
        self._tmp = tempfile.TemporaryDirectory()
        self.workspace = Path(self._tmp.name, "workspace").resolve()
        init_repo(self.workspace)
        agents = self.workspace / ".agents"
        (agents / "rules").mkdir(parents=True)
        (agents / "judges.yaml").write_text(JUDGES, encoding="utf-8")
        (agents / "rules" / "05-docs-no-code-blocks.md").write_text(COMPLEXITY0_RULE, "utf-8")
        (agents / "rules" / "20-todo-tracked.md").write_text(COMPLEXITY1_RULE, "utf-8")

        self.sub = self.workspace / "controlplane"
        init_repo(self.sub)
        doc = self.sub / "docs/docs/user-journey/guide.md"
        doc.parent.mkdir(parents=True)
        doc.write_text("# Guide\n\n```yaml\nkind: Rover\n```\n", encoding="utf-8")
        handler = self.sub / "rover/internal/handler/rover.go"
        handler.parent.mkdir(parents=True)
        handler.write_text("package handler\n\n// TODO fix later\n", encoding="utf-8")
        self.state = agents / ".audit" / "repos" / "controlplane"

    def tearDown(self) -> None:
        self._tmp.cleanup()

    def create(self, *args: str, env: dict | None = None):
        if "--mode" not in args:
            args = ("--mode", "worktree", *args)
        return run("create_audit_report.py", self.sub, "--quiet", *args, env=env)

    def report(self) -> dict:
        return json.loads((self.state / "report.json").read_text(encoding="utf-8"))

    def test_resolve_keeps_repo_and_config_apart(self) -> None:
        ctx = contextlib_.resolve(self.workspace, start=self.sub)
        self.assertEqual(ctx.repo_root, self.sub)
        self.assertEqual(ctx.config_root, self.workspace)
        self.assertEqual(ctx.audit_dir, self.state)

    def test_resolve_defaults_to_the_repository_itself(self) -> None:
        ctx = contextlib_.resolve(None, start=self.sub)
        self.assertEqual(ctx.config_root, self.sub)
        self.assertEqual(ctx.audit_dir, self.sub / ".agents" / ".audit")

    def test_create_uses_workspace_rules_on_subrepo_relative_paths(self) -> None:
        result = self.create("--config-root", str(self.workspace))
        self.assertEqual(result.returncode, 0, result.stderr)

        report = self.report()
        self.assertEqual(report["model"], "github-copilot/gpt-6-luna#low")
        self.assertEqual(
            report["context"],
            {"repo_root": str(self.sub), "config_root": str(self.workspace)},
        )
        self.assertEqual(
            sorted(f["path"] for f in report["scope"]["files"]),
            ["docs/docs/user-journey/guide.md", "rover/internal/handler/rover.go"],
        )
        by_rule = {i["rule_id"]: i for i in report["items"]}
        self.assertEqual(by_rule["05-docs-no-code-blocks"]["verdict"], "FAIL")
        self.assertEqual(
            by_rule["20-todo-tracked"]["file"], "rover/internal/handler/rover.go"
        )
        self.assertEqual(len(report["work_units"]), 1)
        self.assertFalse((self.sub / ".agents").exists())

    def test_environment_variable_selects_the_config_root(self) -> None:
        result = self.create(env={"AUDIT_CONFIG_ROOT": str(self.workspace)})
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertTrue((self.state / "report.json").is_file())

    def test_reports_and_index_use_complexity(self) -> None:
        result = self.create("--config-root", str(self.workspace))
        self.assertEqual(result.returncode, 0, result.stderr)
        index = json.loads((self.state / "rules.index.json").read_text("utf-8"))
        for document in (self.report(), index):
            by_rule = {rule["id"]: rule for rule in document["rules"]}
            self.assertEqual(by_rule["05-docs-no-code-blocks"]["complexity"], 0)
            self.assertEqual(by_rule["20-todo-tracked"]["complexity"], 1)
            self.assertTrue(all("tier" not in rule for rule in by_rule.values()))

    def test_invalid_complexity_is_rejected(self) -> None:
        path = self.workspace / ".agents/rules/05-docs-no-code-blocks.md"
        path.write_text(
            COMPLEXITY0_RULE.replace("complexity: 0", "complexity: 3"), "utf-8"
        )
        with self.assertRaisesRegex(rulelib.RuleError, "complexity must be one of"):
            rulelib.parse_rule(path)

    def test_legacy_tier_is_not_a_complexity(self) -> None:
        path = self.workspace / ".agents/rules/05-docs-no-code-blocks.md"
        path.write_text(COMPLEXITY0_RULE.replace("complexity: 0", "tier: 0"), "utf-8")
        with self.assertRaisesRegex(rulelib.RuleError, "complexity must be one of"):
            rulelib.parse_rule(path)

    def test_shared_auditignore_applies_and_expires_with_the_snippet(self) -> None:
        result = self.create("--config-root", str(self.workspace))
        self.assertEqual(result.returncode, 0, result.stderr)
        item = next(
            i for i in self.report()["items"] if i["rule_id"] == "20-todo-tracked"
        )
        reason = "Accepted: tracked elsewhere (human, 2026-11-11)"
        (self.workspace / ".agents/auditignore").write_text(
            f'{item["item_id"]}: "{reason}"\n', encoding="utf-8"
        )
        result = self.create("--config-root", str(self.workspace))
        self.assertEqual(result.returncode, 0, result.stderr)
        waived = next(
            i for i in self.report()["items"] if i["item_id"] == item["item_id"]
        )
        self.assertEqual(waived["source"], "waiver")
        self.assertEqual(waived["verdict"], "PASS")
        self.assertEqual(waived["evidence"]["reason"], f"waived: {reason}")

        (self.sub / "rover/internal/handler/rover.go").write_text(
            "package handler\n\n// TODO a different task\n", encoding="utf-8"
        )
        result = self.create("--config-root", str(self.workspace))
        self.assertEqual(result.returncode, 0, result.stderr)
        changed = next(
            i for i in self.report()["items"] if i["rule_id"] == "20-todo-tracked"
        )
        self.assertNotEqual(changed["item_id"], item["item_id"])
        self.assertEqual(changed["status"], "pending")

    def test_malformed_auditignore_is_reported_by_validation(self) -> None:
        (self.workspace / ".agents/auditignore").write_text("not a mapping\n", "utf-8")
        waivers = Waivers(self.workspace)
        self.assertTrue(waivers.errors)
        self.assertIn("auditignore", waivers.errors[0])
        result = run("validate_rules.py", self.workspace, "--json")
        self.assertEqual(result.returncode, 1, result.stdout + result.stderr)
        self.assertTrue(
            any(
                "auditignore" in problem
                for problem in json.loads(result.stdout)["problems"]
            )
        )

    def add_spec_detector(self) -> None:
        agents = SCRIPTS.parents[2]
        rule = "rules/09-spec-field-doc-comment.md"
        (self.workspace / ".agents" / rule).write_text(
            (agents / rule).read_text("utf-8"), "utf-8"
        )
        tools = self.workspace / ".agents/tools"
        tools.mkdir()
        (tools / "spec_field_docs.py").write_text(
            (agents / "tools/spec_field_docs.py").read_text("utf-8"), "utf-8"
        )
        path = self.sub / "admin/api/v1/widget_types.go"
        path.parent.mkdir(parents=True)
        path.write_text(
            'package v1\n\ntype WidgetSpec struct {\n\tMode string `json:"mode"`\n}\n',
            "utf-8",
        )

    def test_shared_tool_detector_uses_workspace_script_on_subrepo_files(self) -> None:
        self.add_spec_detector()
        result = self.create("--config-root", str(self.workspace))
        self.assertEqual(result.returncode, 0, result.stderr)
        items = [
            i for i in self.report()["items"]
            if i["rule_id"] == "09-spec-field-doc-comment"
        ]
        self.assertEqual(len(items), 1)
        self.assertEqual(items[0]["file"], "admin/api/v1/widget_types.go")
        self.assertEqual(items[0]["line"], 4)
        self.assertEqual(items[0]["verdict"], "FAIL")

    def test_shared_tool_detector_runs_in_the_gate(self) -> None:
        self.add_spec_detector()
        git(self.sub, "add", "admin/api/v1/widget_types.go")
        result = run(
            "audit_gate.py",
            self.sub,
            "--fail-on",
            "HIGH",
            "--config-root",
            str(self.workspace),
        )
        self.assertEqual(result.returncode, 1, result.stdout + result.stderr)
        self.assertIn("09-spec-field-doc-comment", result.stderr)

    def test_shared_detector_resolves_config_owned_option_values(self) -> None:
        self.add_spec_detector()
        agents = self.workspace / ".agents"
        (agents / "codespell-ignore.txt").write_text("workspace vocabulary", "utf-8")
        (agents / "tools/option_detector.py").write_text(
            "import sys\nfrom pathlib import Path\n"
            "data = Path(sys.argv[1].split('=', 1)[1]).read_text()\n"
            "print(f'{sys.argv[2]}:4: {data}')\n",
            "utf-8",
        )
        path = agents / "rules/09-spec-field-doc-comment.md"
        path.write_text(
            path.read_text("utf-8").replace(
                "python3 .agents/tools/spec_field_docs.py",
                "python3 .agents/tools/option_detector.py "
                "--ignore-words=.agents/codespell-ignore.txt",
            ),
            "utf-8",
        )
        result = self.create("--config-root", str(self.workspace))
        self.assertEqual(result.returncode, 0, result.stderr)
        item = next(
            i for i in self.report()["items"]
            if i["rule_id"] == "09-spec-field-doc-comment"
        )
        self.assertEqual(item["detail"], "workspace vocabulary")

    def test_missing_shared_detector_is_recorded_as_an_engine_error(self) -> None:
        self.add_spec_detector()
        (self.workspace / ".agents/tools/spec_field_docs.py").unlink()
        self.create("--config-root", str(self.workspace))
        self.assertTrue(
            any("spec_field_docs.py" in error for error in self.report()["errors"])
        )
        rule = next(
            rule for rule in self.report()["rules"]
            if rule["id"] == "09-spec-field-doc-comment"
        )
        self.assertEqual(rule["outcome"], "ERROR")

    def test_workspace_changes_stay_out_of_the_subrepo_audit(self) -> None:
        (self.workspace / "outside.go").write_text(
            "// TODO outside\n", encoding="utf-8"
        )
        self.create("--config-root", str(self.workspace))
        paths = {f["path"] for f in self.report()["scope"]["files"]}
        self.assertNotIn("outside.go", paths)

    def test_without_config_root_the_subrepo_has_no_rules(self) -> None:
        result = self.create()
        self.assertEqual(result.returncode, 2)
        self.assertIn(".agents/judges.yaml is missing", result.stderr)

    def test_config_root_without_judges_is_rejected(self) -> None:
        (self.workspace / ".agents" / "judges.yaml").unlink()
        result = self.create("--config-root", str(self.workspace))
        self.assertEqual(result.returncode, 2)
        self.assertIn(".agents/judges.yaml", result.stderr)
        self.assertFalse(self.state.exists())

    def test_config_root_without_rules_is_rejected(self) -> None:
        for path in (self.workspace / ".agents" / "rules").iterdir():
            path.unlink()
        (self.workspace / ".agents" / "rules").rmdir()
        result = self.create("--config-root", str(self.workspace))
        self.assertEqual(result.returncode, 2)
        self.assertIn(".agents/rules", result.stderr)

    def test_config_root_must_enclose_the_repository(self) -> None:
        elsewhere = Path(self._tmp.name, "elsewhere")
        (elsewhere / ".agents" / "rules").mkdir(parents=True)
        (elsewhere / ".agents" / "judges.yaml").write_text(JUDGES, encoding="utf-8")
        result = self.create("--config-root", str(elsewhere))
        self.assertEqual(result.returncode, 2)
        self.assertIn("nor one of its ancestors", result.stderr)

    def test_show_reads_the_shared_report(self) -> None:
        self.create("--config-root", str(self.workspace))
        unit = self.report()["work_units"][0]["unit_id"]
        result = run(
            "show_work_unit.py", self.sub, unit, "--config-root", str(self.workspace)
        )
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn("rover/internal/handler/rover.go", result.stdout)

    def test_process_and_gate_share_receipts_in_the_workspace(self) -> None:
        git(self.sub, "add", "rover/internal/handler/rover.go", "docs/docs/user-journey/guide.md")
        self.create("--config-root", str(self.workspace), "--mode", "staged")
        report = self.report()
        pending = [i for i in report["items"] if i["status"] == "pending"]
        verdicts = {
            "verdicts": [
                {
                    "item_id": i["item_id"],
                    "verdict": "PASS",
                    "evidence": {
                        "line": 3,
                        "quote": "// TODO fix later",
                        "reason": "ok",
                    },
                }
                for i in pending
            ]
        }
        (self.state / "verdicts.json").write_text(
            json.dumps(verdicts), encoding="utf-8"
        )

        result = run(
            "process_audit_report.py", self.sub, "--config-root", str(self.workspace)
        )
        self.assertEqual(result.returncode, 1, result.stdout + result.stderr)
        self.assertIn("05-docs-no-code-blocks", result.stdout)
        self.assertTrue((self.state / "receipts.json").is_file())
        self.assertTrue((self.state / "verdicts.cache.json").is_file())

        gate = run(
            "audit_gate.py",
            self.sub,
            "--fail-on",
            "CRITICAL",
            "--config-root",
            str(self.workspace),
        )
        self.assertEqual(gate.returncode, 0, gate.stderr)

        again = self.create("--config-root", str(self.workspace))
        self.assertEqual(again.returncode, 0, again.stderr)
        sources = {i["rule_id"]: i["source"] for i in self.report()["items"]}
        self.assertEqual(sources["20-todo-tracked"], "cache")

    def test_gate_without_config_root_fails_closed(self) -> None:
        git(self.sub, "add", "rover/internal/handler/rover.go")
        gate = run("audit_gate.py", self.sub)
        self.assertNotEqual(gate.returncode, 0)

    def test_process_rejects_a_report_from_another_context(self) -> None:
        self.create("--config-root", str(self.workspace))
        result = run(
            "process_audit_report.py",
            self.sub,
            "--report",
            str(self.state / "report.json"),
        )
        self.assertEqual(result.returncode, 2)
        self.assertIn("pass the same --config-root", result.stderr)

    def strip_context(self, path: Path) -> None:
        report = json.loads(path.read_text(encoding="utf-8"))
        del report["context"]
        path.write_text(json.dumps(report), encoding="utf-8")

    def test_process_rejects_a_contextless_report_with_shared_config(self) -> None:
        self.create("--config-root", str(self.workspace))
        self.strip_context(self.state / "report.json")
        result = run(
            "process_audit_report.py", self.sub, "--config-root", str(self.workspace)
        )
        self.assertEqual(result.returncode, 2)
        self.assertIn("records no context", result.stderr)
        self.assertFalse((self.state / "receipts.json").exists())

    def test_process_accepts_a_contextless_report_in_local_mode(self) -> None:
        local = self.workspace / ".agents" / ".audit"
        created = run("create_audit_report.py", self.workspace, "--mode", "local", "--quiet")
        self.assertEqual(created.returncode, 0, created.stderr)
        self.strip_context(local / "report.json")
        result = run("process_audit_report.py", self.workspace)
        self.assertNotIn("Audit rejected", result.stderr)
        self.assertIn(result.returncode, (0, 1), result.stderr)
        self.assertTrue((local / "receipts.json").is_file())


if __name__ == "__main__":
    unittest.main()
