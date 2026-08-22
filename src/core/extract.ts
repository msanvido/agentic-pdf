import { PDFDocument, PDFName, PDFDict, PDFString, PDFHexString, PDFRef, PDFArray, PDFRawStream, decodePDFRawStream } from "pdf-lib";
import { AGENT_HTML, AGENT_MD, LLMS_TXT, parseFrontmatter } from "./spec.js";

export interface AgentLayer {
  markdown: string | null;
  html: string | null;
  llmsTxt: string | null;
  frontmatter: Record<string, string>;
  metadata: {
    title?: string;
    subject?: string;
    keywords?: string[];
    canonicalSource?: string;
    agentReadability?: string;
  };
}

export async function readAgentLayer(pdfBytes: Uint8Array): Promise<AgentLayer> {
  const pdf = await PDFDocument.load(pdfBytes);
  const attachments = readAttachments(pdf);
  const decode = (a: { bytes: Uint8Array } | undefined) =>
    a ? new TextDecoder().decode(a.bytes) : null;

  const markdown = decode(attachments[AGENT_MD]);
  const html = decode(attachments[AGENT_HTML]);
  const llmsTxt = decode(attachments[LLMS_TXT]);

  let title: string | undefined;
  let subject: string | undefined;
  try {
    title = pdf.getTitle() || undefined;
    subject = pdf.getSubject() || undefined;
  } catch {
    /* ignore */
  }

  let agentReadability: string | undefined;
  let canonicalSource: string | undefined;
  try {
    const dict = pdf.context.lookupMaybe(pdf.context.trailerInfo.Info, PDFDict);
    if (dict) {
      agentReadability = decodeInfoValue(dict, "AgentReadability");
      canonicalSource = decodeInfoValue(dict, "CanonicalSource");
    }
  } catch {
    /* ignore */
  }

  return {
    markdown,
    html,
    llmsTxt,
    frontmatter: markdown ? parseFrontmatter(markdown).frontmatter : {},
    metadata: {
      title,
      subject,
      keywords: safeKeywords(pdf),
      canonicalSource,
      agentReadability,
    },
  };
}

/** Walk the catalog's EmbeddedFiles name tree (pdf-lib 1.x has no getAttachments). */
function readAttachments(pdf: PDFDocument): Record<string, { bytes: Uint8Array; description?: string }> {
  const out: Record<string, { bytes: Uint8Array; description?: string }> = {};
  try {
    const catalog = pdf.context.lookupMaybe(pdf.context.trailerInfo.Root, PDFDict);
    const names = catalog?.lookupMaybe(PDFName.of("Names"), PDFDict);
    const embeddedFiles = names?.lookupMaybe(PDFName.of("EmbeddedFiles"), PDFDict);
    if (!embeddedFiles) return out;

    const visit = (node: PDFDict) => {
      const kids = node.lookupMaybe(PDFName.of("Kids"), PDFArray);
      if (kids) {
        for (let i = 0; i < kids.size(); i++) {
          const kid = kids.get(i);
          const kidDict =
            kid instanceof PDFRef
              ? pdf.context.lookupMaybe(kid, PDFDict)
              : undefined;
          if (kidDict) visit(kidDict);
        }
        return;
      }
      const entries = node.lookupMaybe(PDFName.of("Names"), PDFArray);
      if (!entries) return;
      for (let i = 0; i + 1 < entries.size(); i += 2) {
        const nameVal = entries.lookup(i);
        const spec = entries.lookup(i + 1, PDFDict);
        let filename: string | undefined;
        if (nameVal instanceof PDFHexString) filename = nameVal.decodeText();
        else if (nameVal instanceof PDFString) filename = nameVal.asString();
        if (!filename || !spec) continue;

        const ef = spec.lookupMaybe(PDFName.of("EF"), PDFDict);
        const fileStreamRef = ef?.get(PDFName.of("F"));
        let stream: PDFRawStream | undefined;
        if (fileStreamRef instanceof PDFRef) {
          const obj = pdf.context.lookup(fileStreamRef);
          if (obj instanceof PDFRawStream) stream = obj;
        } else if (fileStreamRef instanceof PDFRawStream) {
          stream = fileStreamRef;
        }
        if (!stream) continue;

        const descValue = spec.lookupMaybe(PDFName.of("Desc"), PDFHexString);
        out[filename] = {
          bytes: decodePDFRawStream(stream).decode(),
          description: descValue?.decodeText(),
        };
      }
    };
    visit(embeddedFiles);
  } catch {
    /* malformed name tree — return what we have */
  }
  return out;
}

function decodeInfoValue(dict: PDFDict, name: string): string | undefined {
  const value = dict.get(PDFName.of(name));
  if (!value || value instanceof PDFRef) return undefined;
  if (value instanceof PDFHexString) return value.decodeText();
  if (value instanceof PDFString) return value.asString();
  return String(value);
}

function safeKeywords(pdf: PDFDocument): string[] | undefined {
  try {
    return pdf.getKeywords()?.split(/\s+/).filter(Boolean);
  } catch {
    return undefined;
  }
}

/** Extract just the raw markdown layer (convenience for CLI/agents). */
export async function extractAgentMarkdown(pdfBytes: Uint8Array): Promise<string> {
  const layer = await readAgentLayer(pdfBytes);
  if (!layer.markdown) {
    throw new Error("No agentic layer found. Is this an agentic-pdf document?");
  }
  return layer.markdown;
}
