# Copyright 2026 Deutsche Telekom AG
#
# SPDX-License-Identifier: Apache-2.0

"""Run: python3 -m unittest discover -s .agents/skills/review-architecture/scripts/tests"""

from __future__ import annotations

import contextlib
import io
import sys
import tempfile
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent.parent))

import sync_reviewer  # noqa: E402


class SyncReviewerTest(unittest.TestCase):
    def run_sync(self, root: Path, *args: str) -> int:
        with contextlib.redirect_stdout(io.StringIO()), contextlib.redirect_stderr(io.StringIO()):
            return sync_reviewer.main(["--root", str(root), *args])

    def test_check_fails_until_written_and_after_hand_edits(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            self.assertEqual(self.run_sync(root, "--check"), 1)

            self.assertEqual(self.run_sync(root), 0)
            self.assertEqual(self.run_sync(root, "--check"), 0)

            copilot = root / ".github/agents/architecture-reviewer.agent.md"
            copilot.write_text(copilot.read_text(encoding="utf-8") + "edit\n", encoding="utf-8")
            self.assertEqual(self.run_sync(root, "--check"), 1)


if __name__ == "__main__":
    unittest.main()
