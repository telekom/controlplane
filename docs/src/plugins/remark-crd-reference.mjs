// SPDX-FileCopyrightText: 2025 Deutsche Telekom AG
//
// SPDX-License-Identifier: Apache-2.0

import fs from "fs";
import path from "path";
import { fileURLToPath } from "url";
import { visit } from "unist-util-visit";

const __dirname = path.dirname(fileURLToPath(import.meta.url));

// ---------------------------------------------------------------------------
// Helper functions
// ---------------------------------------------------------------------------

/** Convert a type/heading name to a Docusaurus-compatible anchor slug. */
function toAnchor(name) {
  return name
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/(^-|-$)/g, "");
}

/** Capitalise the first character of a string. */
function toPascalCase(s) {
  return s.charAt(0).toUpperCase() + s.slice(1);
}

/** Naïve English singularisation used for naming array-item types. */
function singularise(s) {
  if (s.endsWith("ies")) return s.slice(0, -3) + "y";
  if (s.endsWith("ses") || s.endsWith("xes") || s.endsWith("zes"))
    return s.slice(0, -2);
  if (s.endsWith("s") && !s.endsWith("ss")) return s.slice(0, -1);
  return s;
}

/**
 * Try to extract a Go type name from a description string.
 * Matches patterns like "ApplicationSpec defines the desired state …".
 */
function extractTypeName(description) {
  if (!description) return null;
  const m = description.match(
    /^([A-Z][A-Za-z0-9]+)\s+(is|defines|represents|describes|contains|specifies|holds)\b/,
  );
  return m ? m[1] : null;
}

/**
 * Compute a structural hash for a list of fields so that structurally
 * identical types can be de-duplicated.
 */
function structuralHash(fields) {
  if (!fields || fields.length === 0) return "empty";
  const sorted = [...fields].sort((a, b) => a.name.localeCompare(b.name));
  const parts = sorted.map((f) => ({
    n: f.name,
    t: f.type,
    c: f.fields ? structuralHash(f.fields) : null,
    i: f.items && f.items.fields ? structuralHash(f.items.fields) : null,
  }));
  return JSON.stringify(parts);
}

// ---------------------------------------------------------------------------
// Type collector – accumulates extracted types with de-duplication
// ---------------------------------------------------------------------------

function createCollector() {
  return {
    /** Insertion-ordered list of unique types. */
    ordered: [],
    /** hash → type */
    byHash: new Map(),
    /** name → type */
    byName: new Map(),
  };
}

/**
 * Pick a unique display name for a type.  If `preferred` is already taken by
 * a *different* hash, disambiguate first by prepending the CRD kind, then by
 * appending a numeric suffix.
 */
function pickUniqueName(collector, preferred, hash, crdKind) {
  // Exact name already used for the same hash → re-use it.
  const existing = collector.byName.get(preferred);
  if (existing && existing.hash === hash) return preferred;

  // Name is free → take it.
  if (!existing) return preferred;

  // Collision – try prefixing with the CRD kind.
  const prefixed = crdKind + preferred;
  const existingPrefixed = collector.byName.get(prefixed);
  if (!existingPrefixed) return prefixed;
  if (existingPrefixed.hash === hash) return prefixed;

  // Still collides – append a numeric suffix.
  let i = 2;
  while (true) {
    const candidate = prefixed + i;
    const ex = collector.byName.get(candidate);
    if (!ex) return candidate;
    if (ex.hash === hash) return candidate;
    i++;
  }
}

/**
 * Register (or de-duplicate) a type in the collector.
 *
 * Returns the canonical type entry (which may already exist if the hash was
 * seen before).
 */
function registerType(
  collector,
  name,
  description,
  fields,
  hash,
  parentName,
  parentAnchor,
  crdKind,
  origin,
) {
  const dup = collector.byHash.get(hash);
  if (dup) {
    // De-duplicate: just add a new "appears in" reference.
    if (
      parentName &&
      !dup.appearsIn.some(
        (a) => a.name === parentName && a.anchor === parentAnchor,
      )
    ) {
      dup.appearsIn.push({ name: parentName, anchor: parentAnchor });
    }
    return dup;
  }

  const uniqueName = pickUniqueName(collector, name, hash, crdKind);
  const entry = {
    name: uniqueName,
    anchor: toAnchor(uniqueName),
    hash,
    description: description || null,
    fields: fields || [],
    appearsIn: [],
    crdKind,
    origin,
  };
  if (parentName) {
    entry.appearsIn.push({ name: parentName, anchor: parentAnchor });
  }
  collector.ordered.push(entry);
  collector.byHash.set(hash, entry);
  collector.byName.set(uniqueName, entry);
  return entry;
}

// ---------------------------------------------------------------------------
// Recursive type extraction
// ---------------------------------------------------------------------------

/**
 * Walk `fields` and extract nested object / object[] types, registering them
 * in the collector.  Mutates each field to add a `_refType` property that
 * points to the resolved type entry (used later for cross-reference links).
 */
function extractNestedTypes(
  collector,
  fields,
  parentName,
  parentAnchor,
  crdKind,
  origin,
) {
  if (!fields) return;

  for (const field of fields) {
    if (field.type === "object" && field.fields && field.fields.length > 0) {
      const hash = structuralHash(field.fields);
      const preferred =
        extractTypeName(field.description) || toPascalCase(field.name);
      const entry = registerType(
        collector,
        preferred,
        field.description,
        field.fields,
        hash,
        parentName,
        parentAnchor,
        crdKind,
        origin,
      );
      field._refType = entry;
      // Recurse into the nested type's own fields.
      extractNestedTypes(
        collector,
        field.fields,
        entry.name,
        entry.anchor,
        crdKind,
        origin,
      );
    } else if (
      field.type === "object[]" &&
      field.items &&
      field.items.fields &&
      field.items.fields.length > 0
    ) {
      const hash = structuralHash(field.items.fields);
      const preferred =
        extractTypeName(field.description) ||
        toPascalCase(singularise(field.name));
      const entry = registerType(
        collector,
        preferred,
        field.items.description || field.description,
        field.items.fields,
        hash,
        parentName,
        parentAnchor,
        crdKind,
        origin,
      );
      field._refType = entry;
      // Recurse into the array item type's own fields.
      extractNestedTypes(
        collector,
        field.items.fields,
        entry.name,
        entry.anchor,
        crdKind,
        origin,
      );
    }
  }
}

// ---------------------------------------------------------------------------
// View model builder
// ---------------------------------------------------------------------------

/**
 * Build the complete view model for a set of CRDs.
 *
 * @param {object[]} crds – Array of CRD objects from the JSON file.
 * @returns {{ crds: object[], types: object[] }}
 */
function buildViewModel(crds) {
  const collector = createCollector();
  const crdModels = [];

  for (const crd of crds) {
    let specType = null;
    let statusType = null;

    // -- Spec type ----------------------------------------------------------
    if (crd.spec && crd.spec.fields && crd.spec.fields.length > 0) {
      const hash = structuralHash(crd.spec.fields);
      const specName = crd.kind + "Spec";
      specType = registerType(
        collector,
        specName,
        crd.spec.description,
        crd.spec.fields,
        hash,
        crd.kind,
        toAnchor(crd.kind),
        crd.kind,
        "spec",
      );

      extractNestedTypes(
        collector,
        crd.spec.fields,
        specType.name,
        specType.anchor,
        crd.kind,
        "spec",
      );
    }

    // -- Status type --------------------------------------------------------
    if (crd.status && crd.status.fields && crd.status.fields.length > 0) {
      const hash = structuralHash(crd.status.fields);
      const statusName = crd.kind + "Status";
      statusType = registerType(
        collector,
        statusName,
        crd.status.description,
        crd.status.fields,
        hash,
        crd.kind,
        toAnchor(crd.kind),
        crd.kind,
        "status",
      );

      extractNestedTypes(
        collector,
        crd.status.fields,
        statusType.name,
        statusType.anchor,
        crd.kind,
        "status",
      );
    }

    crdModels.push({ crd, specType, statusType });
  }

  return { crds: crdModels, types: collector.ordered };
}

// ---------------------------------------------------------------------------
// Markdown AST node helpers
// ---------------------------------------------------------------------------

/** Escape pipe characters for markdown table cells. */
function escPipe(text) {
  if (!text) return "";
  return String(text).replace(/\|/g, "\\|");
}

/** Sanitise a string for use inside a markdown table cell. */
function cellText(text) {
  if (!text) return "";
  return escPipe(String(text).replace(/\n/g, " "));
}

function mkText(value) {
  return { type: "text", value };
}

function mkInlineCode(value) {
  return { type: "inlineCode", value: String(value) };
}

function mkStrong(children) {
  if (typeof children === "string") {
    children = [mkText(children)];
  }
  return { type: "strong", children };
}

function mkEmphasis(children) {
  if (typeof children === "string") {
    children = [mkText(children)];
  }
  return { type: "emphasis", children };
}

function mkLink(url, children) {
  if (typeof children === "string") {
    children = [mkText(children)];
  }
  return { type: "link", url, children };
}

function mkParagraph(children) {
  return { type: "paragraph", children };
}

function mkHeading(depth, text) {
  return {
    type: "heading",
    depth,
    children: [mkText(text)],
  };
}

function mkTableCell(children) {
  if (!Array.isArray(children)) children = [children];
  return { type: "tableCell", children };
}

function mkTableRow(cells) {
  return { type: "tableRow", children: cells };
}

function mkTable(headerLabels, dataRows) {
  const headerCells = headerLabels.map((label) =>
    mkTableCell(mkText(label)),
  );
  const header = mkTableRow(headerCells);
  return {
    type: "table",
    align: headerLabels.map(() => "left"),
    children: [header, ...dataRows],
  };
}

// ---------------------------------------------------------------------------
// Build AST nodes for types / CRDs
// ---------------------------------------------------------------------------

/**
 * Build the "Validation" cell text for a single field.
 */
function buildValidation(field) {
  const parts = [];
  parts.push(field.required ? "Required" : "Optional");

  if (field.enum && field.enum.length > 0) {
    // Use `\|` inside table cells as pipe separator between enum values.
    parts.push(
      "Enum: " + field.enum.map((v) => escPipe(String(v))).join(" \\| "),
    );
  }

  if (field.format) {
    parts.push("Format: " + escPipe(field.format));
  }

  if (field.constraints) {
    const c = field.constraints;
    if (c.minLength != null) parts.push("minLength: " + c.minLength);
    if (c.maxLength != null) parts.push("maxLength: " + c.maxLength);
    if (c.minimum != null) parts.push("minimum: " + c.minimum);
    if (c.maximum != null) parts.push("maximum: " + c.maximum);
    if (c.pattern != null) parts.push("pattern: " + escPipe(c.pattern));
    if (c.minItems != null) parts.push("minItems: " + c.minItems);
    if (c.maxItems != null) parts.push("maxItems: " + c.maxItems);
  }

  return parts.join(", ");
}

/**
 * Build the AST children for the "Type" cell of a field row.
 */
function buildTypeCell(field) {
  if (field._refType) {
    const ref = field._refType;
    const isArray = field.type === "object[]";
    const linkChildren = [mkEmphasis(ref.name)];
    const nodes = [mkLink("#" + ref.anchor, linkChildren)];
    if (isArray) {
      nodes.push(mkText("[]"));
    }
    return nodes;
  }
  // Plain type (no cross-reference).
  return [mkEmphasis(escPipe(field.type))];
}

/**
 * Build the AST children for the "Default" cell of a field row.
 */
function buildDefaultCell(field) {
  if (field.default != null && field.default !== "") {
    return [mkInlineCode(String(field.default))];
  }
  return [mkText("\u2014")]; // em-dash
}

/**
 * Build a table row for a single field.
 */
function buildFieldRow(field) {
  return mkTableRow([
    mkTableCell(mkInlineCode(field.name)),
    mkTableCell(buildTypeCell(field)),
    mkTableCell(buildDefaultCell(field)),
    mkTableCell(mkText(buildValidation(field))),
  ]);
}

/**
 * Build all AST nodes for a single extracted type section (h4 + appears-in +
 * description + field table).
 */
function buildTypeSection(entry) {
  const nodes = [];

  // h4 heading
  nodes.push(mkHeading(4, entry.name));

  // "Appears in" paragraph
  if (entry.appearsIn.length > 0) {
    const emphChildren = [mkText("Appears in: ")];
    entry.appearsIn.forEach((ref, i) => {
      if (i > 0) emphChildren.push(mkText(", "));
      emphChildren.push(mkLink("#" + ref.anchor, [mkText(ref.name)]));
    });
    nodes.push(mkParagraph([mkEmphasis(emphChildren)]));
  }

  // Description paragraph (only if non-empty and meaningful)
  if (entry.description) {
    const descText = entry.description.replace(/\n/g, " ");
    nodes.push(mkParagraph([mkText(descText)]));
  }

  // Field table
  if (entry.fields && entry.fields.length > 0) {
    const dataRows = entry.fields.map((f) => buildFieldRow(f));
    nodes.push(mkTable(["Field", "Type", "Default", "Validation"], dataRows));
  }

  return nodes;
}

/**
 * Build the "meta" paragraph for a CRD:
 *   **Group:** `group` · **Version:** `version` · **Scope:** scope
 */
function buildMetaParagraph(crd) {
  return mkParagraph([
    mkStrong("Group:"),
    mkText(" "),
    mkInlineCode(crd.group),
    mkText(" \u00B7 "), // middle dot
    mkStrong("Version:"),
    mkText(" "),
    mkInlineCode(crd.version),
    mkText(" \u00B7 "), // middle dot
    mkStrong("Scope:"),
    mkText(" "),
    mkText(crd.scope),
  ]);
}

/**
 * Build the complete list of AST nodes for the entire domain (all CRDs and
 * their extracted types).
 */
function buildAllNodes(viewModel) {
  const nodes = [];
  const renderedHashes = new Set();

  for (const model of viewModel.crds) {
    const { crd, specType, statusType } = model;

    // h3 – CRD kind
    nodes.push(mkHeading(3, crd.kind));

    // Description
    if (crd.description) {
      nodes.push(mkParagraph([mkText(crd.description.replace(/\n/g, " "))]));
    }

    // Meta
    nodes.push(buildMetaParagraph(crd));

    // Collect the types that belong to this CRD in DFS (insertion) order.
    // We iterate the global ordered list and pick those whose crdKind matches.
    // Because extraction is DFS and we processed spec before status, the
    // ordering is already correct.
    const crdTypes = viewModel.types.filter((t) => t.crdKind === crd.kind);

    for (const entry of crdTypes) {
      if (renderedHashes.has(entry.hash)) continue;
      renderedHashes.add(entry.hash);
      nodes.push(...buildTypeSection(entry));
    }
  }

  return nodes;
}

// ---------------------------------------------------------------------------
// Remark plugin
// ---------------------------------------------------------------------------

export default function remarkCrdReference() {
  return (tree) => {
    // We cannot use visit's own splice helpers because we are replacing one
    // node with *multiple* nodes.  Collect replacements, then apply them
    // in reverse index order so that indices remain valid.
    const replacements = [];

    visit(tree, "mdxJsxFlowElement", (node, index, parent) => {
      if (node.name !== "CRDReference") return;
      if (index == null || !parent) return;

      // Extract attributes.
      const domainAttr = (node.attributes || []).find(
        (a) => a.name === "domain",
      );
      const kindAttr = (node.attributes || []).find((a) => a.name === "kind");

      if (!domainAttr || !domainAttr.value) return;

      const domain = domainAttr.value;
      const kindFilter = kindAttr ? kindAttr.value : null;

      // Resolve JSON file path.
      const jsonPath = path.resolve(
        __dirname,
        "../data/crds/",
        domain + ".json",
      );

      if (!fs.existsSync(jsonPath)) {
        // Leave a warning paragraph in place of the JSX node.
        replacements.push({
          parent,
          index,
          nodes: [
            mkParagraph([
              mkEmphasis(
                `CRDReference: data file not found for domain "${domain}".`,
              ),
            ]),
          ],
        });
        return;
      }

      let data;
      try {
        data = JSON.parse(fs.readFileSync(jsonPath, "utf-8"));
      } catch (err) {
        replacements.push({
          parent,
          index,
          nodes: [
            mkParagraph([
              mkEmphasis(
                `CRDReference: failed to parse "${domain}.json": ${err.message}`,
              ),
            ]),
          ],
        });
        return;
      }

      let crds = data.crds || [];

      // Optional kind filter.
      if (kindFilter) {
        crds = crds.filter((c) => c.kind === kindFilter);
      }

      if (crds.length === 0) {
        replacements.push({
          parent,
          index,
          nodes: [
            mkParagraph([
              mkEmphasis(
                `CRDReference: no CRDs found for domain "${domain}"${kindFilter ? ` with kind "${kindFilter}"` : ""}.`,
              ),
            ]),
          ],
        });
        return;
      }

      const viewModel = buildViewModel(crds);
      const newNodes = buildAllNodes(viewModel);

      replacements.push({ parent, index, nodes: newNodes });
    });

    // Apply replacements in reverse order to preserve indices.
    replacements.sort((a, b) => b.index - a.index);
    for (const { parent, index, nodes } of replacements) {
      parent.children.splice(index, 1, ...nodes);
    }
  };
}
