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

// TestDispatchCoversDetectedLanguages guards the opposite direction: for
// every extension a supported language family owns, the language name the
// grammar registry emits (which is what code.DetectLanguage feeds coupling)
// must dispatch to a resolver. This catches pack-splits like .tsx living in
// its own "tsx" pack rather than under "typescript".
func TestDispatchCoversDetectedLanguages(t *testing.T) {
	registered := make(map[string]bool)
	for _, lr := range newLanguageResolvers(newProjectFS(t.TempDir())) {
		for _, lang := range lr.languages() {
			registered[lang] = true
		}
	}

	reg := grammar.DefaultPackRegistry()
	exts := []string{
		".go",
		".ts", ".tsx", ".js", ".jsx", ".mjs", ".cjs",
		".py",
		".rs",
		".java", ".kt", ".kts", ".scala",
		".cs",
	}
	for _, ext := range exts {
		lang, ok := reg.LangForExtension(ext)
		if !ok {
			t.Errorf("no grammar pack claims extension %q", ext)
			continue
		}
		if !registered[lang] {
			t.Errorf("extension %q detects as language %q, which no resolver dispatches", ext, lang)
		}
	}
}
