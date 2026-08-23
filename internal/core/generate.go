package core

import (
	"sort"
	"strings"
	"time"
)

// BuildGlossary extracts likely acronyms / recurring capitalized terms.
func BuildGlossary(pages []PageText) string {
	counts := map[string]int{}
	stop := map[string]bool{
		"The": true, "This": true, "That": true, "And": true, "For": true,
		"With": true, "From": true, "Page": true, "Chapter": true,
		"Section": true, "Appendix": true, "Table": true, "Figure": true, "Note": true,
	}
	for _, pg := range pages {
		for _, line := range pg.Lines {
			for _, term := range termRE.FindAllString(line, -1) {
				if len(term) < 2 || len(term) > 24 || stop[term] {
					continue
				}
				counts[term]++
			}
		}
	}
	type kv struct {
		t string
		n int
	}
	var top []kv
	for t, n := range counts {
		if n > 1 || allUpperRE.MatchString(t) {
			top = append(top, kv{t, n})
		}
	}
	if len(top) == 0 {
		return "_No recurring terminology detected._"
	}
	sort.Slice(top, func(i, j int) bool { return top[i].n > top[j].n })
	if len(top) > 12 {
		top = top[:12]
	}
	parts := make([]string, len(top))
	for i, e := range top {
		parts[i] = "- **" + e.t + "** — appears " + itoa(e.n) + "×; verify meaning in context."
	}
	return strings.Join(parts, "\n")
}

// MarkdownToHTML renders a small, dependency-free subset of markdown to HTML
func MarkdownToHTML(md string) string {
	var out []string
	inList := false
	closeList := func() {
		if inList {
			out = append(out, "</ul>")
			inList = false
		}
	}
	for _, raw := range strings.Split(md, "\n") {
		line := strings.TrimRight(raw, " \t")
		switch {
		case strings.HasPrefix(line, "#"):
			level := 0
			for level < len(line) && line[level] == '#' {
				level++
			}
			if level <= 6 && level < len(line) && line[level] == ' ' {
				closeList()
				out = append(out, htmlTag("h"+itoa(level), renderInline(line[level+1:])))
				continue
			}
			out = append(out, "<p>"+renderInline(escapeHTML(line))+"</p>")
		case strings.HasPrefix(line, "- ") || strings.HasPrefix(line, "* "):
			if !inList {
				out = append(out, "<ul>")
				inList = true
			}
			out = append(out, "<li>"+renderInline(line[2:])+"</li>")
		case strings.TrimSpace(line) == "":
			closeList()
			out = append(out, "")
		default:
			closeList()
			out = append(out, "<p>"+renderInline(escapeHTML(line))+"</p>")
		}
	}
	closeList()
	return strings.Join(out, "\n")
}

func escapeHTML(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")
	return r.Replace(s)
}

func renderInline(s string) string {
	s = escapeHTML(s)
	s = boldRE.ReplaceAllString(s, "<strong>$1</strong>")
	s = emRE.ReplaceAllString(s, "$1<em>$2</em>$3")
	s = codeRE.ReplaceAllString(s, "<code>$1</code>")
	s = linkRE.ReplaceAllString(s, `<a href="$2">$1</a>`)
	return s
}

// BuildAgentMarkdown assembles the full agent.md content per the spec:
// frontmatter, agent note, summary, TOC, content, sitemap and glossary.
func BuildAgentMarkdown(pages []PageText, title, description, canonical, docVersion string) (markdown string, fm Frontmatter) {
	if title == "" {
		title = GuessTitle(pages)
	}
	if title == "" {
		title = "Untitled Document"
	}
	if len(description) < 50 {
		description = DeriveSummary(pages, 220)
	}
	if docVersion == "" {
		docVersion = "1.0"
	}

	fm = Frontmatter{
		Title:       title,
		Description: description,
		DocVersion:  docVersion,
		LastUpdated: time.Now().UTC().Format("2006-01-02T15:04:05Z"),
		Canonical:   canonical,
		Generator:   "agentic-pdf/" + SpecVersion,
		SourcePages: len(pages),
	}

	body := TextToMarkdown(pages)
	glossary := BuildGlossary(pages)
	toc := buildTOC(body)

	tables := DetectTables(pages)
	attachCaptions(pages, tables)
	figures := DetectFigures(pages)

	var b []string
	b = append(b, fm.Render(), "", "# "+title, "",
		"> **For AI agents:** This is the machine-readable layer of a printed PDF document.",
		"> It mirrors the human-readable PDF content in structured markdown so you can parse",
		"> it directly without OCR or layout heuristics. The visual PDF remains unchanged.",
		"", "## Summary", "", description, "")
	if toc != "" {
		b = append(b, "## Table of Contents", "", toc, "")
	}
	b = append(b, "## Content", "", body)
	if ts := renderTables(tables); ts != "" {
		b = append(b, "## Tables", "", ts, "")
	}
	if fs := renderFigures(figures); fs != "" {
		b = append(b, "## Figures", "", fs, "")
	}
	b = append(b,
		"## Sitemap", "",
		"- [agent.md](agent.md) — this file (markdown mirror of the document)",
		"- [agent.html](agent.html) — HTML rendering of this mirror",
		"- `/` — the original visual PDF (canonical)",
		"", "## Glossary", "", glossary, "")
	return collapseBlank(strings.Join(b, "\n")), fm
}

// renderTables formats detected tables as markdown. Returns "" when none.
func renderTables(tables []Table) string {
	if len(tables) == 0 {
		return ""
	}
	var b []string
	for i, t := range tables {
		if t.Caption != "" {
			b = append(b, "**"+t.Caption+"**", "")
		} else {
			b = append(b, "**Table "+itoa(i+1)+"** _(p."+itoa(t.Page)+")_", "")
		}
		b = append(b, "| "+strings.Join(t.Header, " | ")+" |")
		b = append(b, "|"+strings.Repeat(" --- |", len(t.Header)))
		for _, row := range t.Rows {
			b = append(b, "| "+strings.Join(row, " | ")+" |")
		}
		b = append(b, "")
	}
	return strings.Join(b, "\n")
}

// renderFigures lists figure/chart captions with page references.
// Chart *pixels* stay in the visual layer; captions give agents provenance.
func renderFigures(figures []Figure) string {
	if len(figures) == 0 {
		return ""
	}
	parts := make([]string, len(figures))
	for i, f := range figures {
		parts[i] = "- **" + f.Kind + " " + f.Number + "** _(p." + itoa(f.Page) + ")_ — " + f.Title
	}
	return strings.Join(parts, "\n")
}

func buildTOC(body string) string {
	var items []string
	for _, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(line, "## ") {
			items = append(items, "- "+strings.TrimPrefix(line, "## "))
		}
	}
	if len(items) < 2 {
		return ""
	}
	return strings.Join(items, "\n")
}
