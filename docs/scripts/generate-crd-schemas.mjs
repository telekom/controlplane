// SPDX-FileCopyrightText: 2025 Deutsche Telekom AG
//
// SPDX-License-Identifier: Apache-2.0

import {
  readFileSync,
  writeFileSync,
  readdirSync,
  mkdirSync,
  existsSync,
} from "fs";
import { resolve, dirname, join } from "path";
import { fileURLToPath } from "url";
import yaml from "js-yaml";

const __filename = fileURLToPath(import.meta.url);
const __dirname = dirname(__filename);

const REPO_ROOT = resolve(__dirname, "../..");
const OUTPUT_DIR = resolve(__dirname, "../static/schemas");
const BASE_URL = "https://telekom.github.io/controlplane/schemas";

/**
 * All known domains and the folder name that contains their CRDs.
 * The key is the domain name used in the docs, the value is the directory name
 * relative to the repository root.
 */
const DOMAINS = [
  "admin",
  "api",
  "application",
  "approval",
  "event",
  "gateway",
  "identity",
  "notification",
  "organization",
  "pubsub",
  "rover",
];

// ─────────────────────────────────────────────────────────────────────────────
// ObjectMeta schema injected into the "metadata" field for richer autocomplete
// ─────────────────────────────────────────────────────────────────────────────

const OBJECT_META_SCHEMA = {
  type: "object",
  description:
    "Standard Kubernetes object metadata. See https://kubernetes.io/docs/reference/kubernetes-api/common-definitions/object-meta/",
  properties: {
    name: {
      type: "string",
      description:
        "Name must be unique within a namespace. Is required when creating resources.",
    },
    namespace: {
      type: "string",
      description:
        "Namespace defines the space within which each name must be unique.",
    },
    labels: {
      type: "object",
      description:
        "Map of string keys and values that can be used to organize and categorize objects.",
      additionalProperties: {
        type: "string",
      },
    },
    annotations: {
      type: "object",
      description:
        "Annotations is an unstructured key value map stored with a resource that may be set by external tools.",
      additionalProperties: {
        type: "string",
      },
    },
    generateName: {
      type: "string",
      description:
        "GenerateName is an optional prefix, used by the server, to generate a unique name only if the Name field has not been provided.",
    },
  },
};

// ─────────────────────────────────────────────────────────────────────────────
// Conversion helpers
// ─────────────────────────────────────────────────────────────────────────────

/**
 * Convert an OpenAPI v3 schema (from a CRD YAML) into a standalone JSON Schema
 * document (draft-07).
 *
 * @param {object} openApiSchema - The openAPIV3Schema object from the CRD
 * @param {string} group         - API group (e.g. "admin.cp.ei.telekom.de")
 * @param {string} kind          - CRD kind (e.g. "Environment")
 * @param {string} version       - API version (e.g. "v1")
 * @returns {object} A valid JSON Schema draft-07 document
 */
function toJsonSchema(openApiSchema, group, kind, version) {
  // Deep-clone to avoid mutating the original parsed object
  const schema = structuredClone(openApiSchema);

  const lowercaseKind = kind.toLowerCase();
  const schemaId = `${BASE_URL}/${group}/${lowercaseKind}_${version}.json`;

  // Add JSON Schema envelope
  schema["$schema"] = "http://json-schema.org/draft-07/schema#";
  schema["$id"] = schemaId;
  schema.title = kind;

  // Pin apiVersion to the exact value
  if (schema.properties?.apiVersion) {
    schema.properties.apiVersion = {
      type: "string",
      description: `Must be "${group}/${version}".`,
      const: `${group}/${version}`,
    };
  }

  // Pin kind to the exact value
  if (schema.properties?.kind) {
    schema.properties.kind = {
      type: "string",
      description: `Must be "${kind}".`,
      const: kind,
    };
  }

  // Enrich metadata with ObjectMeta fields
  if (schema.properties?.metadata) {
    schema.properties.metadata = structuredClone(OBJECT_META_SCHEMA);
  }

  // Ensure the top-level required list includes apiVersion, kind, metadata
  if (!schema.required) {
    schema.required = [];
  }
  for (const field of ["apiVersion", "kind", "metadata"]) {
    if (!schema.required.includes(field)) {
      schema.required.push(field);
    }
  }

  return schema;
}

// ─────────────────────────────────────────────────────────────────────────────
// Main
// ─────────────────────────────────────────────────────────────────────────────

console.log("Generating CRD JSON Schemas...");

// Ensure root output directory exists
mkdirSync(OUTPUT_DIR, { recursive: true });

let totalSchemas = 0;
const index = [];

for (const domain of DOMAINS) {
  const crdDir = join(REPO_ROOT, domain, "config", "crd", "bases");

  if (!existsSync(crdDir)) {
    console.warn(`  Warning: No CRD directory for domain "${domain}" at ${crdDir}`);
    continue;
  }

  const crdFiles = readdirSync(crdDir).filter((f) => f.endsWith(".yaml"));
  if (crdFiles.length === 0) {
    console.warn(`  Warning: No YAML files in ${crdDir}`);
    continue;
  }

  for (const file of crdFiles) {
    const raw = readFileSync(join(crdDir, file), "utf-8");
    const doc = yaml.load(raw);

    const spec = doc.spec;
    const group = spec.group;

    // Process each version (typically only v1)
    for (const version of spec.versions) {
      const versionName = version.name;
      const openApiSchema = version.schema?.openAPIV3Schema;

      if (!openApiSchema) {
        console.warn(
          `  Warning: No openAPIV3Schema for ${spec.names.kind} ${versionName} in ${file}`,
        );
        continue;
      }

      const kind = spec.names.kind;
      const lowercaseKind = kind.toLowerCase();
      const jsonSchema = toJsonSchema(openApiSchema, group, kind, versionName);

      // Ensure group subdirectory exists
      const groupDir = join(OUTPUT_DIR, group);
      mkdirSync(groupDir, { recursive: true });

      // Write the schema file
      const fileName = `${lowercaseKind}_${versionName}.json`;
      const outPath = join(groupDir, fileName);
      writeFileSync(outPath, JSON.stringify(jsonSchema, null, 2) + "\n", "utf-8");

      // Track for the index
      index.push({
        group,
        version: versionName,
        kind,
        plural: spec.names.plural,
        scope: spec.scope,
        description: openApiSchema.description || null,
        schema: `${group}/${fileName}`,
        url: `${BASE_URL}/${group}/${fileName}`,
      });

      totalSchemas++;
    }
  }
}

// Sort index by group, then kind
index.sort((a, b) => {
  const groupCmp = a.group.localeCompare(b.group);
  if (groupCmp !== 0) return groupCmp;
  return a.kind.localeCompare(b.kind);
});

// Write index manifest
const indexDoc = {
  $schema: "http://json-schema.org/draft-07/schema#",
  title: "Control Plane CRD Schema Index",
  description:
    "Index of all available JSON Schemas for Control Plane Custom Resource Definitions.",
  generatedAt: new Date().toISOString(),
  baseUrl: BASE_URL,
  schemas: index,
};

writeFileSync(
  join(OUTPUT_DIR, "index.json"),
  JSON.stringify(indexDoc, null, 2) + "\n",
  "utf-8",
);

console.log(
  `Done! Generated ${totalSchemas} JSON Schema(s) across ${DOMAINS.length} domains.`,
);
console.log(`  Index written to static/schemas/index.json`);
