import { readFile, writeFile } from "node:fs/promises";
import { execFile } from "node:child_process";
import { promisify } from "node:util";
import path from "node:path";
import { extractPages } from "./text.js";
import { injectAgentLayer } from "./inject.js";

const execFileP = promisify(execFile);

export interface PrintResult {
  outputPath: string;
  pages: number;
  hadAgentLayer: boolean;
}

/**
 * "Print" a file to an agentic PDF:
 *   1. convert input to PDF if needed (macOS cupsfilter; passthrough for .pdf)
 *   2. extract text
 *   3. embed the hidden agent layer (agent.md / agent.html attachments)
 */
export async function printToAgenticPdf(
  inputPath: string,
  outputPath: string,
  options: { title?: string; canonical?: string; html?: boolean } = {}
): Promise<PrintResult> {
  const pdfBytes = await toPdf(inputPath);
  const pages = await extractPages(pdfBytes);
  const result = await injectAgentLayer(pdfBytes, pages, {
    title: options.title,
    canonical: options.canonical ?? `file://${path.resolve(inputPath)}`,
    html: options.html,
  });
  await writeFile(outputPath, result);
  return {
    outputPath,
    pages: pages.length,
    hadAgentLayer: false,
  };
}

/** Convert arbitrary printable input to PDF bytes. */
async function toPdf(inputPath: string): Promise<Uint8Array> {
  const buf = await readFile(inputPath);
  const isPdf =
    path.extname(inputPath).toLowerCase() === ".pdf" ||
    buf.subarray(0, 5).toString("latin1") === "%PDF-";

  if (isPdf) return new Uint8Array(buf);

  // macOS: cupsfilter converts text/images/etc. through the CUPS filter chain.
  // It writes the result to stdout; never use -D here (it deletes the input!).
  const { stdout } = await execFileP(
    "/usr/sbin/cupsfilter",
    ["-o", "media=A4", "-o", "fit-to-page", inputPath],
    {
      timeout: 60_000,
      encoding: "buffer" as never,
      maxBuffer: 256 * 1024 * 1024,
    }
  );
  const pdfOut = Buffer.from(stdout as unknown as ArrayBuffer);
  if (pdfOut.subarray(0, 5).toString("latin1") !== "%PDF-") {
    throw new Error(`cupsfilter could not convert ${inputPath} to PDF`);
  }
  return new Uint8Array(pdfOut);
}
