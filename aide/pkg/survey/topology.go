package survey

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// TopologyResult holds the output of the topology analyzer.
type TopologyResult struct {
	Entries []*Entry
}

// RunTopology detects repo structure by filesystem inspection.
// It walks the directory tree looking for build system files, language markers,
// workspace definitions, and emits survey entries for each discovery.
func RunTopology(rootDir string) (*TopologyResult, error) {
	result := &TopologyResult{}

	// Detect Go modules
	result.detectGoModules(rootDir)

	// Detect Node.js projects
	result.detectNodeProjects(rootDir)

	// Detect Python projects
	result.detectPythonProjects(rootDir)

	// Detect Rust projects
	result.detectRustProjects(rootDir)

	// Detect build systems
	result.detectBuildSystems(rootDir)

	// Detect Docker
	result.detectDocker(rootDir)

	// Detect CI/CD
	result.detectCICD(rootDir)

	// Detect monorepo tools
	result.detectMonorepoTools(rootDir)

	return result, nil
}

// detectGoModules finds go.mod files and emits module/workspace entries.
func (r *TopologyResult) detectGoModules(rootDir string) {
	walkForFile(rootDir, "go.mod", 3, func(path string) {
		relPath, _ := filepath.Rel(rootDir, filepath.Dir(path))
		if relPath == "." {
			relPath = ""
		}

		// Parse module name from go.mod
		modName := parseGoModuleName(path)
		if modName == "" {
			modName = relPath
		}

		// Check for go.work (workspace)
		workPath := filepath.Join(filepath.Dir(path), "go.work")
		if _, err := os.Stat(workPath); err == nil {
			r.Entries = append(r.Entries, &Entry{
				Analyzer: AnalyzerTopology,
				Kind:     KindWorkspace,
				Name:     modName,
				FilePath: relPath,
				Title:    "Go workspace",
				Detail:   fmt.Sprintf("Go workspace at %s with go.work", relPath),
				Metadata: map[string]string{"language": "go", "build_system": "go.work"},
			})
		}

		r.Entries = append(r.Entries, &Entry{
			Analyzer: AnalyzerTopology,
			Kind:     KindModule,
			Name:     modName,
			FilePath: relPath,
			Title:    fmt.Sprintf("Go module: %s", modName),
			Metadata: map[string]string{"language": "go"},
		})

		r.Entries = append(r.Entries, &Entry{
			Analyzer: AnalyzerTopology,
			Kind:     KindTechStack,
			Name:     "go",
			FilePath: relPath,
			Title:    "Go language detected",
			Metadata: map[string]string{"language": "go", "marker": "go.mod"},
		})
	})
}

// detectNodeProjects finds package.json files and emits entries.
func (r *TopologyResult) detectNodeProjects(rootDir string) {
	walkForFile(rootDir, "package.json", 3, func(path string) {
		relPath, _ := filepath.Rel(rootDir, filepath.Dir(path))
		if relPath == "." {
			relPath = ""
		}

		// Skip node_modules
		if strings.Contains(path, "node_modules") {
			return
		}

		name := parseJSONField(path, "name")
		if name == "" {
			name = filepath.Base(filepath.Dir(path))
		}

		r.Entries = append(r.Entries, &Entry{
			Analyzer: AnalyzerTopology,
			Kind:     KindModule,
			Name:     name,
			FilePath: relPath,
			Title:    fmt.Sprintf("Node.js package: %s", name),
			Metadata: map[string]string{"language": "javascript", "marker": "package.json"},
		})

		// Detect TypeScript
		tsConfig := filepath.Join(filepath.Dir(path), "tsconfig.json")
		if _, err := os.Stat(tsConfig); err == nil {
			r.Entries = append(r.Entries, &Entry{
				Analyzer: AnalyzerTopology,
				Kind:     KindTechStack,
				Name:     "typescript",
				FilePath: relPath,
				Title:    "TypeScript detected",
				Metadata: map[string]string{"language": "typescript", "marker": "tsconfig.json"},
			})
		}

		r.Entries = append(r.Entries, &Entry{
			Analyzer: AnalyzerTopology,
			Kind:     KindTechStack,
			Name:     "nodejs",
			FilePath: relPath,
			Title:    "Node.js detected",
			Metadata: map[string]string{"language": "javascript", "marker": "package.json"},
		})
	})
}

// detectPythonProjects finds Python project markers.
func (r *TopologyResult) detectPythonProjects(rootDir string) {
	markers := []string{"pyproject.toml", "setup.py", "setup.cfg"}
	for _, marker := range markers {
		walkForFile(rootDir, marker, 2, func(path string) {
			relPath, _ := filepath.Rel(rootDir, filepath.Dir(path))
			if relPath == "." {
				relPath = ""
			}

			r.Entries = append(r.Entries, &Entry{
				Analyzer: AnalyzerTopology,
				Kind:     KindTechStack,
				Name:     "python",
				FilePath: relPath,
				Title:    "Python project detected",
				Metadata: map[string]string{"language": "python", "marker": marker},
			})
		})
	}
}

// detectRustProjects finds Cargo.toml files.
func (r *TopologyResult) detectRustProjects(rootDir string) {
	walkForFile(rootDir, "Cargo.toml", 3, func(path string) {
		relPath, _ := filepath.Rel(rootDir, filepath.Dir(path))
		if relPath == "." {
			relPath = ""
		}

		r.Entries = append(r.Entries, &Entry{
			Analyzer: AnalyzerTopology,
			Kind:     KindTechStack,
			Name:     "rust",
			FilePath: relPath,
			Title:    "Rust project detected",
			Metadata: map[string]string{"language": "rust", "marker": "Cargo.toml"},
		})

		r.Entries = append(r.Entries, &Entry{
			Analyzer: AnalyzerTopology,
			Kind:     KindModule,
			Name:     filepath.Base(filepath.Dir(path)),
			FilePath: relPath,
			Title:    fmt.Sprintf("Rust crate: %s", filepath.Base(filepath.Dir(path))),
			Metadata: map[string]string{"language": "rust"},
		})
	})
}

// detectBuildSystems finds common build system files at the root.
func (r *TopologyResult) detectBuildSystems(rootDir string) {
	buildFiles := map[string]string{
		"Makefile":       "make",
		"CMakeLists.txt": "cmake",
		"BUILD.bazel":    "bazel",
		"BUILD":          "bazel",
		"Justfile":       "just",
		"Taskfile.yml":   "task",
		"Rakefile":       "rake",
	}

	for file, system := range buildFiles {
		path := filepath.Join(rootDir, file)
		if _, err := os.Stat(path); err == nil {
			r.Entries = append(r.Entries, &Entry{
				Analyzer: AnalyzerTopology,
				Kind:     KindTechStack,
				Name:     system,
				FilePath: "",
				Title:    fmt.Sprintf("Build system: %s", system),
				Metadata: map[string]string{"build_system": system, "marker": file},
			})
		}
	}
}

// detectDocker finds Docker-related files.
func (r *TopologyResult) detectDocker(rootDir string) {
	dockerFiles := []string{"Dockerfile", "docker-compose.yml", "docker-compose.yaml", "compose.yml", "compose.yaml"}
	found := false
	for _, df := range dockerFiles {
		path := filepath.Join(rootDir, df)
		if _, err := os.Stat(path); err == nil && !found {
			r.Entries = append(r.Entries, &Entry{
				Analyzer: AnalyzerTopology,
				Kind:     KindTechStack,
				Name:     "docker",
				FilePath: "",
				Title:    "Docker detected",
				Metadata: map[string]string{"marker": df},
			})
			found = true
		}
	}

	// Also check for Dockerfiles in subdirectories (common pattern)
	walkForFile(rootDir, "Dockerfile", 2, func(path string) {
		if filepath.Dir(path) == rootDir {
			return // Already handled above
		}
		relPath, _ := filepath.Rel(rootDir, filepath.Dir(path))
		r.Entries = append(r.Entries, &Entry{
			Analyzer: AnalyzerTopology,
			Kind:     KindTechStack,
			Name:     "docker",
			FilePath: relPath,
			Title:    fmt.Sprintf("Dockerfile in %s", relPath),
			Metadata: map[string]string{"marker": "Dockerfile"},
		})
	})
}

// detectCICD finds CI/CD configuration.
func (r *TopologyResult) detectCICD(rootDir string) {
	ciFiles := map[string]string{
		".github/workflows":    "github-actions",
		".gitlab-ci.yml":       "gitlab-ci",
		"Jenkinsfile":          "jenkins",
		".circleci/config.yml": "circleci",
		".travis.yml":          "travis-ci",
		"azure-pipelines.yml":  "azure-devops",
	}

	for path, system := range ciFiles {
		fullPath := filepath.Join(rootDir, path)
		fi, err := os.Stat(fullPath)
		if err != nil {
			continue
		}
		// For directories (like .github/workflows), check it has files
		if fi.IsDir() {
			entries, err := os.ReadDir(fullPath)
			if err != nil || len(entries) == 0 {
				continue
			}
		}

		r.Entries = append(r.Entries, &Entry{
			Analyzer: AnalyzerTopology,
			Kind:     KindTechStack,
			Name:     system,
			FilePath: path,
			Title:    fmt.Sprintf("CI/CD: %s", system),
			Metadata: map[string]string{"ci_cd": system, "marker": path},
		})
	}
}

// detectMonorepoTools finds monorepo management tools.
func (r *TopologyResult) detectMonorepoTools(rootDir string) {
	monoFiles := map[string]string{
		"nx.json":             "nx",
		"lerna.json":          "lerna",
		"turbo.json":          "turborepo",
		"pnpm-workspace.yaml": "pnpm",
	}

	for file, tool := range monoFiles {
		path := filepath.Join(rootDir, file)
		if _, err := os.Stat(path); err == nil {
			r.Entries = append(r.Entries, &Entry{
				Analyzer: AnalyzerTopology,
				Kind:     KindWorkspace,
				Name:     tool,
				FilePath: "",
				Title:    fmt.Sprintf("Monorepo tool: %s", tool),
				Metadata: map[string]string{"monorepo": tool, "marker": file},
			})
		}
	}
}

// walkForFile walks the directory tree up to maxDepth levels looking for files with the given name.
func walkForFile(rootDir, fileName string, maxDepth int, fn func(path string)) {
	filepath.WalkDir(rootDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // Skip errors
		}

		// Calculate depth
		rel, _ := filepath.Rel(rootDir, path)
		depth := strings.Count(rel, string(filepath.Separator))
		if depth > maxDepth {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		// Skip hidden directories and common vendor dirs
		if d.IsDir() {
			base := d.Name()
			if strings.HasPrefix(base, ".") || base == "node_modules" || base == "vendor" || base == "__pycache__" || base == "target" {
				return filepath.SkipDir
			}
		}

		if !d.IsDir() && d.Name() == fileName {
			fn(path)
		}

		return nil
	})
}

// parseGoModuleName reads the module directive from a go.mod file.
func parseGoModuleName(goModPath string) string {
	f, err := os.Open(goModPath)
	if err != nil {
		return ""
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "module ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "module"))
		}
	}
	return ""
}

// parseJSONField does a quick scan for a "name": "value" pattern in a JSON file.
// This is intentionally simple — no full JSON parsing needed for this use case.
func parseJSONField(path, field string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	target := fmt.Sprintf(`"%s"`, field)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.Contains(line, target) {
			// Extract value: "name": "value"
			parts := strings.SplitN(line, ":", 2)
			if len(parts) < 2 {
				continue
			}
			val := strings.TrimSpace(parts[1])
			val = strings.Trim(val, `",`)
			if val != "" {
				return val
			}
		}
	}
	return ""
}
