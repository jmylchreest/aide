package survey

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRunTopology_GoModule(t *testing.T) {
	dir := t.TempDir()

	// Create a go.mod file
	goMod := filepath.Join(dir, "go.mod")
	if err := os.WriteFile(goMod, []byte("module example.com/myproject\n\ngo 1.21\n"), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := RunTopology(dir)
	if err != nil {
		t.Fatalf("RunTopology: %v", err)
	}

	// Should find at least a module entry and a tech_stack entry for Go
	var foundModule, foundTechStack bool
	for _, e := range result.Entries {
		if e.Kind == KindModule && e.Name == "example.com/myproject" {
			foundModule = true
			if e.Analyzer != AnalyzerTopology {
				t.Errorf("module entry analyzer = %q, want %q", e.Analyzer, AnalyzerTopology)
			}
			if e.Metadata["language"] != "go" {
				t.Errorf("module metadata[language] = %q, want %q", e.Metadata["language"], "go")
			}
		}
		if e.Kind == KindTechStack && e.Name == "go" {
			foundTechStack = true
			if e.Metadata["marker"] != "go.mod" {
				t.Errorf("tech_stack metadata[marker] = %q, want %q", e.Metadata["marker"], "go.mod")
			}
		}
	}
	if !foundModule {
		t.Error("expected to find Go module entry")
	}
	if !foundTechStack {
		t.Error("expected to find Go tech_stack entry")
	}
}

func TestRunTopology_GoWorkspace(t *testing.T) {
	dir := t.TempDir()

	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/ws\n\ngo 1.21\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "go.work"), []byte("go 1.21\n\nuse ./sub\n"), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := RunTopology(dir)
	if err != nil {
		t.Fatalf("RunTopology: %v", err)
	}

	var foundWorkspace bool
	for _, e := range result.Entries {
		if e.Kind == KindWorkspace && e.Metadata["language"] == "go" {
			foundWorkspace = true
		}
	}
	if !foundWorkspace {
		t.Error("expected to find Go workspace entry")
	}
}

func TestRunTopology_NodeProject(t *testing.T) {
	dir := t.TempDir()

	pkg := filepath.Join(dir, "package.json")
	if err := os.WriteFile(pkg, []byte("{\n  \"name\": \"my-app\",\n  \"version\": \"1.0.0\"\n}\n"), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := RunTopology(dir)
	if err != nil {
		t.Fatalf("RunTopology: %v", err)
	}

	var foundModule, foundTechStack bool
	for _, e := range result.Entries {
		if e.Kind == KindModule && e.Name == "my-app" {
			foundModule = true
		}
		if e.Kind == KindTechStack && e.Name == "javascript" && e.Metadata["marker"] == "package.json" {
			foundTechStack = true
		}
	}
	if !foundModule {
		t.Error("expected to find Node.js module entry")
	}
	if !foundTechStack {
		t.Error("expected to find javascript tech_stack entry from package.json")
	}
}

func TestRunTopology_TypeScript(t *testing.T) {
	dir := t.TempDir()

	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"name": "ts-app"}`+"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "tsconfig.json"), []byte(`{"compilerOptions": {}}`+"\n"), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := RunTopology(dir)
	if err != nil {
		t.Fatalf("RunTopology: %v", err)
	}

	var foundTS bool
	for _, e := range result.Entries {
		if e.Kind == KindTechStack && e.Name == "typescript" {
			foundTS = true
		}
	}
	if !foundTS {
		t.Error("expected to find TypeScript tech_stack entry")
	}
}

func TestRunTopology_PythonProject(t *testing.T) {
	dir := t.TempDir()

	if err := os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte("[project]\nname = \"mylib\"\n"), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := RunTopology(dir)
	if err != nil {
		t.Fatalf("RunTopology: %v", err)
	}

	var found bool
	for _, e := range result.Entries {
		if e.Kind == KindTechStack && e.Name == "python" {
			found = true
			if e.Metadata["marker"] != "pyproject.toml" {
				t.Errorf("python metadata[marker] = %q, want %q", e.Metadata["marker"], "pyproject.toml")
			}
		}
	}
	if !found {
		t.Error("expected to find Python tech_stack entry")
	}
}

func TestRunTopology_RustProject(t *testing.T) {
	dir := t.TempDir()

	if err := os.WriteFile(filepath.Join(dir, "Cargo.toml"), []byte("[package]\nname = \"mycrate\"\n"), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := RunTopology(dir)
	if err != nil {
		t.Fatalf("RunTopology: %v", err)
	}

	var foundTech, foundModule bool
	for _, e := range result.Entries {
		if e.Kind == KindTechStack && e.Name == "rust" {
			foundTech = true
		}
		if e.Kind == KindModule && e.Metadata["language"] == "rust" {
			foundModule = true
		}
	}
	if !foundTech {
		t.Error("expected to find Rust tech_stack entry")
	}
	if !foundModule {
		t.Error("expected to find Rust module entry")
	}
}

func TestRunTopology_BuildSystems(t *testing.T) {
	dir := t.TempDir()

	if err := os.WriteFile(filepath.Join(dir, "Makefile"), []byte("all:\n\techo hello\n"), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := RunTopology(dir)
	if err != nil {
		t.Fatalf("RunTopology: %v", err)
	}

	var found bool
	for _, e := range result.Entries {
		if e.Kind == KindTechStack && e.Name == "make" {
			found = true
			if e.Metadata["build_system"] != "make" {
				t.Errorf("metadata[build_system] = %q, want %q", e.Metadata["build_system"], "make")
			}
		}
	}
	if !found {
		t.Error("expected to find make build system entry")
	}
}

func TestRunTopology_Docker(t *testing.T) {
	dir := t.TempDir()

	if err := os.WriteFile(filepath.Join(dir, "Dockerfile"), []byte("FROM golang:1.21\n"), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := RunTopology(dir)
	if err != nil {
		t.Fatalf("RunTopology: %v", err)
	}

	var found bool
	for _, e := range result.Entries {
		if e.Kind == KindTechStack && e.Name == "docker" {
			found = true
		}
	}
	if !found {
		t.Error("expected to find Docker tech_stack entry")
	}
}

func TestRunTopology_EmptyDir(t *testing.T) {
	dir := t.TempDir()

	result, err := RunTopology(dir)
	if err != nil {
		t.Fatalf("RunTopology: %v", err)
	}

	if len(result.Entries) != 0 {
		t.Errorf("expected 0 entries for empty dir, got %d", len(result.Entries))
	}
}

func TestRunTopology_SkipsHiddenDirs(t *testing.T) {
	dir := t.TempDir()

	// Create a hidden directory with a go.mod inside
	hidden := filepath.Join(dir, ".hidden")
	if err := os.MkdirAll(hidden, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(hidden, "go.mod"), []byte("module hidden\n"), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := RunTopology(dir)
	if err != nil {
		t.Fatalf("RunTopology: %v", err)
	}

	for _, e := range result.Entries {
		if e.Kind == KindModule && e.Name == "hidden" {
			t.Error("should NOT find module in hidden directory")
		}
	}
}

func TestRunTopology_SkipsNodeModules(t *testing.T) {
	dir := t.TempDir()

	nm := filepath.Join(dir, "node_modules", "some-pkg")
	if err := os.MkdirAll(nm, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nm, "package.json"), []byte(`{"name": "some-pkg"}`+"\n"), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := RunTopology(dir)
	if err != nil {
		t.Fatalf("RunTopology: %v", err)
	}

	for _, e := range result.Entries {
		if e.Kind == KindModule && e.Name == "some-pkg" {
			t.Error("should NOT find package.json inside node_modules")
		}
	}
}

func TestRunTopology_MonorepoTools(t *testing.T) {
	dir := t.TempDir()

	if err := os.WriteFile(filepath.Join(dir, "turbo.json"), []byte(`{"pipeline": {}}`+"\n"), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := RunTopology(dir)
	if err != nil {
		t.Fatalf("RunTopology: %v", err)
	}

	var found bool
	for _, e := range result.Entries {
		if e.Kind == KindWorkspace && e.Name == "turborepo" {
			found = true
		}
	}
	if !found {
		t.Error("expected to find turborepo workspace entry")
	}
}

func TestParseGoModuleName(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "go.mod")

	if err := os.WriteFile(path, []byte("module github.com/test/project\n\ngo 1.21\n"), 0644); err != nil {
		t.Fatal(err)
	}

	name := parseGoModuleName(path)
	if name != "github.com/test/project" {
		t.Errorf("parseGoModuleName = %q, want %q", name, "github.com/test/project")
	}
}

func TestParseGoModuleName_Empty(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "go.mod")

	if err := os.WriteFile(path, []byte("go 1.21\n"), 0644); err != nil {
		t.Fatal(err)
	}

	name := parseGoModuleName(path)
	if name != "" {
		t.Errorf("parseGoModuleName = %q, want empty", name)
	}
}

func TestParseJSONField(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "package.json")

	if err := os.WriteFile(path, []byte(`{
  "name": "my-project",
  "version": "1.0.0"
}`+"\n"), 0644); err != nil {
		t.Fatal(err)
	}

	name := parseJSONField(path, "name")
	if name != "my-project" {
		t.Errorf("parseJSONField(name) = %q, want %q", name, "my-project")
	}

	version := parseJSONField(path, "version")
	if version != "1.0.0" {
		t.Errorf("parseJSONField(version) = %q, want %q", version, "1.0.0")
	}
}

func TestParseJSONField_Missing(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "package.json")

	if err := os.WriteFile(path, []byte(`{"name": "test"}`+"\n"), 0644); err != nil {
		t.Fatal(err)
	}

	val := parseJSONField(path, "description")
	if val != "" {
		t.Errorf("parseJSONField(description) = %q, want empty", val)
	}
}

func TestRunTopology_CICD(t *testing.T) {
	dir := t.TempDir()

	// Create .github/workflows with a file
	wfDir := filepath.Join(dir, ".github", "workflows")
	if err := os.MkdirAll(wfDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wfDir, "ci.yml"), []byte("name: CI\n"), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := RunTopology(dir)
	if err != nil {
		t.Fatalf("RunTopology: %v", err)
	}

	var found bool
	for _, e := range result.Entries {
		if e.Kind == KindTechStack && e.Name == "github-actions" {
			found = true
		}
	}
	if !found {
		t.Error("expected to find GitHub Actions CI/CD entry")
	}
}
