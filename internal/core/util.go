package core

import (
	"regexp"
	"strconv"
)

var (
	boldRE = regexp.MustCompile(`\*\*([^*]+)\*\*`)
	emRE   = regexp.MustCompile(`(^|\s)_([^_]+)_(\s|$)`)
	codeRE = regexp.MustCompile("`([^`]+)`")
	linkRE = regexp.MustCompile(`\[([^\]]+)\]\(([^)]+)\)`)

	termRE     = regexp.MustCompile(`\b[A-Z][A-Za-z0-9]*(-[A-Za-z0-9]+)*\b`)
	allUpperRE = regexp.MustCompile(`^[A-Z]{2,}$`)
)

func itoa(n int) string { return strconv.Itoa(n) }

func htmlTag(tag, inner string) string {
	return "<" + tag + ">" + inner + "</" + tag + ">"
}
