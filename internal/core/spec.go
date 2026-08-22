package core

import (
	"fmt"
	"strings"
)

const (
	SpecVersion = "1.0"
	AgentMD     = "agent.md"
	AgentHTML   = "agent.html"
	LLMSTxt     = "llms.txt"

	InfoKey      = "AgentReadability"
	InfoValue    = "agentic-pdf-spec/" + SpecVersion
	CanonicalKey = "CanonicalSource"
)

// MarkerKeywords are written to the PDF Keywords field so corpus-wide
// indexing can discover agentic files.
var MarkerKeywords = []string{"agent-readable", "agentic-pdf", "llm-readable"}

type Frontmatter struct {
	Title       string
	Description string
	DocVersion  string
	LastUpdated string
	Canonical   string
	Generator   string
	SourcePages int
}

func (fm Frontmatter) Render() string {
	var b strings.Builder
	b.WriteString("---\n")
	fmt.Fprintf(&b, "title: %q\n", fm.Title)
	fmt.Fprintf(&b, "description: %q\n", fm.Description)
	fmt.Fprintf(&b, "doc_version: %q\n", fm.DocVersion)
	fmt.Fprintf(&b, "last_updated: %q\n", fm.LastUpdated)
	if fm.Canonical != "" {
		fmt.Fprintf(&b, "canonical: %q\n", fm.Canonical)
	}
	if fm.SourcePages > 0 {
		fmt.Fprintf(&b, "source_pages: %d\n", fm.SourcePages)
	}
	fmt.Fprintf(&b, "generator: %q\n", fm.Generator)
	b.WriteString("---\n")
	return b.String()
}

// ParseFrontmatter splits a leading YAML frontmatter block from the body.
func ParseFrontmatter(md string) (map[string]string, string) {
	fm := map[string]string{}
	s := strings.TrimPrefix(md, "---\n")
	if s == md {
		return fm, md
	}
	end := strings.Index(s, "\n---")
	if end < 0 {
		return fm, md
	}
	for _, line := range strings.Split(s[:end], "\n") {
		k, v, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		v = strings.TrimSpace(v)
		v = strings.Trim(v, `"`)
		fm[strings.TrimSpace(k)] = v
	}
	body := s[end+4:]
	body = strings.TrimPrefix(body, "\n")
	return fm, body
}
