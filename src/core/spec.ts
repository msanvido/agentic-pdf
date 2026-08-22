export const SPEC_VERSION = "1.0";

export const AGENT_MD = "agent.md";
export const AGENT_HTML = "agent.html";
export const LLMS_TXT = "llms.txt";

export const INFO_KEY = "AgentReadability";
export const INFO_VALUE = `agentic-pdf-spec/${SPEC_VERSION}`;

export const MARKER_KEYWORD = "agent-readable";

export interface AgentFrontmatter {
  title: string;
  description: string;
  doc_version: string;
  last_updated: string;
  canonical?: string;
  generator: string;
  source_pages?: number;
}

export function renderFrontmatter(fm: AgentFrontmatter): string {
  const lines = ["---"];
  lines.push(`title: ${JSON.stringify(fm.title)}`);
  lines.push(`description: ${JSON.stringify(fm.description)}`);
  lines.push(`doc_version: ${JSON.stringify(fm.doc_version)}`);
  lines.push(`last_updated: ${JSON.stringify(fm.last_updated)}`);
  if (fm.canonical) lines.push(`canonical: ${JSON.stringify(fm.canonical)}`);
  if (fm.source_pages != null) lines.push(`source_pages: ${fm.source_pages}`);
  lines.push(`generator: ${JSON.stringify(fm.generator)}`);
  lines.push("---");
  return lines.join("\n");
}

export function parseFrontmatter(md: string): { frontmatter: Record<string, string>; body: string } {
  const m = /^---\r?\n([\s\S]*?)\r?\n---\r?\n?/.exec(md);
  if (!m) return { frontmatter: {}, body: md };
  const frontmatter: Record<string, string> = {};
  for (const line of m[1].split(/\r?\n/)) {
    const kv = /^([A-Za-z_][\w-]*):\s*(.*)$/.exec(line);
    if (!kv) continue;
    let value = kv[2].trim();
    const q = /^"(.*)"(\s*#.*)?$/.exec(value) ?? /^'(.*)'(\s*#.*)?$/.exec(value);
    if (q) value = q[1];
    frontmatter[kv[1]] = value;
  }
  return { frontmatter, body: md.slice(m[0].length) };
}
