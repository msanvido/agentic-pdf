import { AgentFrontmatter, renderFrontmatter, SPEC_VERSION } from "./spec.js";
import { PageText, deriveSummary, guessTitle, textToMarkdown } from "./text.js";

export interface GenerateOptions {
  title?: string;
  description?: string;
  canonical?: string;
  docVersion?: string;
  now?: Date;
}

export function buildAgentMarkdown(pages: PageText[], opts: GenerateOptions = {}): { markdown: string; frontmatter: AgentFrontmatter } {
  const now = opts.now ?? new Date();
  const title = opts.title ?? guessTitle(pages) ?? "Untitled Document";
  const description =
    opts.description && opts.description.length >= 50
      ? opts.description
      : deriveSummary(pages);

  const frontmatter: AgentFrontmatter = {
    title,
    description,
    doc_version: opts.docVersion ?? "1.0",
    last_updated: now.toISOString(),
    canonical: opts.canonical,
    generator: `agentic-pdf/${SPEC_VERSION}`,
    source_pages: pages.length,
  };

  const body = textToMarkdown(pages);
  const glossary = buildGlossary(pages);
  const toc = buildToc(body);

  const markdown = [
    renderFrontmatter(frontmatter),
    "",
    `# ${title}`,
    "",
    `> **For AI agents:** This is the machine-readable layer of a printed PDF document.`,
    `> It mirrors the human-readable PDF content in structured markdown so you can parse`,
    `> it directly without OCR or layout heuristics. The visual PDF remains unchanged.`,
    "",
    "## Summary",
    "",
    description,
    "",
    ...(toc ? ["## Table of Contents", "", toc, ""] : []),
    "## Content",
    "",
    body,
    "## Sitemap",
    "",
    "- [agent.md](agent.md) — this file (markdown mirror of the document)",
    "- [agent.html](agent.html) — HTML rendering of this mirror",
    "- `/` — the original visual PDF (canonical)",
    "",
    "## Glossary",
    "",
    glossary,
    "",
  ].join("\n");

  return { markdown, frontmatter };
}

function buildToc(body: string): string {
  const headings = [...body.matchAll(/^## (.+)$/gm)].map((m) => m[1]);
  if (headings.length < 2) return "";
  return headings.map((h) => `- ${h}`).join("\n");
}

/** Extract likely acronyms / capitalized terms as a lightweight glossary, per spec. */
function buildGlossary(pages: PageText[]): string {
  const counts = new Map<string, number>();
  for (const { lines } of pages) {
    for (const line of lines) {
      for (const m of line.matchAll(/\b[A-Z][A-Za-z0-9]*(-[A-Za-z0-9]+)*\b/g)) {
        const term = m[0];
        if (
          term.length >= 2 &&
          term.length <= 24 &&
          !/^(The|This|That|And|For|With|From|Page|Chapter|Section|Appendix|Table|Figure|Note)$/.test(term)
        ) {
          counts.set(term, (counts.get(term) ?? 0) + 1);
        }
      }
    }
  }
  const top = [...counts.entries()]
    .filter(([t, n]) => n > 1 || /^[A-Z]{2,}$/.test(t))
    .sort((a, b) => b[1] - a[1])
    .slice(0, 12);
  if (!top.length) return "_No recurring terminology detected._";
  return top.map(([term, n]) => `- **${term}** — appears ${n}×; verify meaning in context.`).join("\n");
}

export function markdownToHtml(md: string): string {
  // Minimal renderer to avoid heavy deps at runtime; marked is used in the viewer.
  const escape = (s: string) =>
    s.replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;");
  const lines = md.split("\n");
  const out: string[] = [];
  let inList = false;
  for (const raw of lines) {
    const line = raw.trimEnd();
    const h = /^(#{1,6})\s+(.*)$/.exec(line);
    const li = /^[-*]\s+(.*)$/.exec(line);
    if (h) {
      if (inList) { out.push("</ul>"); inList = false; }
      out.push(`<h${h[1].length}>${inline(h[2])}</h${h[1].length}>`);
    } else if (li) {
      if (!inList) { out.push("<ul>"); inList = true; }
      out.push(`<li>${inline(li[1])}</li>`);
    } else if (line === "") {
      if (inList) { out.push("</ul>"); inList = false; }
      out.push("");
    } else {
      if (inList) { out.push("</ul>"); inList = false; }
      out.push(`<p>${inline(escape(line))}</p>`);
    }
  }
  if (inList) out.push("</ul>");
  return out.join("\n");

  function inline(s: string): string {
    return s
      .replace(/&(?!(amp|lt|gt);)/g, "&amp;")
      .replace(/\*\*([^*]+)\*\*/g, "<strong>$1</strong>")
      .replace(/(^|\s)_([^_]+)_(\s|$)/g, "$1<em>$2</em>$3")
      .replace(/`([^`]+)`/g, "<code>$1</code>")
      .replace(/\[([^\]]+)\]\(([^)]+)\)/g, '<a href="$2">$1</a>')
      .replace(/^&gt;\s?(.*)$/, "<blockquote>$1</blockquote>");
  }
}
