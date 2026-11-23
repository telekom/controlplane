# Copyright 2026 Deutsche Telekom AG
#
# SPDX-License-Identifier: Apache-2.0

"""Run: python3 -m unittest discover -s .agents/skills/review-architecture/scripts/tests"""

from __future__ import annotations

import json
import os
import subprocess
import sys
import tempfile
import textwrap
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent.parent))

import adr  # noqa: E402

VALID_BODY = """
## Context

Tenants share one cluster.

## Decision

Every object carries an environment label.

```mermaid
flowchart LR
    A --> B
```

## Options considered

- **Labels** (chosen): cheap and uniform.
- **One cluster per tenant**: too expensive to operate.
"""


def frontmatter(**fields) -> str:
    base = {
        "title": '"Environments isolate tenants"',
        "status": "accepted",
        "date": "2026-11-23",
        "origin": "authored",
        "domains": "[gateway]",
        "governs": '["gateway/**/*.go"]',
    }
    base.update(fields)
    lines = [f"{key}: {value}" for key, value in base.items() if value is not None]
    return "---\n" + "\n".join(lines) + "\n---\n"


class RepoTest(unittest.TestCase):
    def setUp(self) -> None:
        self.tmp = tempfile.TemporaryDirectory()
        self.root = Path(self.tmp.name)
        self.git("init", "-q", "-b", "main")
        self.git("config", "user.email", "test@example.com")
        self.git("config", "user.name", "test")
        self.write("gateway/internal/route.go", "package internal\n")
        self.write("docs/docs/architecture/overview.md", "# Overview\n")
        self.commit("init")

    def tearDown(self) -> None:
        self.tmp.cleanup()

    def git(self, *args: str) -> str:
        # A machine-wide trace2 listener may write into .git after a commit and
        # race the temporary directory's cleanup.
        env = {**os.environ, "GIT_TRACE2_EVENT": "0"}
        return subprocess.run(
            ["git", *args], cwd=self.root, check=True, capture_output=True, text=True, env=env
        ).stdout

    def write(self, path: str, text: str) -> None:
        target = self.root / path
        target.parent.mkdir(parents=True, exist_ok=True)
        target.write_text(text, encoding="utf-8")

    def commit(self, message: str) -> None:
        self.git("add", "-A")
        self.git("commit", "-q", "-m", message)

    def write_adr(self, name: str, body: str = VALID_BODY, **fields) -> None:
        self.write(f"docs/docs/decisions/{name}", frontmatter(**fields) + body)

    def errors(self) -> list[str]:
        return adr.validate(self.root).errors


class ValidateTest(RepoTest):
    def test_valid_adr_passes(self) -> None:
        self.write_adr("0001-environments.md")
        self.commit("adr")
        self.assertEqual(self.errors(), [])

    def test_backfilled_adr_needs_resolvable_evidence(self) -> None:
        self.write_adr("0001-environments.md", origin="backfilled")
        self.commit("adr")
        self.assertIn("must cite its evidence", "\n".join(self.errors()))

        self.write_adr(
            "0001-environments.md",
            origin="backfilled",
            evidence=(
                '["docs/docs/architecture/missing.md", "docs/docs/architecture/overview.md#x", '
                '"https://github.com/telekom/controlplane/pull/12"]'
            ),
        )
        self.commit("adr")
        errors = self.errors()
        self.assertEqual(len(errors), 1)
        self.assertIn("missing.md", errors[0])

    def test_governs_must_still_match_a_file(self) -> None:
        self.write_adr("0001-environments.md", governs='["identity/**"]')
        self.commit("adr")
        self.assertIn("matches no tracked file", "\n".join(self.errors()))

    def test_only_mermaid_or_justified_code_blocks(self) -> None:
        body = VALID_BODY.replace("```mermaid", "```yaml")
        self.write_adr("0001-environments.md", body=body)
        self.commit("adr")
        self.assertIn("reserved for mermaid", "\n".join(self.errors()))

        justified = VALID_BODY.replace(
            "```mermaid", "<!-- code-block: the wire format is the decision -->\n```yaml"
        )
        self.write_adr("0001-environments.md", body=justified)
        self.commit("adr")
        self.assertEqual(self.errors(), [])

        detached = justified.replace("-->\n```yaml", "-->\n\n```yaml")
        self.write_adr("0001-environments.md", body=detached)
        self.commit("adr")
        self.assertIn("reserved for mermaid", "\n".join(self.errors()))

    def test_diagram_label_lines_must_fit_their_box(self) -> None:
        long_label = VALID_BODY.replace(
            "    A --> B", '    A["Admin · Notification · Secret Manager"] -->|"creates resources via the API"| B'
        )
        self.write_adr("0001-environments.md", body=long_label)
        self.commit("adr")
        errors = "\n".join(self.errors())
        self.assertIn("'Admin · Notification · Secret Manager' has 37 characters", errors)
        self.assertIn("'creates resources via the API' has 29 characters", errors)

        broken = VALID_BODY.replace(
            "    A --> B",
            '    A["Admin<br/>Notification<br/>Secret Manager"] -->|"creates resources<br/>via the API"| B',
        )
        self.write_adr("0001-environments.md", body=broken)
        self.commit("adr")
        self.assertEqual(self.errors(), [])

    def test_a_decision_needs_an_alternative(self) -> None:
        body = VALID_BODY.replace("- **One cluster per tenant**: too expensive to operate.\n", "")
        self.write_adr("0001-environments.md", body=body)
        self.commit("adr")
        self.assertIn("at least one alternative", "\n".join(self.errors()))

        honest = body + "\nNo alternatives were evaluated. Labels were the initial idea.\n"
        self.write_adr("0001-environments.md", body=honest)
        self.commit("adr")
        self.assertEqual(self.errors(), [])

        self.write_adr("0001-environments.md", body=VALID_BODY + "\nNo alternatives were evaluated.\n")
        self.commit("adr")
        self.assertIn("needs exactly the chosen option", "\n".join(self.errors()))

    def test_supersede_links_must_agree(self) -> None:
        self.write_adr("0001-environments.md", status="superseded", **{"superseded-by": "2"})
        self.write_adr("0002-namespaces.md")
        self.commit("adrs")
        self.assertIn("does not list ADR-0001", "\n".join(self.errors()))

        self.write_adr("0002-namespaces.md", supersedes="[1]")
        self.commit("adrs")
        self.assertEqual(self.errors(), [])

    def test_proposed_successor_leaves_predecessor_in_force(self) -> None:
        self.write_adr("0001-environments.md")
        self.write_adr("0002-namespaces.md", status="proposed", supersedes="[1]")
        self.commit("adrs")
        self.assertEqual(self.errors(), [])

        self.write_adr("0002-namespaces.md", supersedes="[1]")
        self.commit("adrs")
        self.assertIn("must have status superseded", "\n".join(self.errors()))

    def test_new_skeleton_is_unfinished_until_written(self) -> None:
        self.write_adr("0001-environments.md")
        self.write("docs/docs/decisions/0002-broken.md", "no frontmatter\n")
        self.commit("adr")
        path = adr.create(self.root, "Use Kong", ["gateway"], backfilled=False)
        self.assertEqual(path.name, "0003-use-kong.md")
        self.assertIn("unfinished section", "\n".join(self.errors()))


class MatchTest(RepoTest):
    def test_matches_governed_paths_and_skips_retired_adrs(self) -> None:
        self.write_adr("0001-environments.md", status="proposed")
        self.write_adr("0002-old.md", status="deprecated")
        self.commit("adrs")
        found, ungoverned = adr.match(
            adr.load(self.root), adr.expand(self.root, ["gateway", "README.md"])
        )
        self.assertEqual([item["ref"] for item in found], ["ADR-0001"])
        self.assertEqual(found[0]["matched"], ["gateway/internal/route.go"])
        self.assertEqual(ungoverned, ["README.md"])

    def test_deleted_files_count_as_changed(self) -> None:
        base = self.git("rev-parse", "HEAD").strip()
        (self.root / "gateway/internal/route.go").unlink()
        self.assertEqual(
            adr.changed_paths(self.root, "local", base, "HEAD"), ["gateway/internal/route.go"]
        )


class SignalsTest(RepoTest):
    def test_reports_new_modules_dependencies_and_kinds(self) -> None:
        graph = {"gateway": {"path": "gateway", "depends_on": ["common"]}}
        self.write(".github/ci/module-graph.json", json.dumps(graph))
        self.write(
            "gateway/go.mod",
            "module github.com/telekom/controlplane/gateway\n\nrequire (\n"
            "\tgithub.com/telekom/controlplane/common v0.0.0\n\tgithub.com/kong/go-kong v0.1.0\n)\n",
        )
        self.commit("base")
        base = self.git("rev-parse", "HEAD").strip()

        graph["gateway"]["depends_on"].append("identity")
        graph["rover"] = {"path": "rover", "depends_on": []}
        self.write(".github/ci/module-graph.json", json.dumps(graph))
        self.write(
            "gateway/go.mod",
            "module github.com/telekom/controlplane/gateway\n\nrequire (\n"
            "\tgithub.com/telekom/controlplane/common v0.0.0\n\tgithub.com/kong/go-kong v0.2.0\n"
            "\tgithub.com/redis/go-redis v9.0.0\n\tgolang.org/x/sync v0.1.0 // indirect\n)\n",
        )
        self.write(
            "gateway/api/v1/plugin_types.go",
            textwrap.dedent(
                """\
                package v1

                // +kubebuilder:object:root=true
                type Plugin struct{}

                // +kubebuilder:object:root=true
                type PluginList struct{}
                """
            ),
        )
        self.commit("change")

        found = {
            (item["signal"], item["detail"])
            for item in adr.signals(self.root, "range", base, "HEAD")
        }
        self.assertEqual(
            found,
            {
                ("new-module", "rover"),
                ("new-domain-dependency", "gateway now depends on identity"),
                ("new-external-dependency", "github.com/redis/go-redis"),
                ("new-api-kind", "Plugin"),
            },
        )


if __name__ == "__main__":
    unittest.main()
