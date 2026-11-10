# Copyright 2026 Deutsche Telekom AG
#
# SPDX-License-Identifier: Apache-2.0

"""Run: python3 -m unittest discover -s .agents/skills/code-auditor/scripts/tests"""

from __future__ import annotations

import contextlib
import io
import sys
import tempfile
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent.parent))

import sync_judges  # noqa: E402
from auditlib.judges import Judge  # noqa: E402

BODY = "# Persona"


def judges_yaml(**pins: str) -> str:
    lines = ["default: fast"] + [f"{tier}: {pin}" for tier, pin in pins.items()]
    return "\n".join(lines) + "\n"


class TargetsTest(unittest.TestCase):
    root = Path("/repo")

    def names(self, judge: Judge) -> set[str]:
        return {
            str(p.relative_to(self.root))
            for p in sync_judges.targets(self.root, judge, BODY)
        }

    def test_copilot_pin_targets_opencode_and_vscode_only(self) -> None:
        judge = Judge("fast", "github-copilot/gpt-6-luna", "low")
        self.assertEqual(
            self.names(judge),
            {
                ".opencode/agents/audit-judge-fast.md",
                ".github/agents/audit-judge-fast.agent.md",
            },
        )

    def test_anthropic_pin_targets_opencode_and_claude_only(self) -> None:
        judge = Judge("fast", "anthropic/claude-haiku-5-5", "low")
        self.assertEqual(
            self.names(judge),
            {
                ".opencode/agents/audit-judge-fast.md",
                ".claude/agents/audit-judge-fast.md",
            },
        )

    def test_other_provider_targets_opencode_only(self) -> None:
        judge = Judge("fast", "openai/gpt-6", "high")
        self.assertEqual(self.names(judge), {".opencode/agents/audit-judge-fast.md"})

    def test_copilot_definition_maps_model_and_effort(self) -> None:
        judge = Judge("careful", "github-copilot/gpt-6.1-sol", "medium")
        text = sync_judges._copilot(judge, BODY)
        header = text.split("---\n")[1]
        self.assertIn("name: audit-judge-careful\n", header)
        self.assertIn("target: github-copilot\n", header)
        self.assertIn("model: gpt-6.1-sol\n", header)
        self.assertIn("reasoning-effort: medium\n", header)
        self.assertIn("tools: ['execute']\n", header)
        self.assertIn("user-invocable: false\n", header)
        self.assertIn(sync_judges.GENERATED, text)
        self.assertTrue(text.endswith(f"{BODY}\n"))

    def test_copilot_definition_without_effort_omits_it(self) -> None:
        text = sync_judges._copilot(
            Judge("fast", "github-copilot/gpt-6-luna", ""), BODY
        )
        self.assertNotIn("reasoning-effort", text)

    def test_opencode_keeps_full_pin(self) -> None:
        judge = Judge("fast", "github-copilot/gpt-6-luna", "low")
        self.assertIn(
            "model: github-copilot/gpt-6-luna#low\n", sync_judges._opencode(judge, BODY)
        )


class MainTest(unittest.TestCase):
    def setUp(self) -> None:
        self.tmp = tempfile.TemporaryDirectory()
        self.root = Path(self.tmp.name)
        (self.root / ".agents").mkdir()
        self.write_pins(
            fast="github-copilot/gpt-6-luna#low",
            careful="github-copilot/gpt-6.1-sol#medium",
        )

    def tearDown(self) -> None:
        self.tmp.cleanup()

    def write_pins(self, **pins: str) -> None:
        (self.root / ".agents" / "judges.yaml").write_text(judges_yaml(**pins))

    def run_sync(self, *args: str) -> tuple[int, str]:
        err = io.StringIO()
        with contextlib.redirect_stdout(io.StringIO()), contextlib.redirect_stderr(err):
            code = sync_judges.main(["--root", str(self.root), *args])
        return code, err.getvalue()

    def test_write_then_check_is_clean(self) -> None:
        self.assertEqual(self.run_sync("--check")[0], 1)
        self.assertEqual(self.run_sync()[0], 0)
        self.assertEqual(self.run_sync("--check")[0], 0)
        self.assertTrue(
            (self.root / ".github/agents/audit-judge-careful.agent.md").is_file()
        )
        self.assertFalse((self.root / ".claude/agents").exists())

    def test_check_detects_drifted_definition(self) -> None:
        self.run_sync()
        path = self.root / ".github/agents/audit-judge-fast.agent.md"
        path.write_text(path.read_text().replace("gpt-6-luna", "gpt-4o"))
        code, err = self.run_sync("--check")
        self.assertEqual(code, 1)
        self.assertIn(".github/agents/audit-judge-fast.agent.md", err)

    def test_generated_definition_for_incompatible_harness_is_stale(self) -> None:
        self.write_pins(fast="anthropic/claude-haiku-5-5#low")
        self.run_sync()
        claude = self.root / ".claude/agents/audit-judge-fast.md"
        self.assertTrue(claude.is_file())

        self.write_pins(fast="github-copilot/gpt-6-luna#low")
        code, err = self.run_sync("--check")
        self.assertEqual(code, 1)
        self.assertIn(".claude/agents/audit-judge-fast.md (stale)", err)

        self.assertEqual(self.run_sync()[0], 0)
        self.assertFalse(claude.exists())
        self.assertEqual(self.run_sync("--check")[0], 0)

    def test_removed_tier_is_stale(self) -> None:
        self.run_sync()
        self.write_pins(fast="github-copilot/gpt-6-luna#low")
        code, err = self.run_sync("--check")
        self.assertEqual(code, 1)
        self.assertIn(".github/agents/audit-judge-careful.agent.md (stale)", err)
        self.assertIn(".opencode/agents/audit-judge-careful.md (stale)", err)
        self.run_sync()
        self.assertFalse(
            (self.root / ".github/agents/audit-judge-careful.agent.md").exists()
        )

    def test_hand_written_files_are_never_removed(self) -> None:
        hand = [
            self.root / ".claude/agents/audit-judge-mine.md",
            self.root / ".github/agents/audit-judge-mine.agent.md",
            self.root / ".github/agents/reviewer.agent.md",
        ]
        for path in hand:
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_text("---\nname: mine\n---\nhand written\n")
        self.run_sync()
        self.assertEqual(self.run_sync("--check")[0], 0)
        for path in hand:
            self.assertTrue(path.is_file(), path)


if __name__ == "__main__":
    unittest.main()
