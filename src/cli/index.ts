#!/usr/bin/env node
import { createRequire } from "node:module";
import { readFile, writeFile, mkdir } from "node:fs/promises";
import { spawnSync } from "node:child_process";
import path from "node:path";
import { printToAgenticPdf } from "../core/printer.js";
import { extractAgentMarkdown, readAgentLayer } from "../core/extract.js";
import { markdownToHtml } from "../core/generate.js";

const require_ = createRequire(import.meta.url);
const VERSION = require_("../../package.json").version as string;

const HELP = `
agentic-pdf v${VERSION} — PDF printer driver + viewer with a hidden agent-readable layer

Usage:
  agentic-pdf print <input> [-o out.pdf] [--title "T"] [--canonical URL] [--no-html]
      Convert any printable file to a PDF and embed the hidden agentic layer
      (agent.md / agent.html attachments). If <input> is already a PDF it is used as-is.

  agentic-pdf read <file.pdf> [--raw | --html | --meta]
      Extract and display the hidden agentic layer.
        (default)   pretty-printed markdown
        --raw       raw markdown exactly as embedded (pipe-friendly for agents)
        --html      rendered HTML
        --meta      frontmatter + document metadata only

  agentic-pdf view <file.pdf> [--port 4173] [--no-browser]
      Serve the viewer at http://localhost:<port>/
        GET /            viewer UI (Accept: text/markdown returns the raw layer)
        GET /agent.md    raw markdown mirror  (Link header rel=canonical -> /doc.pdf)
        GET /agent.html  HTML rendering of the mirror
        GET /doc.pdf     the original visual PDF

  agentic-pdf check <file.pdf>
      Exit 0 and print a summary if the file carries an agentic layer.

  agentic-pdf install-backend [--spool DIR]   Install CUPS virtual printer (sudo)
  agentic-pdf uninstall-backend               Remove the CUPS virtual printer

Options are passed through to the underlying commands where applicable.
`;

async function main() {
  const argv = process.argv.slice(2);
  const [cmd, ...rest] = argv;
  if (!cmd || cmd === "-h" || cmd === "--help" || cmd === "help") {
    process.stdout.write(HELP + "\n");
    return;
  }
  if (cmd === "--version" || cmd === "-v") {
    console.log(VERSION);
    return;
  }

  switch (cmd) {
    case "print":
      return await cmdPrint(rest);
    case "read":
      return await cmdRead(rest);
    case "view":
      return await cmdView(rest);
    case "check":
      return await cmdCheck(rest);
    case "install-backend":
      return await cmdInstallBackend(rest);
    case "uninstall-backend":
      return await cmdUninstallBackend();
    default:
      console.error(`Unknown command: ${cmd}\n`);
      process.stdout.write(HELP);
      process.exit(1);
  }
}

function parseFlags(args: string[]): { _: string[]; flags: Record<string, string | boolean> } {
  const out: { _: string[]; flags: Record<string, string | boolean> } = { _: [], flags: {} };
  for (let i = 0; i < args.length; i++) {
    const a = args[i];
    if (a === "-o" || a === "--out") out.flags.out = args[++i];
    else if (a === "--title") out.flags.title = args[++i];
    else if (a === "--canonical") out.flags.canonical = args[++i];
    else if (a === "--port") out.flags.port = args[++i];
    else if (a === "--no-html") out.flags.html = false;
    else if (a === "--no-browser") out.flags.browser = false;
    else if (a === "--raw") out.flags.raw = true;
    else if (a === "--html") out.flags.htmlMode = true;
    else if (a === "--meta") out.flags.meta = true;
    else if (a === "--spool") out.flags.spool = args[++i];
    else out._.push(a);
  }
  return out;
}

async function cmdPrint(args: string[]) {
  const { _, flags } = parseFlags(args);
  const input = _[0];
  if (!input) die("print: missing <input>");
  const output =
    (flags.out as string) ??
    input.replace(/\.[^.]+$/, "") + ".agentic.pdf";

  console.error(`⏳ printing ${input} → ${output}`);
  const result = await printToAgenticPdf(input, output, {
    title: flags.title as string | undefined,
    canonical: flags.canonical as string | undefined,
    html: flags.html !== false,
  });
  console.error(`✅ wrote ${result.outputPath} (${result.pages} page(s), agentic layer embedded)`);
}

async function cmdRead(args: string[]) {
  const { _, flags } = parseFlags(args);
  const input = _[0];
  if (!input) die("read: missing <file.pdf>");
  const bytes = new Uint8Array(await readFile(input));

  if (flags.meta) {
    const layer = await readAgentLayer(bytes);
    if (!layer.markdown && !layer.html) die("No agentic layer found in this PDF.");
    console.log(JSON.stringify(layer.metadata, null, 2));
    console.log("---");
    console.log(JSON.stringify(layer.frontmatter, null, 2));
    return;
  }

  const md = await extractAgentMarkdown(bytes);
  if (flags.raw) {
    process.stdout.write(md);
    return;
  }
  if (flags.htmlMode) {
    process.stdout.write(markdownToHtml(md));
    return;
  }
  // Pretty terminal view
  const m = /^---\n([\s\S]*?)\n---\n/.exec(md);
  if (m) console.log(m[0].replace(/^---\n/, "").replace(/\n---$/, "\n"));
  process.stdout.write(md.replace(/^---[\s\S]*?---\n/, ""));
}

async function cmdCheck(args: string[]) {
  const input = args.find((a) => !a.startsWith("-"));
  if (!input) die("check: missing <file.pdf>");
  const bytes = new Uint8Array(await readFile(input));
  const layer = await readAgentLayer(bytes);
  if (!layer.markdown && !layer.html) {
    console.log(`${input}: no agentic layer`);
    process.exit(1);
  }
  console.log(`${input}: agentic layer present`);
  console.log(`  spec:     ${layer.metadata.agentReadability ?? "unknown"}`);
  console.log(`  title:    ${layer.frontmatter.title ?? layer.metadata.title ?? "?"}`);
  console.log(`  updated:  ${layer.frontmatter.last_updated ?? "?"}`);
  console.log(`  agent.md: ${layer.markdown ? "yes" : "no"}`);
  console.log(`  agent.html: ${layer.html ? "yes" : "no"}`);
  console.log(`  llms.txt: ${layer.llmsTxt ? "yes" : "no"}`);
}

async function cmdView(args: string[]) {
  const { _, flags } = parseFlags(args);
  const input = _[0];
  if (!input) die("view: missing <file.pdf>");
  const port = Number(flags.port ?? 4173);
  const { startViewer } = await import("../viewer/server.js");
  await startViewer({ pdfPath: path.resolve(input), port, openBrowser: flags.browser !== false });
}

async function cmdInstallBackend(args: string[]) {
  const { flags } = parseFlags(args);
  const spool = (flags.spool as string) ?? `${process.env.HOME}/Documents/Agentic-PDF`;
  const script = buildBackendScript(spool);
  const tmpScript = "/tmp/agentic-pdf-backend-agentpdf";
  await writeFile(tmpScript, script, { mode: 0o755 });
  const cliPath = path.resolve(import.meta.dirname, "../../");
  const conf = `AGENTIC_PDF_HOME=${cliPath}\nSPOOL_DIR=${spool}\n`;
  await writeFile("/tmp/agentic-pdf.conf", conf);
  await mkdir(spool, { recursive: true });
  console.error("Installing CUPS backend (requires sudo)…");
  run("sudo", ["cp", tmpScript, "/usr/libexec/cups/backend/agentpdf"]);
  run("sudo", ["chown", "root:wheel", "/usr/libexec/cups/backend/agentpdf"]);
  run("sudo", ["chmod", "755", "/usr/libexec/cups/backend/agentpdf"]);
  run("sudo", ["mkdir", "-p", spool]);
  run("sudo", ["chown", `${process.env.USER}:staff`, spool]);
  run("sudo", ["cp", "/tmp/agentic-pdf.conf", "/etc/agentic-pdf.conf"]);
  // Register the printer queue pointing at the backend.
  const uri = `agentpdf:${spool}`;
  run("sudo", ["lpadmin", "-p", "AgenticPDF", "-E", "-v", uri, "-D", "Agentic PDF Printer"]);
  run("sudo", ["launchctl", "kickstart", "-k", "system/org.cups.cupsd"]);
  console.log(`✅ Installed "Agentic PDF Printer".
   - Backend: /usr/libexec/cups/backend/agentpdf
   - Queue:   AgenticPDF (${uri})
   - Output:  ${spool}
Print from any app and pick "Agentic PDF Printer" — the resulting PDF lands
in the output folder with an embedded agent-readable layer.`);
}

async function cmdUninstallBackend() {
  console.error("Removing CUPS backend (requires sudo)…");
  tryRun("lpadmin", ["-x", "AgenticPDF"]);
  run("sudo", ["rm", "-f", "/usr/libexec/cups/backend/agentpdf", "/etc/agentic-pdf.conf"]);
  run("sudo", ["launchctl", "kickstart", "-k", "system/org.cups.cupsd"]);
  console.log("✅ Uninstalled.");
}

function run(cmd: string, args: string[]) {
  const res = spawnSync(cmd, args, { stdio: "inherit" });
  if (res.status !== 0) {
    die(`${cmd} failed with exit code ${res.status}`);
  }
}

function tryRun(cmd: string, args: string[]) {
  try {
    spawnSync(cmd, args, { stdio: "ignore" });
  } catch {
    /* ignore */
  }
}

export function buildBackendScript(spool: string): string {
  return `#!/bin/sh
# CUPS backend for agentic-pdf — converts print jobs into agentic PDFs.
CONF=/etc/agentic-pdf.conf
[ -f "$CONF" ] && . "$CONF"
AGENTIC_PDF_HOME=\${AGENTIC_PDF_HOME:-/Users/${process.env.USER}/src/agentic-pdf}
SPOOL_DIR=\${SPOOL_DIR:-${spool}}
NODE=\${NODE:-$(command -v node)}

# Discovery (no args): advertise the device.
if [ $# -eq 0 ]; then
  echo "network agentpdf \\"Agentic PDF Printer\\" \\"Prints to agentic PDFs with an embedded agent-readable layer v${VERSION}\\""
  exit 0
fi

job_id="$1"; user="$2"; title="$3"; copies="$4"; options="$5"; file="$6"

[ -x "$NODE" ] || { echo "ERROR: node not found"; exit 1; }
CLI="$AGENTIC_PDF_HOME/dist/cli/index.js"
[ -f "$CLI" ] || { echo "ERROR: agentic-pdf CLI not installed at $CLI"; exit 1; }

mkdir -p "$SPOOL_DIR" 2>/dev/null || exit 1

tmp_in=$(mktemp /tmp/agentpdf.XXXXXX)
if [ -n "$file" ] && [ -r "$file" ]; then
  cp "$file" "$tmp_in"
else
  cat > "$tmp_in"
fi

safe_title=$(printf '%s' "$title" | tr '/:' '__' | cut -c1-80)
out="$SPOOL_DIR/$safe_title-$job_id.pdf"

"$NODE" "$CLI" print "$tmp_in" -o "$out" --title "$title" >/dev/null 2>&1
rc=$?
rm -f "$tmp_in"
[ $rc -ne 0 ] && { echo "ERROR: agentic conversion failed"; exit 1; }
chmod 644 "$out" 2>/dev/null

echo "READY"
exit 0
`;
}

function die(msg: string): never {
  console.error(`error: ${msg}`);
  process.exit(1);
}

const isMain =
  process.argv[1] &&
  (process.argv[1].endsWith("cli/index.js") ||
    path.resolve(process.argv[1]) === import.meta.url);

if (isMain) {
  main().catch((err) => {
    console.error(String(err instanceof Error ? err.message : err));
    process.exit(1);
  });
}
