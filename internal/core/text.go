package core

import (
	"bytes"
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"

	"github.com/ledongthuc/pdf"
)

type PageText struct {
	Page  int
	Lines []string
}

// ExtractPages pulls per-page text out of a PDF. The ledongthuc/pdf reader
// preserves line structure, which our heuristics rely on.
func ExtractPages(pdfBytes []byte) ([]PageText, error) {
	rd := bytes.NewReader(pdfBytes)
	reader, err := pdf.NewReader(rd, int64(len(pdfBytes)))
	if err != nil {
		return nil, fmt.Errorf("opening PDF: %w", err)
	}
	var pages []PageText
	for i := 1; i <= reader.NumPage(); i++ {
		page := reader.Page(i)
		txt, err := page.GetPlainText(nil)
		if err != nil {
			return nil, fmt.Errorf("page %d: %w", i+1, err)
		}
		pt := PageText{Page: i + 1}
		for _, line := range strings.Split(txt, "\n") {
			line = strings.Join(strings.Fields(line), " ")
			if line != "" {
				pt.Lines = append(pt.Lines, line)
			} else if len(pt.Lines) > 0 && pt.Lines[len(pt.Lines)-1] != "" {
				pt.Lines = append(pt.Lines, "")
			}
		}
		pages = append(pages, pt)
	}
	if pages == nil {
		return nil, fmt.Errorf("no pages found")
	}
	return pages, nil
}

var bulletRE = regexp.MustCompile(`^([-*•·‣◦]|\d+[.)])\s+`)
var sectionHintRE = regexp.MustCompile(`(?i)^(chapter|section|appendix|part)\b`)

const headingMaxLen = 80

// TextToMarkdown converts extracted page text into structured markdown using
// the same heuristics as the original implementation: short standalone lines
// become headings, bullets become lists, the rest paragraphs.
func TextToMarkdown(pages []PageText) string {
	var out []string
	var para []string
	flush := func() {
		if len(para) > 0 {
			out = append(out, strings.Join(para, " "), "")
			para = nil
		}
	}

	for _, pg := range pages {
		out = append(out, fmt.Sprintf("### Page %d", pg.Page), "")
		for i, line := range pg.Lines {
			if line == "" {
				flush()
				if len(out) > 0 && out[len(out)-1] != "" {
					out = append(out, "")
				}
				continue
			}
			prev, next := "", ""
			if i > 0 {
				prev = pg.Lines[i-1]
			}
			if i+1 < len(pg.Lines) {
				next = pg.Lines[i+1]
			}

			if bulletRE.MatchString(line) {
				flush()
				out = append(out, "- "+bulletRE.ReplaceAllString(line, ""))
				if !bulletRE.MatchString(next) {
					out = append(out, "")
				}
				continue
			}

			allUpper := line == strings.ToUpper(line) && strings.IndexFunc(line, unicode.IsLetter) >= 0
			last := rune(line[len(line)-1])
			isHeadingish := len(line) <= headingMaxLen &&
				!strings.ContainsRune(".,;:", last) &&
				(i == 0 || prev == "" || next == "" || allUpper)

			switch {
			case isHeadingish && allUpper:
				flush()
				out = append(out, "## "+TitleCase(line)+fmt.Sprintf(" _(p.%d)_", pg.Page), "")
			case isHeadingish && !strings.HasSuffix(line, "."):
				flush()
				out = append(out, "#### "+line, "")
			default:
				para = append(para, line)
				if strings.HasSuffix(line, ".") && (next == "" || bulletRE.MatchString(next)) {
					flush()
				}
			}
		}
		flush()
	}
	return collapseBlank(strings.Join(out, "\n"))
}

func collapseBlank(s string) string {
	for strings.Contains(s, "\n\n\n") {
		s = strings.ReplaceAll(s, "\n\n\n", "\n\n")
	}
	return s
}

func TitleCase(s string) string {
	words := strings.Fields(strings.ToLower(s))
	for i, w := range words {
		r := []rune(w)
		if len(r) > 0 {
			r[0] = unicode.ToUpper(r[0])
			words[i] = string(r)
		}
	}
	return strings.Join(words, " ")
}

// DeriveSummary builds a description from the first content lines.
func DeriveSummary(pages []PageText, maxChars int) string {
	var b strings.Builder
outer:
	for _, pg := range pages {
		for _, line := range pg.Lines {
			if b.Len() > 0 {
				b.WriteString(" ")
			}
			b.WriteString(line)
			if b.Len() >= maxChars {
				break outer
			}
		}
	}
	text := strings.TrimSpace(b.String())
	if text == "" {
		return "(no extractable text)"
	}
	if len(text) <= maxChars {
		return text
	}
	cut := text[:maxChars-1]
	if idx := strings.LastIndex(cut, " "); idx > 0 {
		cut = cut[:idx]
	}
	return cut + "…"
}

// GuessTitle takes the first plausible title-looking line.
func GuessTitle(pages []PageText) string {
	for _, pg := range pages {
		max := 5
		if len(pg.Lines) < max {
			max = len(pg.Lines)
		}
		for _, line := range pg.Lines[:max] {
			if len(line) >= 3 && len(line) <= headingMaxLen && !sectionHintRE.MatchString(line) {
				return TitleCase(line)
			}
		}
	}
	return ""
}

// ToPdf converts arbitrary printable input to PDF bytes. Already-PDF input is
// passed through; anything else goes through the platform's cups filter chain.
func ToPdf(inputPath string) ([]byte, error) {
	data, err := osReadFile(inputPath)
	if err != nil {
		return nil, err
	}
	ext := strings.ToLower(filepath.Ext(inputPath))
	if ext == ".pdf" || bytes.HasPrefix(data, []byte("%PDF-")) {
		return data, nil
	}
	return cupsFilterToPDF(inputPath)
}

func cupsFilterToPDF(inputPath string) ([]byte, error) {
	candidates := []string{"/usr/sbin/cupsfilter", "/usr/lib/cups/server/cupsfilter", "cupsfilter"}
	var cmd *exec.Cmd
	for _, c := range candidates {
		if filepath.IsAbs(c) {
			if _, err := osStat(c); err == nil {
				cmd = exec.Command(c, "-o", "media=A4", "-o", "fit-to-page", inputPath)
				break
			}
			continue
		}
		if p, err := exec.LookPath(c); err == nil {
			cmd = exec.Command(p, "-o", "media=A4", "-o", "fit-to-page", inputPath)
			break
		}
	}
	if cmd == nil {
		return nil, fmt.Errorf("cupsfilter not found: cannot convert %q to PDF; provide a PDF instead", inputPath)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		detail := strings.TrimSpace(stderr.String())
		if detail == "" {
			detail = err.Error()
		}
		return nil, fmt.Errorf("cupsfilter could not convert %q: %s", inputPath, detail)
	}
	if !bytes.HasPrefix(out, []byte("%PDF-")) {
		return nil, fmt.Errorf("cupsfilter produced no PDF for %q", inputPath)
	}
	return out, nil
}
