// SPDX-FileCopyrightText: 2025 Deutsche Telekom AG
//
// SPDX-License-Identifier: Apache-2.0

import { readFileSync, writeFileSync, readdirSync, mkdirSync, existsSync } from "fs";
import { resolve, dirname, join } from "path";
import { fileURLToPath } from "url";
import yaml from "js-yaml";

const __filename = fileURLToPath(import.meta.url);
const __dirname = dirname(__filename);

const REPO_ROOT = resolve(__dirname, "../..");
const OUTPUT_DIR = resolve(__dirname, "../src/data/crds");

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
// Schema parsing helpers
// ─────────────────────────────────────────────────────────────────────────────

/**
 * Recursively extract fields from an OpenAPI v3 properties object.
 *
 * Returns an array of field descriptors:
 * {
 *   name:        string,
 *   type:        string,        // "string", "integer", "boolean", "object", "array", …
 *   description: string | null,
 *   required:    boolean,
 *   default:     any | undefined,
 *   enum:        string[] | undefined,
 *   format:      string | undefined,
 *   constraints: object | undefined, // { minLength, maxLength, minimum, maximum, pattern, minItems, maxItems }
 *   items:       object | undefined, // for arrays: the schema of array items
 *   fields:      Field[] | undefined // nested object fields
 * }
 */
function extractFields(schema, requiredList = []) {
  if (!schema || !schema.properties) return [];

  return Object.entries(schema.properties).map(([name, prop]) => {
    const field = {
      name,
      type: resolveType(prop),
      description: prop.description || null,
      required: requiredList.includes(name),
    };

    // Default value
    if (prop.default !== undefined) {
      field.default = prop.default;
    }

    // Enum values
    if (prop.enum) {
      field.enum = prop.enum;
    }

    // Format (e.g. "email", "uri", "date-time")
    if (prop.format) {
      field.format = prop.format;
    }

    // Constraints
    const constraints = {};
    for (const key of ["minLength", "maxLength", "minimum", "maximum", "pattern", "minItems", "maxItems"]) {
      if (prop[key] !== undefined) constraints[key] = prop[key];
    }
    if (Object.keys(constraints).length > 0) {
      field.constraints = constraints;
    }

    // Nested object fields
    if (prop.properties) {
      field.fields = extractFields(prop, prop.required || []);
    }

    // Array items
    if (prop.type === "array" && prop.items) {
      if (prop.items.properties) {
        // Array of objects → recurse
        field.items = {
          type: "object",
          fields: extractFields(prop.items, prop.items.required || []),
        };
      } else {
        // Array of scalars
        field.items = {
          type: prop.items.type || "string",
        };
      }
    }

    return field;
  });
}

/**
 * Produce a human-friendly type string for a property.
 */
function resolveType(prop) {
  if (prop.type === "array") {
    if (prop.items) {
      if (prop.items.type === "object" || prop.items.properties) return "object[]";
      return `${prop.items.type || "string"}[]`;
    }
    return "array";
  }
  if (prop.type === "object" && prop.additionalProperties) {
    return `map<string, ${prop.additionalProperties.type || "any"}>`;
  }
  return prop.type || "object";
}

// ─────────────────────────────────────────────────────────────────────────────
// Main
// ─────────────────────────────────────────────────────────────────────────────

console.log("Generating CRD data...");

// Ensure output directory exists
mkdirSync(OUTPUT_DIR, { recursive: true });

let totalCrds = 0;

for (const domain of DOMAINS) {
  const crdDir = join(REPO_ROOT, domain, "config", "crd", "bases");

  if (!existsSync(crdDir)) {
    console.warn(`  ⚠ No CRD directory for domain "${domain}" at ${crdDir}`);
    continue;
  }

  const crdFiles = readdirSync(crdDir).filter((f) => f.endsWith(".yaml"));
  if (crdFiles.length === 0) {
    console.warn(`  ⚠ No YAML files in ${crdDir}`);
    continue;
  }

  const domainData = {
    domain,
    crds: [],
  };

  for (const file of crdFiles) {
    const raw = readFileSync(join(crdDir, file), "utf-8");
    const doc = yaml.load(raw);

    const spec = doc.spec;
    const version = spec.versions[0]; // we only use the first (and usually only) version
    const rootSchema = version.schema.openAPIV3Schema;

    const crd = {
      kind: spec.names.kind,
      plural: spec.names.plural,
      singular: spec.names.singular,
      group: spec.group,
      version: version.name,
      scope: spec.scope,
      description: rootSchema.description || null,
    };

    // Extract spec fields
    const specSchema = rootSchema.properties?.spec;
    if (specSchema) {
      crd.spec = {
        description: specSchema.description || null,
        fields: extractFields(specSchema, specSchema.required || []),
      };
    }

    // Extract status fields
    const statusSchema = rootSchema.properties?.status;
    if (statusSchema) {
      crd.status = {
        description: statusSchema.description || null,
        fields: extractFields(statusSchema, statusSchema.required || []),
      };
    }

    domainData.crds.push(crd);
    totalCrds++;
  }

  // Sort CRDs alphabetically by kind within each domain
  domainData.crds.sort((a, b) => a.kind.localeCompare(b.kind));

  const outPath = join(OUTPUT_DIR, `${domain}.json`);
  writeFileSync(outPath, JSON.stringify(domainData, null, 2) + "\n", "utf-8");
  console.log(`  ${domain}: ${domainData.crds.length} CRD(s) → ${domain}.json`);
}

console.log(`Done! Generated data for ${totalCrds} CRDs across ${DOMAINS.length} domains.`);
