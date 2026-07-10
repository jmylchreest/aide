package importresolve

import (
	"os"
	"path/filepath"
	"testing"
)

func writeFixture(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
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

func TestResolveTSConfigPaths(t *testing.T) {
	r := New(writeFixture(t, map[string]string{
		"web/tsconfig.json": `{
  // JSONC: comments and trailing commas must parse
  "compilerOptions": {
    "baseUrl": ".",
    "paths": {
      "@/*": ["./src/*"],
      "@components/*": ["./src/components/*"],
      "utils": ["./src/lib/utils.ts"],
    },
  },
}`,
		"web/src/context/Clipboard.tsx": "export {}\n",
		"web/src/components/Button.tsx": "export {}\n",
		"web/src/lib/utils.ts":          "export {}\n",
		"web/src/pages/Home.tsx":        "export {}\n",
		"other/src/no-alias.ts":         "export {}\n",
	}))

	cases := []struct {
		from, imp, want string
	}{
		{"web/src/pages/Home.tsx", "@/context/Clipboard", "web/src/context/Clipboard.tsx"}, // wildcard alias
		{"web/src/pages/Home.tsx", "@components/Button", "web/src/components/Button.tsx"},  // more specific alias wins
		{"web/src/pages/Home.tsx", "utils", "web/src/lib/utils.ts"},                        // exact alias
		{"web/src/pages/Home.tsx", "@/missing/File", ""},                                   // alias matches, file absent
		{"other/src/no-alias.ts", "@/context/Clipboard", ""},                               // config does not govern this file
	}
	for _, c := range cases {
		if got := r.ResolveUnit("typescript", c.from, c.imp); got != c.want {
			t.Errorf("ResolveUnit(ts, %q from %q) = %q, want %q", c.imp, c.from, got, c.want)
		}
	}

	// The tsx pack owns .tsx: dispatch under language "tsx" must work too.
	if got := r.ResolveUnit("tsx", "web/src/pages/Home.tsx", "@/context/Clipboard"); got != "web/src/context/Clipboard.tsx" {
		t.Errorf("ResolveUnit(tsx alias) = %q", got)
	}
}

func TestResolveJVM(t *testing.T) {
	r := New(writeFixture(t, map[string]string{
		"pom.xml": "<project/>",
		"src/main/java/com/example/app/Main.java":        "package com.example.app;\n",
		"src/main/java/com/example/app/util/Helper.java": "package com.example.app.util;\n",
		"src/main/kotlin/com/example/app/Ext.kt":         "package com.example.app\n",
	}))

	cases := []struct {
		lang, imp, want string
	}{
		{"java", "com.example.app.util.Helper", "src/main/java/com/example/app/util/Helper.java"},       // class import
		{"java", "com.example.app.util.Helper.CONST", "src/main/java/com/example/app/util/Helper.java"}, // static member drop
		{"java", "com.example.app.util.", "src/main/java/com/example/app/util"},                         // wildcard -> package dir
		{"kotlin", "com.example.app.util.Helper", "src/main/java/com/example/app/util/Helper.java"},     // kotlin importing java
		{"java", "com.example.app.Ext", "src/main/kotlin/com/example/app/Ext.kt"},                       // java seeing kotlin
		{"java", "java.util.List", ""},   // JDK
		{"java", "org.slf4j.Logger", ""}, // third-party
	}
	for _, c := range cases {
		if got := r.ResolveUnit(c.lang, "src/main/java/com/example/app/Main.java", c.imp); got != c.want {
			t.Errorf("ResolveUnit(%s, %q) = %q, want %q", c.lang, c.imp, got, c.want)
		}
	}
}

func TestResolveCSharp(t *testing.T) {
	r := New(writeFixture(t, map[string]string{
		"App/Program.cs":              "using MyApp.Services;\n\nnamespace MyApp;\n",
		"App/Services/UserService.cs": "namespace MyApp.Services;\n\npublic class UserService {}\n",
		"App/Services/Sub/Deep.cs":    "namespace MyApp.Services.Deep\n{\n}\n", // block-scoped, dir != namespace
		"Lib/Weird/Anywhere.cs":       "namespace MyApp.Elsewhere;\n",          // namespace unrelated to folder
	}))

	cases := []struct {
		imp, want string
	}{
		{"MyApp.Services", "MyApp.Services"},             // declared namespace
		{"MyApp.Elsewhere", "MyApp.Elsewhere"},           // no folder correspondence — pre-scan finds it
		{"MyApp.Services.Deep", "MyApp.Services.Deep"},   // block-scoped declaration
		{"MyApp.Services.UserService", "MyApp.Services"}, // using static class -> trailing segment drop
		{"MyApp", "MyApp"},                               // ancestor namespace is real
		{"System.Linq", ""},                              // framework
		{"Newtonsoft.Json", ""},                          // third-party
	}
	for _, c := range cases {
		if got := r.ResolveUnit("csharp", "App/Program.cs", c.imp); got != c.want {
			t.Errorf("ResolveUnit(csharp, %q) = %q, want %q", c.imp, got, c.want)
		}
	}

	// Units are namespaces: a file's own unit is its declared namespace.
	if got := r.UnitOf("csharp", "App/Services/UserService.cs"); got != "MyApp.Services" {
		t.Errorf("UnitOf(csharp) = %q, want %q", got, "MyApp.Services")
	}
	if got := r.UnitOf("csharp", "App/Unscanned.txt"); got != "App/Unscanned.txt" {
		t.Errorf("UnitOf(csharp, unscanned) = %q, want the file", got)
	}
}
