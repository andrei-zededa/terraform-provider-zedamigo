package undent

import (
	"regexp"
	"strings"
)

var (
	whitespaceOnly    = regexp.MustCompile("(?m)^[ \t]+$")
	leadingWhitespace = regexp.MustCompile("(?m)(^[ \t]*)(?:[^ \t\n])")

	realBacktick = "`"

	// FakeBacktick is the backtick standin character that will be replaced
	// with a real backtick when undending text for use as Markdown, since
	// in Go multiline strings cannot contain the real backtick char.
	FakeBacktick = "|"
)

// It does the "undent" to the text, it removes any common leading whitespace
// from every line in the text.
//
// This can be used to make multiline strings to line up with the left edge of
// the display, while still presenting them in the source code in indented
// form.
//
// It can also receive a list of "replacement" pairs which will be replaced
// in the text, each pair using `strings.ReplaceAll`.
func It(text string, replacements ...string) string {
	var margin string

	text = whitespaceOnly.ReplaceAllString(text, "")
	indents := leadingWhitespace.FindAllStringSubmatch(text, -1)

	// Look for the longest leading string of spaces and tabs common to all
	// lines.
	for i, indent := range indents {
		if i == 0 {
			margin = indent[1]
		} else if strings.HasPrefix(indent[1], margin) {
			// Current line more deeply indented than previous winner:
			// no change (previous winner is still on top).
			continue
		} else if strings.HasPrefix(margin, indent[1]) {
			// Current line consistent with and no deeper than previous winner:
			// it's the new winner.
			margin = indent[1]
		} else {
			// Current line and previous winner have no common whitespace:
			// there is no margin.
			margin = ""
			break
		}
	}

	if margin != "" {
		text = regexp.MustCompile("(?m)^"+margin).ReplaceAllString(text, "")
	}

	for len(replacements) >= 2 {
		text = strings.ReplaceAll(text, replacements[0], replacements[1])
		replacements = replacements[2:]
	}

	return text
}

// Md is a version of `undent.It` that is useful for text which will be used as
// markdown. Md calls It with replacements FakeBacktick, realBacktick, this is
// useful for example when the text is meant to be used as Markdown, since Go
// multiline strings with backticks could not contain other backticks.
func Md(text string) string {
	return strings.TrimSpace(It(text, FakeBacktick, realBacktick))
}
