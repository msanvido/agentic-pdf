import http from "node:http";
import { createRequire } from "node:module";
import { readFile } from "node:fs/promises";
import path from "node:path";
import { exec } from "node:child_process";
import { readAgentLayer } from "../core/extract.js";
import { markdownToHtml } from "../core/generate.js";

const require_ = createRequire(import.meta.url);
const PDFJS_DIR = path.dirname(require_.resolve("pdfjs-dist/legacy/build/pdf.mjs"));
const MARKED_PATH = require_.resolve("marked/marked.min.js");

export interface ViewerOptions {
  pdfPath: string;
  port?: number;
  openBrowser?: boolean;
}

export async function startViewer(opts: ViewerOptions): Promise<void> {
  const port = opts.port ?? 4173;
  const pdfBytes = await readFile(opts.pdfPath);
  const layer = await readAgentLayer(new Uint8Array(pdfBytes));
  const hasLayer = Boolean(layer.markdown);

  const server = http.createServer(async (req, res) => {
    try {
      const url = new URL(req.url ?? "/", "http://localhost");
      const accept = req.headers.accept ?? "";

      // Content negotiation on / — markdown clients get the raw agent layer.
      if (url.pathname === "/" && accept.includes("text/markdown") && layer.markdown) {
        res.writeHead(200, {
          "Content-Type": "text/plain; charset=utf-8",
          "Link": `</doc.pdf>; rel="canonical"`,
        });
        return res.end(layer.markdown);
      }

      switch (url.pathname) {
        case "/":
          return send(res, 200, "text/html; charset=utf-8", await viewerHtml());
        case "/doc.pdf":
          res.writeHead(200, { "Content-Type": "application/pdf" });
          return res.end(pdfBytes);
        case "/agent.md":
          if (!layer.markdown) return send(res, 404, "text/plain", "no agentic layer");
          res.writeHead(200, {
            "Content-Type": "text/markdown; charset=utf-8",
            "Link": `</doc.pdf>; rel="canonical"`,
          });
          return res.end(layer.markdown);
        case "/agent.html":
          if (!layer.markdown) return send(res, 404, "text/plain", "no agentic layer");
          return send(res, 200, "text/html; charset=utf-8", wrapStandaloneHtml(layer));
        case "/meta.json":
          return send(res, 200, "application/json", JSON.stringify({ hasAgentLayer: hasLayer, ...layer.metadata, frontmatter: layer.frontmatter }));
        case "/vendor/pdf.mjs":
          return sendFile(res, path.join(PDFJS_DIR, "pdf.min.mjs"), "text/javascript");
        case "/vendor/pdf.worker.mjs":
          return sendFile(res, path.join(PDFJS_DIR, "pdf.worker.min.mjs"), "text/javascript");
        case "/vendor/marked.min.js":
          return sendFile(res, MARKED_PATH, "text/javascript");
        default:
          return send(res, 404, "text/plain", "not found");
      }
    } catch (err) {
      const msg = err instanceof Error ? err.message : String(err);
      return send(res, 500, "text/plain", msg);
    }
  });

  await new Promise<void>((resolve) => server.listen(port, resolve));
  const url = `http://localhost:${port}/`;
  console.log(`👁  viewing ${opts.pdfPath}`);
  console.log(`   viewer:     ${url}`);
  console.log(`   agent.md:   ${url}agent.md`);
  if (hasLayer) console.log("   agentic layer detected ✔");
  else console.warn("   ⚠ no agentic layer in this PDF (showing visual content only)");
  console.log("   Ctrl-C to stop");

  if (opts.openBrowser !== false) {
    exec(`open ${JSON.stringify(url)}`);
  }
  process.on("SIGINT", () => {
    server.close();
    process.exit(0);
  });
}

function wrapStandaloneHtml(layer: Awaited<ReturnType<typeof readAgentLayer>>): string {
  const title = layer.frontmatter.title ?? layer.metadata.title ?? "Document";
  const body = layer.html ?? markdownToHtml(layer.markdown ?? "");
  return `<!doctype html>
<html lang="en"><head><meta charset="utf-8">
<meta name="description" content="Agentic layer of ${title}">
<title>${title} — Agentic Layer</title>
<style>body{max-width:52rem;margin:2rem auto;padding:0 1rem;font-family:-apple-system,sans-serif;line-height:1.6}
pre{background:#f5f5f5;padding:.75rem;border-radius:6px;overflow:auto}code{background:#f5f5f5;padding:.1em .3em;border-radius:3px}</style>
</head><body>${body}</body></html>`;
}

function send(res: http.ServerResponse, code: number, type: string, body: string | Buffer) {
  res.writeHead(code, { "Content-Type": type });
  res.end(body);
}

async function sendFile(res: http.ServerResponse, filePath: string, type: string) {
  try {
    const data = await readFile(filePath);
    send(res, 200, type, data);
  } catch {
    send(res, 404, "text/plain", "asset not found");
  }
}

/** Serve the viewer page with local assets (pdf.js + marked from node_modules). */
async function viewerHtml(): Promise<string> {
  return readFile(path.join(import.meta.dirname!, "viewer.html"), "utf8");
}
