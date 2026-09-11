// SPDX-FileCopyrightText: 2026 Deutsche Telekom AG
//
// SPDX-License-Identifier: Apache-2.0

import assert from "node:assert/strict";
import { mkdtempSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import test from "node:test";

import remarkCrdReference from "../src/plugins/remark-crd-reference.mjs";

function markerTree() {
  return {
    type: "root",
    children: [
      {
        type: "mdxJsxFlowElement",
        name: "CRDSchemaList",
        attributes: [],
        children: [],
      },
    ],
  };
}

test("CRDSchemaList renders schemas from the generated index", () => {
  const dir = mkdtempSync(join(tmpdir(), "crd-schema-list-"));
  const indexPath = join(dir, "index.json");
  writeFileSync(
    indexPath,
    JSON.stringify({
      schemas: [
        {
          group: "rover.cp.ei.telekom.de",
          kind: "AgentSpecification",
          schema:
            "rover.cp.ei.telekom.de/agentspecification_v1.json",
        },
      ],
    }),
  );

  const tree = markerTree();
  remarkCrdReference({ schemaIndexPath: indexPath })(tree);
  const output = JSON.stringify(tree);
  const link = tree.children
    .flatMap((node) => node.children || [])
    .flatMap((node) => node.children || [])
    .flatMap((node) => node.children || [])
    .find((node) => node.type === "link");

  assert.match(output, /Rover/);
  assert.match(output, /AgentSpecification/);
  assert.match(
    output,
    /pathname:\/\/\/schemas\/rover\.cp\.ei\.telekom\.de\/agentspecification_v1\.json/,
  );
  assert.deepEqual(link.children, [
    { type: "inlineCode", value: "agentspecification_v1.json" },
  ]);
});

test("CRDSchemaList rejects malformed schema indexes", () => {
  const dir = mkdtempSync(join(tmpdir(), "crd-schema-list-"));
  const indexPath = join(dir, "index.json");
  writeFileSync(indexPath, JSON.stringify({ schemas: [{}] }));

  assert.throws(
    () => remarkCrdReference({ schemaIndexPath: indexPath })(markerTree()),
    /invalid schema index/i,
  );
});
