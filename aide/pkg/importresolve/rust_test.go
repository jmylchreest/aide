package importresolve

import (
	"os"
	"path/filepath"
	"testing"
)

// rustFixture builds a two-crate cargo workspace.
func rustFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()

	files := map[string]string{
		"Cargo.toml": "[workspace]\nmembers = [\"app\", \"app-core\"]\n",

		"app/Cargo.toml":         "[package]\nname = \"app\"\nversion = \"0.1.0\"\n",
		"app/src/main.rs":        "fn main() {}\n",
		"app/src/config.rs":      "pub struct Config;\n",
		"app/src/store/mod.rs":   "pub mod entry;\n",
		"app/src/store/entry.rs": "pub struct Entry;\n",

		"app-core/Cargo.toml":  "[package]\nname = \"app-core\"\nversion = \"0.1.0\"\n",
		"app-core/src/lib.rs":  "pub mod util;\n",
		"app-core/src/util.rs": "pub fn helper() {}\n",
	}
	for rel, content := range files {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatalf("MkdirAll: %v", err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
	}
	return root
}

func TestResolveRust(t *testing.T) {
	r := New(rustFixture(t))

	cases := []struct {
		from, imp, want string
	}{
		{"app/src/main.rs", "crate::config", "app/src/config.rs"},                   // crate-rooted module
		{"app/src/main.rs", "crate::config::Config", "app/src/config.rs"},           // trailing item segment dropped
		{"app/src/main.rs", "crate::store::entry::Entry", "app/src/store/entry.rs"}, // nested module
		{"app/src/main.rs", "crate::store", "app/src/store/mod.rs"},                 // mod.rs layout
		{"app/src/main.rs", "config::Config", "app/src/config.rs"},                  // regex stripped "crate::" — probe recovers it
		{"app/src/store/entry.rs", "super::Store", "app/src/store/mod.rs"},          // super: item in parent module
		{"app/src/store/mod.rs", "self::entry", "app/src/store/entry.rs"},           // self: child module from mod.rs
		{"app/src/store/mod.rs", "super::config::Config", "app/src/config.rs"},      // super from mod.rs pops past the dir
		{"app/src/main.rs", "app_core::util::helper", "app-core/src/util.rs"},       // cross-crate, hyphen/underscore mapping
		{"app/src/main.rs", "app_core::VERSION", "app-core/src/lib.rs"},             // item at crate root
		{"app/src/main.rs", "serde::Deserialize", ""},                               // external dependency
		{"app/src/main.rs", "std::collections::HashMap", ""},                        // stdlib
		{"app/src/main.rs", "app_core::", "app-core/src/lib.rs"},                    // glob capture artifact (use app_core::*)
	}
	for _, c := range cases {
		if got := r.ResolveUnit("rust", c.from, c.imp); got != c.want {
			t.Errorf("ResolveUnit(rust, %q from %q) = %q, want %q", c.imp, c.from, got, c.want)
		}
	}

	if got := r.UnitOf("rust", "app/src/config.rs"); got != "app/src/config.rs" {
		t.Errorf("UnitOf(rust) = %q, want the file itself", got)
	}
}
