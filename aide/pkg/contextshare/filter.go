package contextshare

import (
	"github.com/jmylchreest/aide/aide/pkg/memory"
)

// Filter is a generic include/exclude glob filter over a record's flat token
// set. It is config-agnostic on purpose — the cmd layer maps user config into
// this struct so contextshare never imports pkg/config.
//
// A record passes when it matches ANY include glob (an empty Include, or one
// containing "*", means "match all") AND matches NO exclude glob. Exclude
// always wins over include.
type Filter struct {
	Include []string
	Exclude []string
}

// Match reports whether a record carrying the given tokens passes the filter.
func (f Filter) Match(tokens []string) bool {
	// Exclude wins: any exclude glob that matches a token rejects the record.
	for _, pat := range f.Exclude {
		for _, tok := range tokens {
			if globMatch(pat, tok) {
				return false
			}
		}
	}

	// Empty include (or a "*" include) means match-all.
	if includeMatchesAll(f.Include) {
		return true
	}
	for _, pat := range f.Include {
		for _, tok := range tokens {
			if globMatch(pat, tok) {
				return true
			}
		}
	}
	return false
}

// includeMatchesAll reports whether an include list means "match everything":
// an empty list, or any "*" entry.
func includeMatchesAll(include []string) bool {
	if len(include) == 0 {
		return true
	}
	for _, pat := range include {
		if pat == "*" {
			return true
		}
	}
	return false
}

// MemoryTokens returns the flat token set a filter globs over for a memory:
// every raw tag verbatim PLUS the synthesized "category:<category>" token.
func MemoryTokens(m *memory.Memory) []string {
	tokens := make([]string, 0, len(m.Tags)+1)
	tokens = append(tokens, m.Tags...)
	tokens = append(tokens, "category:"+string(m.Category))
	return tokens
}

// DecisionTokens returns the flat token set a filter globs over for a decision:
// the synthesized "topic:<topic>" and "decided_by:<who>" tokens. Decisions have
// no tags today; if they gain them, append them here.
func DecisionTokens(d *memory.Decision) []string {
	return []string{
		"topic:" + d.Topic,
		"decided_by:" + d.DecidedBy,
	}
}

// globMatch reports whether s matches pattern, where the only wildcard is `*`,
// matching any run of characters (including ":" and "/"). Every other byte is
// matched literally, so this deliberately avoids path.Match — which treats "/"
// specially and would mis-handle tokens like "decided_by:blueprint:go". The
// match is case-sensitive and anchored at both ends.
//
// Implemented as a two-pointer matcher with backtracking on the last `*`, which
// is linear in practice for the short, low-wildcard patterns aide uses.
func globMatch(pattern, s string) bool {
	var (
		px, sx    int  // current positions in pattern and s
		star      = -1 // index in pattern just past the last '*' seen
		starMatch = 0  // position in s at which that '*' began matching
		patLen    = len(pattern)
		strLen    = len(s)
	)
	for sx < strLen {
		switch {
		case px < patLen && pattern[px] == '*':
			star = px + 1
			starMatch = sx
			px++
		case px < patLen && pattern[px] == s[sx]:
			px++
			sx++
		case star != -1:
			// Backtrack: let the last '*' absorb one more character of s.
			px = star
			starMatch++
			sx = starMatch
		default:
			return false
		}
	}
	// Consume any trailing '*' in the pattern.
	for px < patLen && pattern[px] == '*' {
		px++
	}
	return px == patLen
}
