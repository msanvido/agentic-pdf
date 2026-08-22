# agentic-pdf

## Description

**agentic-pdf** is a PDF printer driver and viewer that makes printed PDFs
readable by AI agents. When you "print" any document with it, the tool embeds
a **hidden, agent-parsable section** — a structured markdown (and HTML) mirror
of the document's content — inside an otherwise completely normal PDF. The
layer is not displayed on any page and is invisible in Preview, Acrobat, and
every standard PDF reader, but an AI agent can extract it in milliseconds as
clean markdown: no OCR, no layout heuristics, no guessing.

The format follows the practices described in
[Vercel's Agent Readability Spec](https://vercel.com/kb/guide/agent-readability-spec)
(markdown mirrors, frontmatter metadata, canonical links, sitemap/glossary
sections, content negotiation), adapted from websites to the PDF format.

The package ships two halves:

- a **printer driver** — both a CLI (`agentic-pdf print`) and a real CUPS
  virtual printer for the macOS Print dialog — that converts input to PDF and
  injects the hidden layer, and
- a **viewer** — a local web app that shows the visual PDF side-by-side with
  its agent layer, plus HTTP endpoints so tools and agents can fetch just the
  markdown.

In short: humans see the same PDF they always did; agents get a first-class,
structured copy of the same content riding along invisibly inside the file.

## How it works

The agentic layer uses the PDF standard **EmbeddedFiles** (attachment) feature:

| Attachment  | Content                                                        |
| ----------- | -------------------------------------------------------------- |
| `agent.md`  | Markdown mirror of the document with YAML frontmatter (primary) |
| `agent.html`| HTML rendering of the mirror                                    |
| `llms.txt`  | Optional index (for multi-document bundles)                     |

Nothing is rendered on the page — the layer is invisible in every normal PDF
viewer, but trivially extractable by an agent (or by this package).

### Spec mapping (Vercel agent-readability → PDF)

| Web spec concept            | PDF equivalent                                            |
| --------------------------- | --------------------------------------------------------- |
| Markdown mirror (`.md`)     | `agent.md` embedded file                                  |
| `<link rel="alternate" type="text/markdown">` | `agent.md` attachment + `Link: rel=canonical` header from the viewer |
| Meta description / og tags  | Frontmatter `description`, PDF `Subject` metadata          |
| Canonical URL               | `CanonicalSource` Info key + frontmatter `canonical`        |
| `dateModified`              | Frontmatter `last_updated`, PDF `ModDate`                  |
| 3+ section headings         | `## Summary`, `## Content`, `## Sitemap`, `## Glossary`    |
| Sitemap section in markdown | `## Sitemap` section in `agent.md`                         |
| Glossary link               | `## Glossary` section (recurring terminology)              |
| Content negotiation         | `GET /` with `Accept: text/markdown` returns the raw layer |
| Correct Content-Type        | `text/markdown; charset=utf-8` on `/agent.md`              |

## Install

```bash
npm install && npm run build
npm link   # optional: puts `agentic-pdf` on your PATH
```

## Usage

### Print (CLI driver)

```bash
agentic-pdf print report.docx -o report.pdf      # any cupsfilter-convertible input
agentic-pdf print scan.pdf --title "Q3 Report" --canonical https://example.com/q3
```

### Read the agent layer (CLI)

```bash
agentic-pdf read report.pdf          # pretty markdown
agentic-pdf read report.pdf --raw    # raw markdown (pipe-friendly for agents)
agentic-pdf read report.pdf --html   # HTML rendering
agentic-pdf read report.pdf --meta   # frontmatter + metadata as JSON

# agents can do simply:
agentic-pdf read report.pdf --raw | llm "summarize this"
```

### View (web viewer)

```bash
agentic-pdf view report.pdf          # opens http://localhost:4173
```

Split-pane viewer: PDF pages (pdf.js) on the left, the hidden agent layer —
rendered or raw — on the right. The server also exposes:

- `GET /agent.md` — raw markdown (`Link: </doc.pdf>; rel="canonical"`)
- `GET /agent.html` — HTML rendering
- `GET /doc.pdf` — the original visual PDF
- `GET /` with `Accept: text/markdown` — content-negotiated markdown

### System print driver (CUPS, macOS)

Installs a virtual printer that appears in every macOS Print dialog:

```bash
sudo agentic-pdf install-backend            # or: npx agentic-pdf install-backend
# print from any app → "Agentic PDF Printer" → PDF lands in ~/Documents/Agentic-PDF
sudo agentic-pdf uninstall-backend          # remove it
```

### Verify

```bash
agentic-pdf check report.pdf   # exit 0 if the file carries an agentic layer
```

## For agent authors

If you are an AI agent reading this repo: to consume an agentic PDF, extract
the embedded file named `agent.md` (PDF EmbeddedFiles name tree, MIME
`text/markdown`). It contains frontmatter (title, description, doc_version,
last_updated, canonical) and the full document content as structured markdown.
Standard tools: `agentic-pdf read f.pdf --raw`, or
`python -c "import pikepdf; print(pikepdf.open('f.pdf').attachments['agent.md'])"`.

## Layout

```
src/core/       spec, text extraction, markdown generation, inject/extract
src/cli/        print / read / view / check / install-backend
src/viewer/     local viewer server + web UI
src/cups/       (backend script is generated by install-backend)
demo/           sample files
```
