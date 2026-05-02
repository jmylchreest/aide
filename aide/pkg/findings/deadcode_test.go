package findings

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jmylchreest/aide/aide/pkg/code"
	"github.com/jmylchreest/aide/aide/pkg/grammar"
)

func TestIsExportedByRule(t *testing.T) {
	cases := []struct {
		name string
		rule string
		want bool
	}{
		{"Foo", "first_char_uppercase", true},
		{"foo", "first_char_uppercase", false},
		{"Foo", "no_leading_underscore", true},
		{"_foo", "no_leading_underscore", false},
		{"foo", "no_leading_underscore", true},
		{"Foo", "", false},
		{"", "first_char_uppercase", false},
	}
	for _, c := range cases {
		got := isExportedByRule(c.name, c.rule)
		if got != c.want {
			t.Errorf("isExportedByRule(%q, %q) = %v, want %v", c.name, c.rule, got, c.want)
		}
	}
}

// fakePackProvider returns a pack with the given deadcode rules for the named language.
func fakePackProvider(lang string, rule string, suppression []string) func(string) *grammar.Pack {
	return func(l string) *grammar.Pack {
		if l != lang {
			return nil
		}
		return &grammar.Pack{
			Name: lang,
			Deadcode: &grammar.PackDeadcode{
				ExportedRule:        rule,
				SuppressionPatterns: suppression,
			},
		}
	}
}

func TestAnalyzeDeadCode_SkipsExported(t *testing.T) {
	tmp := t.TempDir()
	src := "package demo\n\nfunc PublicFunc() {}\nfunc privateFunc() {}\n"
	if err := os.WriteFile(filepath.Join(tmp, "demo.go"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	syms := []*code.Symbol{
		{Name: "PublicFunc", Kind: code.KindFunction, FilePath: "demo.go", StartLine: 3, Language: "go"},
		{Name: "privateFunc", Kind: code.KindFunction, FilePath: "demo.go", StartLine: 4, Language: "go"},
	}

	cfg := DeadCodeConfig{
		GetAllSymbols: func() ([]*code.Symbol, error) { return syms, nil },
		GetRefCount:   func(string) (int, error) { return 0, nil },
		ProjectRoot:   tmp,
		PackProvider:  fakePackProvider("go", "first_char_uppercase", nil),
	}
	ff, res, err := AnalyzeDeadCode(cfg)
	if err != nil {
		t.Fatalf("AnalyzeDeadCode: %v", err)
	}
	if len(ff) != 1 || ff[0].Metadata["symbol"] != "privateFunc" {
		t.Fatalf("expected only privateFunc to be flagged, got %d findings: %+v", len(ff), ff)
	}
	if res.SymbolsSkipped == 0 {
		t.Errorf("expected at least one skip for the exported symbol")
	}

	cfg.IncludeExported = true
	ff2, _, err := AnalyzeDeadCode(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(ff2) != 2 {
		t.Errorf("with IncludeExported=true expected 2 findings, got %d", len(ff2))
	}
}

func TestAnalyzeDeadCode_SuppressionPattern(t *testing.T) {
	tmp := t.TempDir()
	src := `package demo

//nolint:unused
func suppressed() {}

func notSuppressed() {}
`
	if err := os.WriteFile(filepath.Join(tmp, "demo.go"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	syms := []*code.Symbol{
		{Name: "suppressed", Kind: code.KindFunction, FilePath: "demo.go", StartLine: 4, Language: "go"},
		{Name: "notSuppressed", Kind: code.KindFunction, FilePath: "demo.go", StartLine: 6, Language: "go"},
	}

	cfg := DeadCodeConfig{
		GetAllSymbols: func() ([]*code.Symbol, error) { return syms, nil },
		GetRefCount:   func(string) (int, error) { return 0, nil },
		ProjectRoot:   tmp,
		PackProvider: fakePackProvider("go", "", []string{
			`//\s*nolint:(?:unused|deadcode|all)`,
		}),
	}
	ff, _, err := AnalyzeDeadCode(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(ff) != 1 || ff[0].Metadata["symbol"] != "notSuppressed" {
		t.Fatalf("expected only notSuppressed to be flagged, got %d: %+v", len(ff), ff)
	}
}

func TestAnalyzeDeadCode_RustTestAttributes(t *testing.T) {
	tmp := t.TempDir()
	src := `// rosec-style test module
pub fn real_helper() {}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn basic_test() {
        assert_eq!(1, 1);
    }

    #[tokio::test]
    async fn async_test() {}

    #[test]
    #[should_panic]
    fn panics_test() { panic!() }

    #[test]
    #[ignore = "slow"]
    fn slow_test() {}

    fn untagged_helper_inside_tests() {}
}

#[zbus::interface(name = "org.example.Foo")]
impl FooService {
    fn dispatched_via_dbus(&self) -> u32 { 0 }
    async fn another_dbus_method(&self) {}
}

#[plugin_fn]
pub fn extism_entry_point() -> i32 { 0 }
`
	if err := os.WriteFile(filepath.Join(tmp, "lib.rs"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	syms := []*code.Symbol{
		{Name: "real_helper", Kind: code.KindFunction, FilePath: "lib.rs", StartLine: 2, Language: "rust"},
		{Name: "tests", Kind: code.KindType, FilePath: "lib.rs", StartLine: 5, EndLine: 26, Language: "rust"},
		{Name: "basic_test", Kind: code.KindFunction, FilePath: "lib.rs", StartLine: 9, Language: "rust"},
		{Name: "async_test", Kind: code.KindFunction, FilePath: "lib.rs", StartLine: 14, Language: "rust"},
		{Name: "panics_test", Kind: code.KindFunction, FilePath: "lib.rs", StartLine: 18, Language: "rust"},
		{Name: "slow_test", Kind: code.KindFunction, FilePath: "lib.rs", StartLine: 22, Language: "rust"},
		{Name: "untagged_helper_inside_tests", Kind: code.KindFunction, FilePath: "lib.rs", StartLine: 24, Language: "rust"},
		{Name: "dispatched_via_dbus", Kind: code.KindFunction, FilePath: "lib.rs", StartLine: 29, Language: "rust"},
		{Name: "another_dbus_method", Kind: code.KindFunction, FilePath: "lib.rs", StartLine: 30, Language: "rust"},
		{Name: "extism_entry_point", Kind: code.KindFunction, FilePath: "lib.rs", StartLine: 34, Language: "rust"},
	}

	registry := grammar.DefaultPackRegistry()
	cfg := DeadCodeConfig{
		GetAllSymbols: func() ([]*code.Symbol, error) { return syms, nil },
		GetRefCount:   func(string) (int, error) { return 0, nil },
		ProjectRoot:   tmp,
		PackProvider:  registry.Get,
	}
	ff, _, err := AnalyzeDeadCode(cfg)
	if err != nil {
		t.Fatal(err)
	}

	flagged := make(map[string]bool, len(ff))
	for _, f := range ff {
		flagged[f.Metadata["symbol"]] = true
	}

	mustSuppress := []string{
		"basic_test", "async_test", "panics_test", "slow_test",
		"untagged_helper_inside_tests",
		"dispatched_via_dbus", "another_dbus_method",
		"extism_entry_point",
		"tests",
	}
	for _, name := range mustSuppress {
		if flagged[name] {
			t.Errorf("symbol %q should be suppressed but was flagged", name)
		}
	}
}

func TestAnalyzeDeadCode_TextGrepVerification(t *testing.T) {
	tmp := t.TempDir()
	// Two files: foo.go declares handleX; bar.go calls s.handleX (qualified
	// receiver call the index would miss, but text-grep should catch).
	if err := os.WriteFile(filepath.Join(tmp, "foo.go"),
		[]byte("package demo\n\nfunc (s *S) handleX() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmp, "bar.go"),
		[]byte("package demo\n\nfunc use(s *S) { s.handleX() }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmp, "baz.go"),
		[]byte("package demo\n\nfunc trulyDead() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	syms := []*code.Symbol{
		{Name: "handleX", Kind: code.KindMethod, FilePath: "foo.go", StartLine: 3, EndLine: 3, Language: "go"},
		{Name: "trulyDead", Kind: code.KindFunction, FilePath: "baz.go", StartLine: 3, EndLine: 3, Language: "go"},
		{Name: "use", Kind: code.KindFunction, FilePath: "bar.go", StartLine: 3, EndLine: 3, Language: "go"},
	}

	cfg := DeadCodeConfig{
		GetAllSymbols: func() ([]*code.Symbol, error) { return syms, nil },
		GetRefCount:   func(string) (int, error) { return 0, nil }, // simulate index missing all refs
		ProjectRoot:   tmp,
	}
	ff, _, err := AnalyzeDeadCode(cfg)
	if err != nil {
		t.Fatal(err)
	}

	got := map[string]bool{}
	for _, f := range ff {
		got[f.Metadata["symbol"]] = true
	}
	if got["handleX"] {
		t.Errorf("handleX should be rescued by text-grep (s.handleX in bar.go)")
	}
	if !got["trulyDead"] {
		t.Errorf("trulyDead has no callers and should remain a finding; got %+v", got)
	}
}

func TestAnalyzeDeadCode_ConsumerFormatsRescue(t *testing.T) {
	tmp := t.TempDir()
	// App.tsx defines a React component; index.astro consumes it via JSX.
	// Without consumer-format scanning, App is flagged dead because .astro is
	// not in the code index.
	if err := os.WriteFile(filepath.Join(tmp, "App.tsx"),
		[]byte("export function App() { return null }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmp, "index.astro"),
		[]byte("---\nimport App from \"./App\"\n---\n<App client:only=\"react\" />\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	syms := []*code.Symbol{
		{Name: "App", Kind: code.KindFunction, FilePath: "App.tsx", StartLine: 1, EndLine: 1, Language: "typescript"},
	}

	cfg := DeadCodeConfig{
		GetAllSymbols:      func() ([]*code.Symbol, error) { return syms, nil },
		GetRefCount:        func(string) (int, error) { return 0, nil },
		ProjectRoot:        tmp,
		ConsumerExtensions: []string{".astro"},
	}
	ff, _, err := AnalyzeDeadCode(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(ff) != 0 {
		t.Errorf("expected App to be rescued via consumer-format scan, got %d findings: %+v", len(ff), ff)
	}

	// Without ConsumerExtensions, App should still be flagged.
	cfg.ConsumerExtensions = nil
	ff2, _, _ := AnalyzeDeadCode(cfg)
	if len(ff2) != 1 {
		t.Errorf("control: expected App to be flagged when consumer scan disabled, got %d", len(ff2))
	}
}
