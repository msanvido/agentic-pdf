export interface PageText {
  page: number;
  lines: string[];
}

/** Extract per-page text from a PDF using pdf.js (Node, no worker). */
export async function extractPages(pdfBytes: Uint8Array): Promise<PageText[]> {
  const pdfjs = await import("pdfjs-dist/legacy/build/pdf.mjs");
  const loadingTask = pdfjs.getDocument({
    data: pdfBytes.slice(),
    isEvalSupported: false,
    useSystemFonts: true,
    disableFontFace: true,
  });
  const doc = await loadingTask.promise;
  try {
    const pages: PageText[] = [];
    for (let i = 1; i <= doc.numPages; i++) {
      const page = await doc.getPage(i);
      const content = await page.getTextContent();
      // Group items into lines by transform Y position.
      const rows = new Map<number, { x: number; str: string }[]>();
      for (const item of content.items) {
        if (!("str" in item) || !item.str.trim()) continue;
        const y = Math.round(item.transform[5]);
        if (!rows.has(y)) rows.set(y, []);
        rows.get(y)!.push({ x: item.transform[4], str: item.str });
      }
      const sortedYs = [...rows.keys()].sort((a, b) => b - a);
      const lines: string[] = [];
      for (const y of sortedYs) {
        const parts = rows.get(y)!.sort((a, b) => a.x - b.x);
        const line = parts
          .map((p, idx) => (idx > 0 && p.x - (parts[idx - 1].x + parts[idx - 1].str.length * 2) > 12 ? "  " : "") + p.str)
          .join("")
          .replace(/\s+/g, " ")
          .trim();
        if (line) lines.push(line);
      }
      pages.push({ page: i, lines });
    }
    return pages;
  } finally {
    await loadingTask.destroy();
  }
}

const HEADING_MAX_LEN = 80;
const BULLET_RE = /^([-*•·‣◦]|\d+[.)])\s+/;
const SECTION_HINT_RE = /^(chapter|section|appendix|part)\b/i;

/**
 * Heuristically convert extracted page text into structured markdown:
 * short standalone lines that look like titles become headings,
 * bullet-like lines become lists, everything else paragraphs.
 */
export function textToMarkdown(pages: PageText[]): string {
  const out: string[] = [];
  let para: string[] = [];

  const flushPara = () => {
    if (para.length) {
      out.push(para.join(" "));
      out.push("");
      para = [];
    }
  };

  for (const { page, lines } of pages) {
    out.push(`### Page ${page}`);
    out.push("");
    for (let i = 0; i < lines.length; i++) {
      const line = lines[i];
      const prev = i > 0 ? lines[i - 1] : "";
      const next = i + 1 < lines.length ? lines[i + 1] : "";

      if (BULLET_RE.test(line)) {
        flushPara();
        out.push("- " + line.replace(BULLET_RE, ""));
        // consume consecutive bullets so we don't emit stray blank logic
        if (!BULLET_RE.test(next)) out.push("");
        continue;
      }

      const looksLikeHeading =
        line.length <= HEADING_MAX_LEN &&
        !/[.,;:]$/.test(line) &&
        (i === 0 ||
          prev === "" ||
          next === "" ||
          (line === line.toUpperCase() && /\p{L}/u.test(line)));

      if (looksLikeHeading && line.length <= HEADING_MAX_LEN && !/\.$/.test(line)) {
        flushPara();
        const level = line === line.toUpperCase() && /\p{L}/u.test(line) ? "##" : "**";
        if (level === "##") {
          out.push(`## ${titleCase(line)} _(p.${page})_`);
          out.push("");
        } else {
          out.push(`#### ${line}`);
          out.push("");
        }
        continue;
      }

      para.push(line);
      if (/[.!?:]$/.test(line) && (next === "" || BULLET_RE.test(next))) {
        flushPara();
      }
    }
    flushPara();
  }
  return out.join("\n").replace(/\n{3,}/g, "\n\n");
}

function titleCase(s: string): string {
  return s
    .toLowerCase()
    .replace(/(^|\s)(\p{L})/gu, (_m, p, c) => p + c.toUpperCase());
}

export function deriveSummary(pages: PageText[], maxChars = 220): string {
  const firstContent: string[] = [];
  for (const { lines } of pages) {
    for (const line of lines) {
      firstContent.push(line);
      if (firstContent.join(" ").length >= maxChars) break;
    }
    if (firstContent.join(" ").length >= maxChars) break;
  }
  const text = firstContent.join(" ").trim();
  if (text.length <= maxChars) return text || "(no extractable text)";
  return text.slice(0, maxChars - 1).replace(/\s+\S*$/, "") + "…";
}

export function guessTitle(pages: PageText[]): string | undefined {
  for (const { lines } of pages.slice(0, 2)) {
    for (const line of lines.slice(0, 5)) {
      if (
        line.length >= 3 &&
        line.length <= HEADING_MAX_LEN &&
        !SECTION_HINT_RE.test(line)
      ) {
        return titleCase(line);
      }
    }
  }
  return undefined;
}
