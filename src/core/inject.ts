import { PDFDocument, PDFName, PDFString, PDFHexString, PDFArray, PDFDict } from "pdf-lib";
import { AGENT_HTML, AGENT_MD, INFO_KEY, INFO_VALUE, LLMS_TXT, MARKER_KEYWORD } from "./spec.js";
import { GenerateOptions, buildAgentMarkdown, markdownToHtml } from "./generate.js";
import { PageText } from "./text.js";

export interface InjectOptions extends GenerateOptions {
  /** Include an agent.html attachment alongside agent.md */
  html?: boolean;
}

/**
 * Embed the agentic layer into a PDF:
 *  - `agent.md` attachment (markdown mirror, hidden from normal viewing)
 *  - optional `agent.html` attachment
 *  - document metadata + custom Info key marking the spec version
 */
export async function injectAgentLayer(
  pdfBytes: Uint8Array,
  pages: PageText[],
  opts: InjectOptions = {}
): Promise<Uint8Array> {
  const pdf = await PDFDocument.load(pdfBytes, { updateMetadata: false });
  const now = opts.now ?? new Date();
  const { markdown, frontmatter } = buildAgentMarkdown(pages, { ...opts, now });
  const enc = new TextEncoder();

  await pdf.attach(enc.encode(markdown), AGENT_MD, {
    mimeType: "text/markdown",
    description: `Agentic layer (markdown mirror) — spec ${INFO_VALUE}`,
    creationDate: now,
    modificationDate: now,
  });

  if (opts.html !== false) {
    const html = wrapHtml(markdownToHtml(markdown), frontmatter.title);
    await pdf.attach(enc.encode(html), AGENT_HTML, {
      mimeType: "text/html",
      description: "Agentic layer (HTML rendering)",
      creationDate: now,
      modificationDate: now,
    });
  }

  // Metadata per spec: title, subject/description, keywords, dates.
  pdf.setTitle(frontmatter.title);
  pdf.setSubject(frontmatter.description);
  pdf.setKeywords([MARKER_KEYWORD, "agentic-pdf", "llm-readable"]);
  pdf.setModificationDate(now);
  if (opts.canonical) {
    // Store canonical source in the Info dictionary as a custom key.
    setInfoKey(pdf, "CanonicalSource", opts.canonical);
  }
  setInfoKey(pdf, INFO_KEY, INFO_VALUE);

  return pdf.save({ addDefaultPage: false });
}

export function hasAgentLayer(pdfBytes: Uint8Array): boolean {
  // Quick check without full attachment parse: look for attachment name + info key.
  const latin = Buffer.from(pdfBytes).toString("latin1");
  return latin.includes(AGENT_MD) || latin.includes(INFO_KEY);
}

function setInfoKey(pdf: PDFDocument, key: string, value: string) {
  const info = pdf.context.lookup(pdf.context.trailerInfo.Info);
  if (info instanceof PDFDict) {
    info.set(PDFName.of(key), PDFHexString.fromText(value));
  }
}

function wrapHtml(body: string, title: string): string {
  return `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="description" content="Agentic layer of a PDF document — machine-readable mirror.">
<meta property="og:title" content="${escapeAttr(title)}">
<meta property="og:description" content="Agent-readable markdown mirror embedded in a PDF.">
<title>${escapeAttr(title)} — Agentic Layer</title>
</head>
<body>
${body}
</body>
</html>`;
}

function escapeAttr(s: string): string {
  return s.replace(/&/g, "&amp;").replace(/"/g, "&quot;").replace(/</g, "&lt;");
}

export { PDFArray };
