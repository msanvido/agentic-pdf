# agentic-pdf

A PDF printer driver and viewer that makes printed PDFs readable by AI agents.
Printing embeds an **invisible, machine-readable markdown layer** inside an
otherwise completely normal PDF. Humans see exactly what they always saw;
agents extract clean, structured content in milliseconds — no OCR, no layout
heuristics.

The format follows the practices of the
[Vercel agent-readability spec](https://vercel.com/kb/guide/agent-readability-spec)
(markdown mirrors, frontmatter metadata, canonical links, sitemap/glossary
sections), adapted from websites to PDF.

**Live spec guide + hosted viewer:** <https://msanvido.github.io/agentic-pdf/>

## How it works

An agentic PDF is a normal PDF plus standard EmbeddedFiles attachments:

| Attachment   | Status      | Content                                          |
| ------------ | ----------- | ------------------------------------------------ |
| `agent.md`   | required    | Markdown mirror with YAML frontmatter (primary)  |
| `agent.html` | recommended | HTML rendering with meta/OpenGraph tags          |
| `llms.txt`   | optional    | Index for multi-document bundles                 |

Plus Info-dictionary markers (`AgentReadability`, `CanonicalSource`) and
consistent `Title`/`Subject`/`Keywords`. Nothing renders on any page — every
standard viewer and printer output is unchanged, and removing the attachments
yields a valid legacy PDF again.

## Install

Go 1.22+; produces a single static binary (~11 MB) with no runtime dependencies:

```bash
make build            # → bin/agentic-pdf
sudo make install     # or: go install ./cmd/agentic-pdf
make cross            # static builds: darwin/linux/windows × amd64/arm64 → dist/
```

## Usage

### Print

Everything (title, summary, timestamps, canonical source) is extracted
automatically from the raw data:

```bash
agentic-pdf print report.txt -o report.pdf        # any cupsfilter-convertible input
agentic-pdf print scan.pdf --title "Q3 Report"    # PDFs pass through + layer added
agentic-pdf check report.pdf                      # verify the layer (exit 0 = present)
```

### Read the agent layer

```bash
agentic-pdf read report.pdf          # markdown (without frontmatter)
agentic-pdf read report.pdf --raw    # raw markdown — pipe-friendly for agents
agentic-pdf read report.pdf --html   # HTML rendering
agentic-pdf read report.pdf --meta   # metadata + frontmatter as JSON

# agents can simply do:
agentic-pdf read report.pdf --raw | llm "summarize this"
```

### Automatic printing drivers

| Platform | Setup | Workflow |
| --- | --- | --- |
| macOS | `sudo agentic-pdf install-backend` | Any app → Print → *Agentic PDF Printer* → agentic PDF in `~/Documents/Agentic-PDF` |
| Windows | `agentic-pdf install-watch` | Any app → *Microsoft Print to PDF* → save into `Documents\Agentic-Spool` → converted automatically into `Documents\Agentic-PDF`, with a toast notification |
| Linux | `agentic-pdf install-watch` | Drop any PDF into the spool folder; a systemd user service converts it |

Windows does not permit unsigned third-party print drivers, so the watcher
pairs with the built-in *Microsoft Print to PDF* writer — same one-print-step
experience, zero driver signing. The watcher skips files that already carry a
layer, so round-trips are safe. Run it manually on any folder with
`agentic-pdf watch <dir> [--out DIR]`; remove auto-start entries with
`uninstall-watch`.

### View

```bash
agentic-pdf view report.pdf       # http://localhost:4173/?file=doc.pdf
```

Split-pane viewer: visual pages (pdf.js) left, the hidden agent layer —
rendered or raw markdown — right. pdf.js extracts `agent.md` entirely
client-side. The same UI is hosted at
<https://msanvido.github.io/agentic-pdf/viewer/> for checking files without
installing anything.

## Architecture

Single Go binary; two pure-Go dependencies (`pdfcpu`, `ledongthuc/pdf`):

```
cmd/agentic-pdf/      CLI entrypoint
internal/core/        spec constants, text extraction, markdown generation,
                      layer injection/extraction
internal/cli/         commands: print/read/check/watch/installers/notifications
internal/viewer/      viewer server; embeds viewer.html + demo PDFs
docs/                 GitHub Pages site: spec guide + hosted viewer (generated:
                      make sync-viewer)
demo/                 sample input file
```

## For agent authors

To consume an agentic PDF, extract the embedded file named `agent.md`
(PDF EmbeddedFiles name tree, MIME `text/markdown`). It contains frontmatter
(`title`, `description`, `doc_version`, `last_updated`, `canonical`) and the
full document content as structured markdown. Use the frontmatter for citation
metadata and `## Content` for retrieval; treat the visual pages as the
human-facing rendering of the same information.

Without this CLI, any PDF library works, e.g. Python:

```python
import pikepdf
print(pikepdf.open("doc.pdf").attachments["agent.md"])
```
