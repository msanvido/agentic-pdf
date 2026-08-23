# LinkedIn post — agentic-pdf intro

We made web pages agentic-friendly ([Vercel's agent readability spec](https://vercel.com/kb/guide/agent-readability-spec), llms.txt): sites publish markdown mirrors so AI agents can read them directly instead of scraping HTML.

We should do the same for all data that LLMs want to process.

Here's a first stab at making PDFs agentic-friendly: **agentic-pdf** ([github.com/msanvido/agentic-pdf](https://github.com/msanvido/agentic-pdf))

PDFs carry most of the world's important information — reports, invoices, contracts, manuals, research — but they were never designed with machines in mind. So today, every AI pipeline has to reconstruct meaning after the fact:

legacy PDF → OCR / vision model → cleanup & repair → LLM (per page, every time)

Instead, go to the source and embed an invisible markdown layer (`agent.md`) inside ordinary PDFs at print time:

Raw content → PDF + agent.md → LLM (free, exact, instant)

Humans see exactly the PDF they always saw. Agents extract one attachment and get clean structure — frontmatter, headings, tables, glossary — as plain markdown. No extraction cost, no third party, no model guesswork.

Scanned archives will always need OCR. But most documents today are *born digital* — they pass through a printer driver anyway. Print them with the agent layer built in, and every downstream AI workload skips the extraction step entirely.
