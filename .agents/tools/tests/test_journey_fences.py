# Copyright 2026 Deutsche Telekom AG
#
# SPDX-License-Identifier: Apache-2.0

import sys
import tempfile
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))
import journey_fences


class JourneyFencesTest(unittest.TestCase):
    def test_openers_only_with_mermaid_and_nested_fences(self):
        text = (
            "```\nyaml: value\n```\n"
            "```mermaid\nflowchart TD\n```\n"
            "~~~~shell\n```\n~~~~\n"
            "   ````go\n```\n````\n"
        )
        with tempfile.TemporaryDirectory() as temp:
            path = Path(temp) / "guide.md"
            path.write_text(text)
            self.assertEqual(
                [int(line.split(":")[1]) for line in journey_fences.report(path)],
                [1, 7, 10],
            )

    def test_mermaid_text_outside_fence_does_not_allow_bare_fence(self):
        with tempfile.TemporaryDirectory() as temp:
            path = Path(temp) / "guide.md"
            path.write_text("mermaid\n```\nnot a diagram\n```\n")
            self.assertEqual(len(journey_fences.report(path)), 1)
