package core

import (
	"regexp"
	"sort"
	"strings"
	"unicode"
)

// Table is a detected tabular block, rendered as a markdown table.
type Table struct {
	Page    int
	Caption string // nearby "Table N: ..." caption, if found
	Header  []string
	Rows    [][]string
}

// Figure is a figure/chart/exhibit caption detected in the source document.
type Figure struct {
	Page   int
	Kind   string // "Figure", "Chart", "Exhibit", "Graph"
	Number string
	Title  string
}

// Detection thresholds (points; 1pt = 1/72 inch).
const (
	rowYTolerance = 3.0 // words within this vertical distance share a row
	colClusterTol = 8.0 // X positions within this distance are the same column
	minColGap     = 18.0
	minTableRows  = 3
	minCols       = 2
)

var (
	captionRE  = regexp.MustCompile(`^(Figure|Fig|Chart|Exhibit|Graph|Table)\s+(\d+)\s*[:.\-–—]?\s*(.*)`)
	pureNumber = regexp.MustCompile(`^[+−–-]?\d[\d.,]*\s*%?$`)
)

// DetectTables scans positioned words for runs of rows that consistently
// split into the same two-or-more columns — the signature of a rendered table.
func DetectTables(pages []PageText) []Table {
	var tables []Table
	for _, pg := range pages {
		if len(pg.Words) < minTableRows*minCols {
			continue
		}
		rows := groupRowsByY(pg.Words)
		var run [][][]WordPos // consecutive multi-column rows
		gap := 0              // intervening single-column rows tolerated
		flush := func() {
			if t := buildTable(run, pg.Page, captionText(pages, pg.Page)); t != nil {
				// an all-numeric grid with no caption is a chart's plot area
				if !(allCellsNumeric(t) && strings.TrimSpace(captionText(pages, pg.Page)) == "") {
					tables = append(tables, *t)
				}
			}
			run = nil
			gap = 0
		}
		for _, row := range rows {
			if segs := splitRow(row, minColGap); len(segs) >= minCols {
				run = append(run, segs)
				gap = 0
			} else if len(run) > 0 && gap < 2 {
				// a wrapped line inside a cell briefly breaks the column
				// pattern; tolerate a couple of those before giving up
				gap++
			} else {
				flush()
			}
		}
		flush()
	}
	return tables
}

// DetectFigures collects figure/chart/exhibit captions. Charts in PDFs are
// vector or raster drawings whose pixels cannot be conveyed as text; their
// captions and locations can be, and give agents provenance.
func DetectFigures(pages []PageText) []Figure {
	var figures []Figure
	for _, pg := range pages {
		for _, line := range pg.Lines {
			m := captionRE.FindStringSubmatch(line)
			if m == nil || m[1] == "Table" || strings.HasPrefix(line, "Table") {
				continue
			}
			kind := m[1]
			if kind == "Fig" {
				kind = "Figure"
			}
			title := strings.TrimSpace(m[3])
			if title == "" {
				continue
			}
			figures = append(figures, Figure{Page: pg.Page, Kind: kind, Number: m[2], Title: title})
		}
	}
	return figures
}

// allCellsNumeric reports whether every cell in the table is a bare number.
func allCellsNumeric(t *Table) bool {
	for _, row := range append([][]string{t.Header}, t.Rows...) {
		for _, cell := range row {
			if !pureNumber.MatchString(strings.TrimSpace(cell)) {
				return false
			}
		}
	}
	return true
}

func tableHasDigits(cells [][]string) bool {
	for _, row := range cells {
		for _, cell := range row {
			if strings.ContainsFunc(cell, unicode.IsDigit) {
				return true
			}
		}
	}
	return false
}

// buildTable turns a run of consistently-columned rows into a Table,
// or nil if the run does not hold together as a table.
func buildTable(run [][][]WordPos, page int, pageCaptions string) *Table {
	if len(run) < minTableRows {
		return nil
	}

	// The table's column count is the median segment count across the run.
	counts := make([]int, len(run))
	for i, row := range run {
		counts[i] = len(row)
	}
	sort.Ints(counts)
	nCols := counts[len(counts)/2]
	if nCols < minCols {
		return nil
	}

	// Column anchors: take the reference row (first with exactly nCols
	// segments) and derive a horizontal band per column from its extents.
	// Bands tolerate left/right/center cell alignment across rows.
	var ref []([]WordPos)
	for _, row := range run {
		if len(row) == nCols {
			ref = row
			break
		}
	}
	if ref == nil {
		return nil
	}
	starts := make([]float64, nCols)
	ends := make([]float64, nCols)
	for c, seg := range ref {
		starts[c] = seg[0].X
		last := seg[len(seg)-1]
		ends[c] = last.X + last.W
	}
	bounds := make([]float64, nCols+1)
	bounds[0] = -1e9
	bounds[nCols] = 1e9
	for c := 0; c+1 < nCols; c++ {
		bounds[c+1] = (ends[c] + starts[c+1]) / 2
	}
	segCenter := func(seg []WordPos) (float64, bool) {
		first := seg[0]
		last := seg[len(seg)-1]
		return (first.X + last.X + last.W) / 2, true
	}

	// Keep only rows whose segments map cleanly onto the column bands.
	var cells [][]string
	for _, row := range run {
		rowCells := make([]string, nCols)
		ok := true
		for _, seg := range row {
			cx, _ := segCenter(seg)
			best := -1
			for c := 0; c < nCols; c++ {
				if cx >= bounds[c] && cx <= bounds[c+1] {
					best = c
					break
				}
			}
			if best < 0 { // fragment straddles a column boundary — not a grid
				ok = false
				break
			}
			if rowCells[best] != "" {
				rowCells[best] += " "
			}
			rowCells[best] += joinSegWords(seg)
		}
		// every column must have content in every kept row — otherwise it is
		// probably wrapped prose rather than a grid
		for _, cell := range rowCells {
			if cell == "" {
				ok = false
				break
			}
		}
		if ok {
			for i, cell := range rowCells {
				rowCells[i] = strings.NewReplacer("ﬁ", "fi", "ﬂ", "fl", "ﬀ", "ff", "ﬃ", "ffi", "ﬄ", "ffl").Replace(cell)
			}
			cells = append(cells, rowCells)
		}
	}
	if len(cells) < minTableRows {
		return nil
	}

	// Chart axis labels often form pseudo-tables with many repeated rows;
	// real tables are mostly unique. Require ≥70% unique rows.
	seen := map[string]bool{}
	unique := 0
	for _, row := range cells {
		key := strings.Join(row, "\x00")
		if !seen[key] {
			seen[key] = true
			unique++
		}
	}
	if unique*100 < len(cells)*70 {
		return nil
	}

	// Drop chart artifacts:
	//  1. a first column of bare numbers is a graph's value axis
	//  2. very wide columns are wrapped prose flowing past a chart
	//  3. body rows starting lowercase are sentences wrapped around a figure
	lowerStart := 0
	for _, row := range cells[1:] {
		if len(row[0]) > 0 {
			r := []rune(row[0])[0]
			if unicode.IsLower(r) {
				lowerStart++
			}
		}
	}
	if len(cells) > 1 && lowerStart*100 > len(cells[1:])*50 && nCols == minCols {
		return nil
	}

	// Chart legends: a row repeating identical cells (e.g. "Monthly | Monthly")
	// is figure furniture, not data — even if other rows carry numbers.
	for _, row := range cells[1:] {
		same := true
		for i := 1; i < len(row); i++ {
			if !strings.EqualFold(row[i], row[0]) {
				same = false
				break
			}
		}
		if same && len(row) > 1 {
			return nil
		}
	}
	// Footnote-marker grids ("t t | t | t t t"): dominated by 1–2 char cells.
	short := 0
	total := 0
	for _, row := range cells[1:] {
		for _, cell := range row {
			total++
			if len(strings.TrimSpace(cell)) <= 2 {
				short++
			}
		}
	}
	if total > 0 && short*100 > total*60 {
		return nil
	}
	// Two-column, digit-free, uncaptioned blocks are page-layout artifacts
	// (wrapped prose beside a figure), not data tables.
	if nCols == minCols && !tableHasDigits(cells) && strings.TrimSpace(pageCaptions) == "" {
		return nil
	}
	for c := 0; c < nCols; c++ {
		total := 0
		numeric := true
		for _, row := range append([][]string{cells[0]}, cells[1:]...) {
			cell := strings.TrimSpace(row[c])
			total += len(cell)
			if !pureNumber.MatchString(cell) {
				numeric = false
			}
		}
		if numeric && c == 0 {
			return nil
		}
		if total/len(cells) > 90 { // very wide columns = wrapped prose
			return nil
		}
	}

	t := &Table{Page: page, Header: cells[0]}
	for _, row := range cells[1:] {
		r := make([]string, nCols)
		copy(r, row)
		t.Rows = append(t.Rows, r)
	}
	return t
}

// captionText returns the "Table N: …" captions found on a page, if any.
func captionText(pages []PageText, page int) string {
	var caps []string
	for _, pg := range pages {
		if pg.Page != page {
			continue
		}
		for _, line := range pg.Lines {
			if m := captionRE.FindStringSubmatch(line); m != nil && m[1] == "Table" && strings.TrimSpace(m[3]) != "" {
				caps = append(caps, strings.TrimSpace(m[0]))
			}
		}
	}
	return strings.Join(caps, "; ")
}

// attachCaptions associates "Table N: …" caption lines with detected tables.
func attachCaptions(pages []PageText, tables []Table) {
	captionsByPage := map[int][]string{}
	for _, pg := range pages {
		for _, line := range pg.Lines {
			if m := captionRE.FindStringSubmatch(line); m != nil && m[1] == "Table" && strings.TrimSpace(m[3]) != "" {
				captionsByPage[pg.Page] = append(captionsByPage[pg.Page], strings.TrimSpace(m[0]))
			}
		}
	}
	used := map[int]int{} // page -> next unused caption index
	for i := range tables {
		pg := tables[i].Page
		if caps := captionsByPage[pg]; len(caps) > used[pg] {
			tables[i].Caption = caps[used[pg]]
			used[pg]++
		}
	}
}

// joinSegWords concatenates a column segment's positioned fragments into one
// cell string. Whitespace fragments and inter-fragment gaps become spaces;
// per-glyph fragments from letter-spaced fonts concatenate directly.
func joinSegWords(seg []WordPos) string {
	var b strings.Builder
	var prev WordPos
	for i, w := range seg {
		raw := w.S
		if strings.TrimSpace(raw) == "" {
			if i > 0 && strings.TrimSpace(b.String()) != "" {
				b.WriteString(" ")
			}
			prev = w
			continue
		}
		if i > 0 && strings.TrimSpace(b.String()) != "" {
			gap := w.X - (prev.X + prev.W)
			if strings.HasPrefix(raw, " ") ||
				strings.HasSuffix(prev.S, " ") ||
				(gap > 1.5 && !strings.HasSuffix(prev.S, "-") && !strings.HasPrefix(raw, "-")) {
				b.WriteString(" ")
			}
		}
		b.WriteString(strings.TrimSpace(raw))
		prev = w
	}
	return strings.TrimSpace(b.String())
}

func groupRowsByY(words []WordPos) [][]WordPos {
	sorted := make([]WordPos, len(words))
	copy(sorted, words)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Y > sorted[j].Y })
	var rows [][]WordPos
	var cur []WordPos
	curY := -1e9
	for _, w := range sorted {
		if cur != nil && curY-w.Y > rowYTolerance {
			rows = append(rows, cur)
			cur = nil
		}
		if cur == nil {
			curY = w.Y
		}
		cur = append(cur, w)
	}
	if cur != nil {
		rows = append(rows, cur)
	}
	return rows
}

// splitRow breaks a row's words into column segments wherever the horizontal
// gap between consecutive fragments exceeds gap.
func splitRow(row []WordPos, gap float64) [][]WordPos {
	sorted := make([]WordPos, len(row))
	copy(sorted, row)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].X < sorted[j].X })
	var segs [][]WordPos
	cur := []WordPos{sorted[0]}
	for _, w := range sorted[1:] {
		prev := cur[len(cur)-1]
		if w.X-(prev.X+prev.W) > gap {
			segs = append(segs, cur)
			cur = nil
		}
		cur = append(cur, w)
	}
	segs = append(segs, cur)
	return segs
}

func abs(f float64) float64 {
	if f < 0 {
		return -f
	}
	return f
}
