package grammar

import "strings"

// IsCommentLine reports whether the given trimmed line starts a comment in the
// named language. Pack data takes precedence; languages without explicit pack
// comment delimiters fall back to the C-family default (//, /*, *).
func IsCommentLine(trimmed, lang string) bool {
	if c := commentsFor(lang); c != nil {
		for _, p := range c.Line {
			if p != "" && strings.HasPrefix(trimmed, p) {
				return true
			}
		}
		for _, b := range c.Block {
			if b[0] != "" && strings.HasPrefix(trimmed, b[0]) {
				return true
			}
		}
		if hasDocStarContinuation(c) && strings.HasPrefix(trimmed, "*") {
			return true
		}
		return false
	}
	return defaultIsCommentLine(trimmed)
}

// ExtractCommentText returns the comment portion of a line, or "" if no comment
// is present. Looks for the language's first comment-start delimiter and returns
// the text after it (stripping a paired block close when present on the same line).
func ExtractCommentText(line, lang string) string {
	if c := commentsFor(lang); c != nil {
		return extractWithPack(line, c)
	}
	return defaultExtractCommentText(line)
}

func commentsFor(lang string) *PackComments {
	if lang == "" {
		return nil
	}
	pack := DefaultPackRegistry().Get(lang)
	if pack == nil || pack.Comments == nil {
		return nil
	}
	return pack.Comments
}

func extractWithPack(line string, c *PackComments) string {
	for _, prefix := range c.Line {
		if prefix == "" {
			continue
		}
		if i := strings.Index(line, prefix); i >= 0 {
			return strings.TrimSpace(line[i+len(prefix):])
		}
	}
	for _, b := range c.Block {
		open, close := b[0], b[1]
		if open == "" {
			continue
		}
		if i := strings.Index(line, open); i >= 0 {
			rest := line[i+len(open):]
			if close != "" {
				if j := strings.Index(rest, close); j >= 0 {
					rest = rest[:j]
				}
			}
			return strings.TrimSpace(rest)
		}
	}
	if hasDocStarContinuation(c) {
		if t := strings.TrimLeft(line, " \t"); strings.HasPrefix(t, "*") {
			return strings.TrimSpace(strings.TrimPrefix(t, "*"))
		}
	}
	return ""
}

// hasDocStarContinuation reports whether the language's block syntax is the
// C-family /* ... */ form. In that style, lines inside the block typically
// start with a leading "*" continuation marker that should be treated as part
// of the comment.
func hasDocStarContinuation(c *PackComments) bool {
	for _, b := range c.Block {
		if b[0] == "/*" {
			return true
		}
	}
	return false
}

func defaultIsCommentLine(trimmed string) bool {
	return strings.HasPrefix(trimmed, "//") ||
		strings.HasPrefix(trimmed, "*") ||
		strings.HasPrefix(trimmed, "/*")
}

func defaultExtractCommentText(line string) string {
	if i := strings.Index(line, "//"); i >= 0 {
		return strings.TrimSpace(line[i+2:])
	}
	if i := strings.Index(line, "/*"); i >= 0 {
		rest := line[i+2:]
		if j := strings.Index(rest, "*/"); j >= 0 {
			rest = rest[:j]
		}
		return strings.TrimSpace(rest)
	}
	if t := strings.TrimLeft(line, " \t"); strings.HasPrefix(t, "*") {
		return strings.TrimSpace(strings.TrimPrefix(t, "*"))
	}
	return ""
}
