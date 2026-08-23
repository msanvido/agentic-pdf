package cli

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/msanvido/agentic-pdf/internal/core"
)

// Print converts an input file to an agentic PDF (embeds the hidden layer).
func Print(input, output, title, canonical string, withHTML bool) error {
	if output == "" {
		ext := filepath.Ext(input)
		output = input[:len(input)-len(ext)] + ".agentic.pdf"
	}
	fmt.Fprintf(os.Stderr, "⏳ printing %s → %s\n", input, output)

	pdfBytes, err := core.ToPdf(input)
	if err != nil {
		return err
	}
	if canonical == "" {
		if abs, aerr := filepath.Abs(input); aerr == nil {
			canonical = "file://" + abs
		}
	}
	pages, err := core.ExtractPages(pdfBytes)
	if err != nil {
		return err
	}
	result, err := core.InjectAgentLayer(pdfBytes, pages, title, "", canonical, "", withHTML)
	if err != nil {
		return err
	}
	if err := os.WriteFile(output, result, 0o644); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "✅ wrote %s (%d page(s), agentic layer embedded)\n", output, len(pages))
	return nil
}

// Read prints the agent layer in the requested format.
func Read(input string, raw, htmlMode, meta bool) error {
	data, err := os.ReadFile(input)
	if err != nil {
		return err
	}
	atts, err := core.ReadAttachments(data)
	if err != nil {
		return fmt.Errorf("reading attachments: %w", err)
	}
	var md, html []byte
	for _, a := range atts {
		switch a.Name {
		case core.AgentMD:
			md = a.Data
		case core.AgentHTML:
			html = a.Data
		}
	}
	if md == nil && html == nil {
		return fmt.Errorf("no agentic layer found in %s — is this an agentic-pdf document?", input)
	}

	switch {
	case meta:
		props, _ := core.ReadProperties(data)
		fm, _ := core.ParseFrontmatter(string(md))
		fmt.Println(toJSON(props))
		fmt.Println("---")
		fmt.Println(toJSON(fm))
	case raw:
		os.Stdout.Write(md)
	case htmlMode:
		if html != nil {
			os.Stdout.Write(html)
		} else {
			fmt.Print(core.MarkdownToHTML(string(md)))
		}
	default:
		s := string(md)
		if m := frontmatterBlock(s); m != "" {
			fmt.Println(m)
		}
		fmt.Print(trimFrontmatter(s))
	}
	return nil
}

// Check reports whether a PDF carries an agentic layer; exits 1 if not.
func Check(input string) error {
	data, err := os.ReadFile(input)
	if err != nil {
		return err
	}
	atts, err := core.ReadAttachments(data)
	if err != nil {
		return err
	}
	hasMD, hasHTML, hasLLMS := false, false, false
	var md []byte
	for _, a := range atts {
		switch a.Name {
		case core.AgentMD:
			hasMD, md = true, a.Data
		case core.AgentHTML:
			hasHTML = true
		case core.LLMSTxt:
			hasLLMS = true
		}
	}
	if !hasMD && !hasHTML {
		fmt.Printf("%s: no agentic layer\n", input)
		os.Exit(1)
	}
	props, _ := core.ReadProperties(data)
	fm, _ := core.ParseFrontmatter(string(md))
	title := fm["title"]
	if title == "" {
		title = props["Title"]
	}
	fmt.Printf("%s: agentic layer present\n", input)
	fmt.Printf("  spec:     %s\n", orUnknown(props[core.InfoKey]))
	fmt.Printf("  title:    %s\n", orUnknown(title))
	fmt.Printf("  updated:  %s\n", orUnknown(fm["last_updated"]))
	fmt.Printf("  agent.md: %s\n", yesNo(hasMD))
	fmt.Printf("  agent.html: %s\n", yesNo(hasHTML))
	fmt.Printf("  llms.txt: %s\n", yesNo(hasLLMS))
	return nil
}

// DebugTables prints detected tables for a PDF (development aid).
func DebugTables(input string) error {
	data, err := os.ReadFile(input)
	if err != nil {
		return err
	}
	pages, err := core.ExtractPages(data)
	if err != nil {
		return err
	}
	for i, pg := range pages {
		fmt.Printf("page %d: %d words, %d lines\n", pg.Page, len(pg.Words), len(pg.Lines))
		if i > 1 && pg.Page > 3 {
			break
		}
	}
	tables := core.DetectTables(pages)
	fmt.Printf("detected %d table(s)\n", len(tables))
	for _, t := range tables {
		fmt.Printf("- p%d caption=%q header=%v rows=%d\n", t.Page, t.Caption, t.Header, len(t.Rows))
	}
	return nil
}

func orUnknown(s string) string {
	if s == "" {
		return "?"
	}
	return s
}
func yesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

func frontmatterBlock(md string) string {
	if !bytes.HasPrefix([]byte(md), []byte("---\n")) {
		return ""
	}
	end := strings.Index(md[4:], "\n---")
	if end < 0 {
		return ""
	}
	return "---" + md[3:4+end+4] + "\n"
}

func trimFrontmatter(md string) string {
	if !strings.HasPrefix(md, "---\n") {
		return md
	}
	end := strings.Index(md[4:], "\n---")
	if end < 0 {
		return md
	}
	rest := md[4+end+4:]
	return strings.TrimPrefix(rest, "\n")
}
