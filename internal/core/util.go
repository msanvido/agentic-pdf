package core

import (
	"regexp"
	"strconv"
	"time"
)

func regexpMustCompile(s string) *regexp.Regexp { return regexp.MustCompile(s) }
func itoa(n int) string                         { return strconv.Itoa(n) }

var (
	boldRE = regexp.MustCompile(`\*\*([^*]+)\*\*`)
	emRE   = regexp.MustCompile(`(^|\s)_([^_]+)_(\s|$)`)
	codeRE = regexp.MustCompile("`([^`]+)`")
	linkRE = regexp.MustCompile(`\[([^\]]+)\]\(([^)]+)\)`)
)

// Now is a small indirection for testability.
type Now struct{ Time time.Time }

func htmlTag(tag, inner string) string {
	return "<" + tag + ">" + inner + "</" + tag + ">"
}
