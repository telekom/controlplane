# Copyright 2026 Deutsche Telekom AG
#
# SPDX-License-Identifier: Apache-2.0

import sys
import tempfile
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))
import similar_tests

COPY = '''
		It("{title}", func() {{
			_, lk1 := cachekeys.{fn}("/x/v1", "team-a")
			_, lk2 := cachekeys.{fn}("/x/v1", "team-b")
			Expect(lk1).NotTo(Equal(lk2))
		}})
'''

DIFFERENT = '''
		It("deletes the realm", func() {
			realm := newValidRealm()
			handler := NewHandlerRealm(unusedFactory)
			Expect(handler.Delete(ctx, realm)).To(Succeed())
			Expect(mockK8s.AssertExpectations(GinkgoT())).To(BeTrue())
		})
'''


class SimilarTestsTest(unittest.TestCase):
    def lines(self, text):
        with tempfile.TemporaryDirectory() as temp:
            path = Path(temp) / "x_test.go"
            path.write_text(text)
            return [f["line"] for f in similar_tests.findings(path)]

    def test_copies_differing_in_function_and_literals_are_reported(self):
        text = COPY.format(title="a", fn="McpServer") + COPY.format(title="b", fn="AgentCard")
        self.assertEqual(self.lines(text), [2, 8])

    def test_distinct_tests_are_not_reported(self):
        self.assertEqual(self.lines(COPY.format(title="a", fn="McpServer") + DIFFERENT), [])


if __name__ == "__main__":
    unittest.main()
