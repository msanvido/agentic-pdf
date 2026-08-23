package core

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// ExtractPagesAuto extracts text using the best extractor available:
// poppler's pdftotext -bbox-layout (word coordinates, excellent line
// fidelity) when installed, falling back to the built-in Go extractor.
func ExtractPagesAuto(pdfPath string, pdfBytes []byte) ([]PageText, error) {
	if pdftotext, err := exec.LookPath("pdftotext"); err == nil {
		pages, err := extractViaPoppler(pdftotext, pdfPath, pdfBytes)
		if err == nil && len(pages) > 0 {
			return pages, nil
		}
	}
	return ExtractPages(pdfBytes)
}

var (
	wordTagRE  = regexp.MustCompile(`<word xMin="([\d.]+)" yMin="([\d.]+)" xMax="([\d.]+)" yMax="([\d.]+)"[^>]*>(.*?)</word>`)
	pageOpenRE = regexp.MustCompile(`<page [^>]*>`)
)

type popWord struct {
	S          string
	X, Y, W, H float64
}

// extractViaPoppler runs `pdftotext -bbox-layout` and converts the XHTML
// output into PageText (lines reconstructed from word boxes; words kept for
// table/figure detection).
func extractViaPoppler(pdftotext, pdfPath string, pdfBytes []byte) ([]PageText, error) {
	tmpIn := pdfPath
	tmpOut := ""
	cleanup := func() {}
	if tmpIn == "" || !fileExists(tmpIn) {
		f, err := os.CreateTemp("", "agentic-pdf-in-*.pdf")
		if err != nil {
			return nil, err
		}
		defer os.Remove(f.Name())
		if _, err := f.Write(pdfBytes); err != nil {
			f.Close()
			return nil, err
		}
		f.Close()
		tmpIn = f.Name()
	}
	htmlFile, err := os.CreateTemp("", "agentic-pdf-out-*.html")
	if err != nil {
		return nil, err
	}
	tmpOut = htmlFile.Name()
	htmlFile.Close()
	cleanup = func() { os.Remove(tmpOut) }
	defer cleanup()

	cmd := exec.Command(pdftotext, "-bbox-layout", tmpIn, tmpOut)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("pdftotext: %s", strings.TrimSpace(stderr.String()))
	}

	raw, err := os.ReadFile(tmpOut)
	if err != nil {
		return nil, err
	}
	return parsePopplerBbox(string(raw))
}

func fileExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}

// parsePopplerBbox converts pdftotext -bbox-layout XHTML into PageText.
// Poppler emits <flow><line><word/>… per page; y grows downward (top-based),
// which is fine — row grouping only needs same-row words to share Y.
func parsePopplerBbox(xml string) ([]PageText, error) {
	var pages []PageText
	pageNo := 0

	for _, block := range splitTopLevel(xml, pageOpenRE) {
		if !strings.Contains(block, "<line") && !strings.Contains(block, "<word") {
			continue
		}
		pageNo++
		pt := PageText{Page: pageNo}

		for _, lineBlock := range splitByRegex(block, `<line [^>]*>`) {
			words := parseWords(lineBlock)
			if len(words) == 0 {
				continue
			}
			pt.Words = append(pt.Words, words...)
			line := joinWordsToLine(words)
			if line != "" {
				pt.Lines = append(pt.Lines, line)
			} else if len(pt.Lines) > 0 && pt.Lines[len(pt.Lines)-1] != "" {
				pt.Lines = append(pt.Lines, "")
			}
		}
		pages = append(pages, pt)
	}
	if len(pages) == 0 {
		return nil, fmt.Errorf("poppler produced no pages")
	}
	return pages, nil
}

func parseWords(s string) []WordPos {
	locs := wordTagRE.FindAllStringSubmatchIndex(s, -1)
	words := make([]WordPos, 0, len(locs))
	for _, loc := range locs {
		x, _ := strconv.ParseFloat(s[loc[2]:loc[3]], 64)
		y, _ := strconv.ParseFloat(s[loc[4]:loc[5]], 64)
		x2, _ := strconv.ParseFloat(s[loc[6]:loc[7]], 64)
		text := htmlUnescape(s[loc[10]:loc[11]])
		text = htmlUnescape(text)
		if strings.TrimSpace(text) == "" {
			continue
		}
		words = append(words, WordPos{
			S: strings.TrimSpace(text),
			X: x,
			Y: y,
			W: x2 - x,
		})
	}
	sort.Slice(words, func(i, j int) bool { return words[i].X < words[j].X })
	return words
}

// joinWordsToLine reconstructs a text line from positioned words.
func joinWordsToLine(words []WordPos) string {
	sorted := make([]WordPos, len(words))
	copy(sorted, words)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Y > sorted[j].Y })
	// group by row first (a <line> may contain wrapped sub-rows in tables)
	var out []string
	var cur []WordPos
	prevY := -1e9
	flushRow := func() {
		if len(cur) == 0 {
			return
		}
		row := make([]WordPos, len(cur))
		copy(row, cur)
		sort.Slice(row, func(i, j int) bool { return row[i].X < row[j].X })
		var b []byte
		for k, w := range row {
			if k > 0 {
				gap := w.X - (row[k-1].X + row[k-1].W)
				if gap > 1.5 {
					b = append(b, ' ')
				}
			}
			b = append(b, w.S...)
		}
		out = append(out, string(b))
		cur = nil
	}
	for _, w := range sorted {
		if cur != nil && prevY-w.Y > 2.0 {
			flushRow()
		}
		if cur == nil {
			prevY = w.Y
		}
		cur = append(cur, w)
	}
	flushRow()

	result := ""
	for _, l := range out {
		l = strings.TrimSpace(l)
		if l == "" {
			continue
		}
		if result != "" {
			result += "\n"
		}
		result += l
	}
	return result
}

func splitByRegex(s, pattern string) []string {
	re := regexp.MustCompile(pattern)
	locs := re.FindAllStringIndex(s, -1)
	if len(locs) == 0 {
		return nil
	}
	var parts []string
	for i, l := range locs {
		end := len(s)
		if i+1 < len(locs) {
			end = locs[i+1][0]
		}
		parts = append(parts, s[l[0]:end])
	}
	return parts
}

// splitTopLevel splits xml content at each regex match, keeping the tail.
func splitTopLevel(xml string, re *regexp.Regexp) []string {
	locs := re.FindAllStringIndex(xml, -1)
	if len(locs) == 0 {
		return []string{xml}
	}
	var parts []string
	for i, l := range locs {
		start := l[0]
		end := len(xml)
		if i+1 < len(locs) {
			end = locs[i+1][0]
		}
		parts = append(parts, xml[start:end])
	}
	return parts
}

var entityReplacer = strings.NewReplacer("&lt;", "<", "&gt;", ">", "&quot;", `"`, "&apos;", "'", "&#39;", "'")

func htmlUnescape(s string) string {
	s = strings.ReplaceAll(s, "&amp;", "&")
	return entityReplacer.Replace(s)
}
