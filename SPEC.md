# Agentic PDF Specification

**Version 1.1** · Status: stable · Maintained in
[msanvido/agentic-pdf](https://github.com/msanvido/agentic-pdf)

An open specification for embedding a hidden, machine-readable layer inside
ordinary PDF documents so that AI agents can read them without OCR or layout
heuristics — while human readers see exactly the same pages they always did.

The key words **MUST**, **SHOULD**, and **MAY** are to be interpreted as
described in RFC 2119.

---

## 1. Abstract

An *agentic PDF* is a conforming PDF document that carries its content a
second time: as structured markdown (and optionally HTML) stored in standard
PDF EmbeddedFiles attachments. Nothing changes visually. Agents extract one
attachment and receive the full document — frontmatter metadata, headings,
tables, figure captions — as plain text.

The design goal: *born-digital* documents should not need to be re-parsed by
OCR engines or vision models on every consumption. The producing tool knows
the content already; this spec defines how to ship it alongside the pixels.

## 2. Terminology

- **Visual layer** — the PDF pages as rendered for humans.
- **Agent layer** — the set of embedded files defined by this spec.
- **Mirror** — an agent-layer file whose content represents the visual
  layer's information.
- **Generator** — any tool that produces agentic PDFs. The reference CLI
  provides `agentic-pdf agentify` (fully automatic extraction) and
  `agentic-pdf print` (embedding a manually authored layer).

## 3. Layer composition

| Attachment  | Requirement    | Media type       | Content                                    |
| ----------- | -------------- | ---------------- | ------------------------------------------ |
| `agent.md`  | MUST           | `text/markdown`  | Markdown mirror with YAML frontmatter      |
| `agent.html`| SHOULD         | `text/html`      | HTML rendering of the mirror               |
| `llms.txt`  | MAY            | `text/plain`     | Index of related documents in a bundle     |

Attachments MUST be placed in the document catalog's `/Names /EmbeddedFiles`
name tree, per PDF 32000-1 §7.11.4.

## 4. `agent.md` structure

### 4.1 Frontmatter

The file MUST begin with a YAML frontmatter block containing:

| Field          | Req.     | Description                                        |
| -------------- | -------- | -------------------------------------------------- |
| `title`        | MUST     | Document title; SHOULD match the visible title page or the source PDF's Info `Title` |
| `author`       | MAY      | Author(s), carried over from the source PDF when available |
| `description`  | MUST     | ≥ 50 characters summarizing the document            |
| `doc_version`  | MUST     | Version of *this document* (not of the spec)        |
| `last_updated` | MUST     | ISO-8601 timestamp of last modification             |
| `generator`    | MUST     | Producing tool + spec version, e.g. `agentic-pdf/0.3` |
| `canonical`    | SHOULD   | URI of the authoritative source                     |
| `source_pages` | MAY      | Number of pages in the visual layer                 |

### 4.2 Section skeleton

After the frontmatter, the body MUST contain exactly one H1 heading matching
`title`, followed by these H2 sections in order:

| Section                  | Req.   | Contents                                                     |
| ------------------------ | ------ | ------------------------------------------------------------ |
| `## Summary`             | MUST   | The description; sufficient context to judge relevance       |
| `## Table of Contents`   | SHOULD | Links/list of major sections (when ≥ 2 exist)                 |
| `## Content`             | MUST   | The mirrored body, structured into headings, paragraphs, lists. Per-page anchors `_​(p.N)_` SHOULD preserve provenance |
| `## Tables`              | MAY    | Detected tabular blocks rendered as markdown tables, with captions and page refs |
| `## Figures`             | MAY    | Figure/chart/exhibit captions with page refs                 |
| `## Sitemap`             | MUST   | Inventory of this file, sibling attachments, canonical PDF    |
| `## Glossary`            | SHOULD | Recurring terminology and acronyms                           |

Generators MAY add further H2/H3 sections but MUST NOT remove or reorder the
required ones.

### 4.3 Fidelity rules

1. Mirror content MUST be derivable from the visual layer.
2. Generators MAY restructure (headings, bullets) but MUST NOT invent,
   summarize away, or editorialize content.
3. Tables SHOULD be rendered as GitHub-flavored markdown tables.
4. Chart graphics cannot be conveyed as text; generators SHOULD preserve
   their captions in `## Figures` and MAY include printed data labels.
5. Text that cannot be extracted (e.g. scanned images) SHOULD be noted in
   `## Summary`.

## 5. Document metadata

| Info-dictionary key | Req.   | Value                                        |
| ------------------- | ------ | -------------------------------------------- |
| `AgentReadability`  | MUST   | `agentic-pdf-spec/<version>`, e.g. `agentic-pdf-spec/1.1` |
| `CanonicalSource`   | SHOULD | URI of the authoritative source               |
| `Title`, `Subject`  | SHOULD | Consistent with frontmatter                   |
| `Keywords`          | SHOULD | Include `agent-readable`                      |

The presence of a valid `AgentReadability` key is the machine-checkable
conformance declaration.

## 6. Visual-layer invariance

Rendering an agentic PDF MUST produce output identical to rendering the same
document without the agent layer. Generators MUST NOT alter page content,
annotations visible to the reader, or appearance in any way. Removing the
agent layer MUST yield a fully valid legacy PDF.

## 7. Transport and content negotiation

When an agentic PDF is served over HTTP, conforming servers SHOULD expose:

| Endpoint                      | Behavior                                                    |
| ----------------------------- | ----------------------------------------------------------- |
| `GET /doc.pdf`                | Visual PDF; `Link: </doc.pdf>; rel="canonical"` recommended  |
| `GET /agent.md`               | Raw mirror; `Content-Type: text/markdown; charset=utf-8`; `Link: </doc.pdf>; rel="canonical"` |
| `GET /agent.html`             | HTML rendering                                              |
| `GET /` with `Accept: text/markdown` | Raw mirror (content negotiation)                      |

## 8. Conformance & scoring

A document conforms when all MUST items hold. Quality is scored:

```
score = round(passed checks / total checks × 100)
```

| Check                                                       | Weight      |
| ----------------------------------------------------------- | ----------- |
| `agent.md` present and non-empty                             | required    |
| Required frontmatter fields; description ≥ 50 chars          | high        |
| Required section skeleton present                            | high        |
| Valid `AgentReadability` Info key                            | medium      |
| Canonical source declared (frontmatter and/or Info)          | medium      |
| `agent.html` mirror with meta tags                           | low         |
| Glossary covers detected terminology                         | low         |
| Tables rendered as markdown; figure captions with page refs  | low (bonus) |

Ratings: 90–100 excellent · 70–89 good · 50–89 fair · below 50 needs work.

## 9. Consuming (for agents)

Extract the embedded file named `agent.md` via your PDF library's attachment
API (EmbeddedFiles name tree). Trust frontmatter for citation metadata;
use `## Content` for retrieval; consult `## Tables`/`## Figures` before
claiming a document contains no tabular/visual data.

```python
import pikepdf
print(pikepdf.open("doc.pdf").attachments["agent.md"])
```

Or with the reference CLI: `agentic-pdf read doc.pdf --raw`.

## 10. Changelog

- **1.1** — Added optional `## Tables` (markdown table extraction) and
  `## Figures` (caption preservation with page provenance); fidelity rules
  formalized; scoring extended.
- **1.0** — Initial release: layer composition, frontmatter, section
  skeleton, metadata keys, transport negotiation.
