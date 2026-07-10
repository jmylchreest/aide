package importresolve

import (
	"testing"

	"github.com/jmylchreest/aide/aide/pkg/grammar"
)

// TestResolverLanguagesMatchGrammarPacks guards against silent drift between
// resolver language declarations and the grammar registry: dispatch is keyed
// by the language names code.DetectLanguage emits, which are pack names. A
// resolver declaring a name with no pack behind it is dead registration —
// no file will ever dispatch to it under that name.
func TestResolverLanguagesMatchGrammarPacks(t *testing.T) {
	reg := grammar.DefaultPackRegistry()
	for _, lr := range newLanguageResolvers(newProjectFS(t.TempDir())) {
		for _, lang := range lr.languages() {
			if reg.Get(lang) == nil {
				t.Errorf("resolver %T declares language %q, but no grammar pack has that name", lr, lang)
			}
		}
	}
}
