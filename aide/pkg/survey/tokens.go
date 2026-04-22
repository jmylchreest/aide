package survey

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/jmylchreest/aide/aide/pkg/code"
)

// MetaEstTokens is the metadata key holding the estimated token cost of
// reading the file the entry points at. Survey MCP handlers sum this across
// returned entries to report a defensible "tokens saved" counterfactual —
// the tokens an agent would have paid to derive the same information by
// reading files directly.
const MetaEstTokens = "est_tokens"

// AnnotateEstTokens populates metadata[MetaEstTokens] on each entry whose
// FilePath resolves to a regular file. It tries rootDir/FilePath first, then
// walks up ancestors of rootDir looking for the file — necessary for the
// churn analyzer whose paths are reported relative to the git repo root,
// which may sit above the rootDir passed to this analyzer in a monorepo.
// Directory, missing, or zero-length paths are left without an estimate;
// aggregators treat absence as "no known cost".
func AnnotateEstTokens(rootDir string, entries []*Entry) {
	for _, e := range entries {
		if e == nil || e.FilePath == "" {
			continue
		}
		if isTokenCostExcluded(e.FilePath) {
			continue
		}
		abs := resolveEntryPath(rootDir, e.FilePath)
		if abs == "" {
			continue
		}
		info, err := os.Stat(abs)
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		tokens := code.EstimateTokensFromSize(e.FilePath, info.Size())
		if tokens <= 0 {
			continue
		}
		if e.Metadata == nil {
			e.Metadata = make(map[string]string, 1)
		}
		e.Metadata[MetaEstTokens] = strconv.Itoa(tokens)
	}
}

// isTokenCostExcluded reports whether a file should be skipped when computing
// the "tokens an agent would have paid to read this" counterfactual. An agent
// would essentially never choose to read these files even when chasing the
// information they represent, so claiming savings against them inflates the
// dashboard numbers without reflecting real avoided work.
//
// Covers: generated protobuf/gRPC stubs, dependency lockfiles, minified
// bundles, and vendored dependency trees. The list is deliberately narrow
// so we don't accidentally exclude hand-maintained files.
func isTokenCostExcluded(filePath string) bool {
	base := filepath.Base(filePath)
	slashed := filepath.ToSlash(filePath)

	if strings.HasSuffix(base, ".pb.go") ||
		strings.HasSuffix(base, "_grpc.pb.go") ||
		strings.HasSuffix(base, ".pb.ts") ||
		strings.HasSuffix(base, ".pb.js") {
		return true
	}
	if strings.HasSuffix(base, ".min.js") || strings.HasSuffix(base, ".min.css") {
		return true
	}
	switch base {
	case "package-lock.json", "yarn.lock", "pnpm-lock.yaml",
		"bun.lock", "bun.lockb",
		"Cargo.lock", "go.sum",
		"Gemfile.lock", "composer.lock", "poetry.lock", "Pipfile.lock":
		return true
	}
	if strings.Contains(slashed, "/vendor/") ||
		strings.Contains(slashed, "/node_modules/") ||
		strings.HasPrefix(slashed, "vendor/") ||
		strings.HasPrefix(slashed, "node_modules/") {
		return true
	}
	return false
}

// resolveEntryPath tries a handful of candidate locations to turn an entry's
// FilePath into an absolute path that actually exists. Returns "" if none match.
func resolveEntryPath(rootDir, filePath string) string {
	if filepath.IsAbs(filePath) {
		if _, err := os.Stat(filePath); err == nil {
			return filePath
		}
		return ""
	}
	// Try rootDir and each ancestor up to the filesystem root. This covers
	// the monorepo case where analyzers hand back repo-root-relative paths
	// while the caller only knows about a sub-module root.
	base, _ := filepath.Abs(rootDir)
	if base == "" {
		base = rootDir
	}
	for dir := base; dir != ""; dir = filepath.Dir(dir) {
		candidate := filepath.Join(dir, filePath)
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
		if parent := filepath.Dir(dir); parent == dir {
			break
		}
	}
	return ""
}

// EstTokensFor returns the parsed est_tokens value on the entry, or 0 when
// the metadata is absent or malformed. Intended for handler-side aggregation
// when computing the "tokens saved" counterfactual.
func EstTokensFor(e *Entry) int {
	if e == nil || e.Metadata == nil {
		return 0
	}
	raw, ok := e.Metadata[MetaEstTokens]
	if !ok {
		return 0
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 0 {
		return 0
	}
	return n
}
