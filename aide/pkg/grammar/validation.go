package grammar

import (
	"fmt"
	"regexp"

	"github.com/bmatcuk/doublestar/v4"
)

// ValidatePatterns rejects malformed overrides before they can silently disable
// exclusions or turn a filtered entrypoint search into an unfiltered one.
func (p *Pack) ValidatePatterns() error {
	var regexes []string
	var globs []string
	if ep := p.Entrypoints; ep != nil {
		for _, s := range ep.Symbols {
			regexes = append(regexes, s.Exclude, s.NameMatch, s.FileMatch)
		}
		for _, r := range ep.Refs {
			regexes = append(regexes, r.Exclude, r.NameMatch, r.Qualifier)
		}
		for _, f := range ep.FilePatterns {
			regexes = append(regexes, f.Content, f.PreContent)
			globs = append(globs, f.FileMatch)
		}
	}
	if p.Files != nil {
		globs = append(globs, p.Files.GeneratedFilePatterns...)
	}
	if p.Deadcode != nil {
		globs = append(globs, p.Deadcode.TestFilePatterns...)
	}
	for _, pattern := range regexes {
		if _, err := regexp.Compile(pattern); err != nil {
			return fmt.Errorf("pack %s regex %q: %w", p.Name, pattern, err)
		}
	}
	for _, pattern := range globs {
		if !doublestar.ValidatePattern(pattern) {
			return fmt.Errorf("pack %s has invalid glob %q", p.Name, pattern)
		}
	}
	return nil
}
