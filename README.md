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
**Normative specification:** [SPEC.md](SPEC.md)

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
automatically from the raw data. Sample inputs live in [`docs/demo/`](docs/demo/):

```bash
agentic-pdf print docs/demo/report.txt -o report.pdf    # rich 2-page business report
agentic-pdf print docs/demo/memo.txt -o memo.pdf        # internal memo
agentic-pdf print docs/demo/fed-monetary-policy-report-july-2026.pdf   # real 77-page Fed report (charts)
agentic-pdf print docs/demo/sample.txt --title "Q3 Report" --canonical https://example.com/q3
agentic-pdf check report.pdf                       # verify the layer (exit 0 = present)
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
| macOS | `sudo agentic-pdf install-backend` | Any app → Print → *Agentic PDF Printer* → agentic PDF in `/Users/Shared/Agentic-PDF` |
| Windows | `agentic-pdf install-watch` | Any app → *Microsoft Print to PDF* → save into `%USERPROFILE%\Documents\Agentic-Spool` → converted automatically into `%USERPROFILE%\Documents\Agentic-PDF`, with a toast notification |
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
docs/demo/            sample inputs (see below)
```

**Demo files** — every example in this README uses a file that ships in the repo:

| File | What it exercises |
| --- | --- |
| `docs/demo/sample.txt` | Minimal plain-text report |
| `docs/demo/report.txt` | Rich structure: numbered sections, lists, metrics, risk factors (prints to 2 pages) |
| `docs/demo/memo.txt` | Business memo with headers, mixed formatting |
| `docs/demo/acme-q2-results.pdf` | **Pie chart + data table**: the table is extracted as a markdown table into `## Tables`, the chart's caption lands in `## Figures`, and its wedge labels appear in `## Content`. Regenerate with `gen_acme_report.py` |
| `docs/demo/fed-monetary-policy-report-july-2026.pdf` | Real-world stress test: 77-page Federal Reserve Monetary Policy Report (public domain). Tables become markdown; 60+ figure captions preserved |

Pre-generated agentic versions of these live in `docs/demo/` for the hosted viewer.

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
