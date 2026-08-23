package core

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

// InjectAgentLayer embeds the agentic layer into a PDF:
// agent.md + agent.html attachments, Title/Subject metadata, and the
// AgentReadability / CanonicalSource custom Info keys.
func InjectAgentLayer(pdfBytes []byte, pages []PageText, title, author, description, canonical, docVersion string, withHTML bool) ([]byte, error) {
	markdown, fm := BuildAgentMarkdown(pages, title, description, canonical, docVersion, author)

	dir, err := os.MkdirTemp("", "agentic-pdf-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)

	mdPath := filepath.Join(dir, AgentMD)
	if err := os.WriteFile(mdPath, []byte(markdown), 0o644); err != nil {
		return nil, err
	}
	files := []string{mdPath}
	if withHTML {
		htmlPath := filepath.Join(dir, AgentHTML)
		html := fmt.Sprintf(`<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="description" content="Agentic layer of a PDF document — machine-readable mirror.">
<title>%s — Agentic Layer</title>
</head>
<body>
%s
</body>
</html>`, escapeAttr(fm.Title), MarkdownToHTML(markdown))
		if err := os.WriteFile(htmlPath, []byte(html), 0o644); err != nil {
			return nil, err
		}
		files = append(files, htmlPath)
	}

	conf := model.NewDefaultConfiguration()

	// Pass 0: drop any pre-existing agent layer (double-printing an agentic
	// PDF must refresh the layer, not stack a stale duplicate next to it).
	// pdfcpu's RemoveAttachments panics on some name trees unless the
	// document is normalized (validated + optimized) first.
	normalized, nerr := func() (out []byte, err error) {
		defer func() {
			if p := recover(); p != nil {
				err = fmt.Errorf("normalize failed: %v", p)
			}
		}()
		ctxIn, err := api.ReadAndValidate(bytes.NewReader(pdfBytes), conf)
		if err != nil {
			return nil, err
		}
		if err := api.OptimizeContext(ctxIn); err != nil {
			return nil, err
		}
		var buf bytes.Buffer
		if err := api.WriteContext(ctxIn, &buf); err != nil {
			return nil, err
		}
		return buf.Bytes(), nil
	}()
	atts, _ := ReadAttachments(normalized)
	cleaned := pdfBytes
	if nerr == nil && hasAnyAgentAttachment(normalized) {
		// Only request names that exist — pdfcpu panics on absent ones.
		var toRemove []string
		for _, name := range []string{AgentMD, AgentHTML, LLMSTxt} {
			for _, a := range atts {
				if a.Name == name {
					toRemove = append(toRemove, name)
					break
				}
			}
		}
		remove := func(b []byte, names []string) (out []byte, err error) {
			defer func() {
				if p := recover(); p != nil {
					err = fmt.Errorf("remove failed: %v", p)
				}
			}()
			var outClean bytes.Buffer
			if err := api.RemoveAttachments(bytes.NewReader(b), &outClean,
				names, conf); err != nil {
				return nil, err
			}
			return outClean.Bytes(), nil
		}
		if outClean, rerr := remove(normalized, toRemove); rerr == nil {
			cleaned = outClean
		}
	}

	// Pass 1: attach files.
	var out bytes.Buffer
	rs := bytes.NewReader(cleaned)
	if err := api.AddAttachments(rs, &out, files, true, conf); err != nil {
		return nil, fmt.Errorf("attaching agent layer: %w", err)
	}

	// Pass 2: keywords.
	var outK bytes.Buffer
	if err := api.AddKeywords(bytes.NewReader(out.Bytes()), &outK, MarkerKeywords, conf); err != nil {
		return nil, fmt.Errorf("setting keywords: %w", err)
	}

	// Pass 3: metadata + spec markers in the Info dictionary.
	props := map[string]string{
		"Title":   fm.Title,
		"Subject": fm.Description,
		InfoKey:   InfoValue,
		"Creator": "agentic-pdf",
	}
	if author != "" {
		props["Author"] = author
	}
	if canonical != "" {
		props[CanonicalKey] = canonical
	}
	var out2 bytes.Buffer
	if err := api.AddProperties(bytes.NewReader(outK.Bytes()), &out2, props, conf); err != nil {
		return nil, fmt.Errorf("setting properties: %w", err)
	}
	return out2.Bytes(), nil
}

// hasAnyAgentAttachment reports whether the PDF already carries a layer.
func hasAnyAgentAttachment(pdfBytes []byte) bool {
	atts, err := ReadAttachments(pdfBytes)
	if err != nil {
		return false
	}
	for _, a := range atts {
		if a.Name == AgentMD || a.Name == AgentHTML || a.Name == LLMSTxt {
			return true
		}
	}
	return false
}

func escapeAttr(s string) string {
	r := strings.NewReplacer("&", "&amp;", `"`, "&quot;", "<", "&lt;")
	return r.Replace(s)
}

type Attachment struct {
	Name string
	Data []byte
	Desc string
}

// ReadAttachments returns all embedded files (name -> data).
func ReadAttachments(pdfBytes []byte) ([]Attachment, error) {
	conf := model.NewDefaultConfiguration()
	dir, err := os.MkdirTemp("", "agentic-read-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)

	atts, err := api.ExtractAttachmentsRaw(bytes.NewReader(pdfBytes), dir, nil, conf)
	if err != nil {
		return nil, err
	}
	out := make([]Attachment, 0, len(atts))
	for _, a := range atts {
		if a.Reader == nil {
			continue
		}
		data, err := io.ReadAll(a.Reader)
		if err != nil {
			continue
		}
		name := a.FileName
		if name == "" {
			name = fmt.Sprintf("attachment_%d", len(out)+1)
		}
		out = append(out, Attachment{Name: name, Data: data, Desc: a.Desc})
	}
	return out, nil
}

// Properties returns the document Info dictionary entries we care about.
func ReadProperties(pdfBytes []byte) (map[string]string, error) {
	conf := model.NewDefaultConfiguration()
	return api.Properties(bytes.NewReader(pdfBytes), conf)
}

// PDFDocInfo is the subset of standard metadata we carry over.
type PDFDocInfo struct {
	Title  string
	Author string
}

// ReadPDFInfo returns the document's standard metadata (title, author, …).
func ReadPDFInfo(pdfBytes []byte) (*PDFDocInfo, error) {
	conf := model.NewDefaultConfiguration()
	info, err := api.PDFInfo(bytes.NewReader(pdfBytes), "", nil, false, conf)
	if err != nil || info == nil {
		return nil, err
	}
	return &PDFDocInfo{Title: info.Title, Author: info.Author}, nil
}
