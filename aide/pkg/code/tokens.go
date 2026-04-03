package code

import (
	"path/filepath"
	"strings"
)

// TokenRatios maps language to chars-per-token for estimation.
// Calibrated against Anthropic count_tokens API (claude-sonnet-4-20250514)
// using the calibrate-tokens tool on a mixed Go/TypeScript/Python codebase.
var TokenRatios = map[string]float64{
	"css":        2.73,
	"go":         2.79,
	"html":       2.94,
	"javascript": 2.46,
	"json":       2.72,
	"markdown":   3.69,
	"proto":      3.27,
	"python":     3.44,
	"ruby":       2.63,
	"shell":      2.97,
	"typescript":  3.22,
	"yaml":       3.28,
}

// DefaultCharsPerToken is the fallback ratio for unknown languages.
// Weighted mean of calibrated ratios across all sampled languages.
const DefaultCharsPerToken = 3.0

// extToLanguage maps file extensions to language keys used in TokenRatios.
var extToLanguage = map[string]string{
	".go":    "go",
	".ts":    "typescript",
	".tsx":   "typescript",
	".js":    "javascript",
	".jsx":   "javascript",
	".mjs":   "javascript",
	".cjs":   "javascript",
	".py":    "python",
	".rs":    "rust",
	".java":  "java",
	".c":     "c",
	".h":     "c",
	".cpp":   "cpp",
	".cc":    "cpp",
	".hpp":   "cpp",
	".rb":    "ruby",
	".sh":    "shell",
	".bash":  "shell",
	".zsh":   "shell",
	".json":  "json",
	".yaml":  "yaml",
	".yml":   "yaml",
	".toml":  "yaml",
	".md":    "markdown",
	".txt":   "markdown",
	".html":  "html",
	".htm":   "html",
	".css":   "css",
	".scss":  "css",
	".proto": "proto",
	".sql":   "sql",
	".zig":   "go", // zig syntax density is similar to go
}

// EstimateTokens estimates the token count for content based on its file path.
// Uses calibrated per-language ratios when available, falls back to DefaultCharsPerToken.
func EstimateTokens(filePath string, contentLen int) int {
	ratio := charsPerTokenForFile(filePath)
	if contentLen <= 0 || ratio <= 0 {
		return 0
	}
	return int(float64(contentLen)/ratio + 0.5)
}

// EstimateTokensFromSize estimates token count from file size in bytes.
// Slightly less accurate than char-based since multi-byte chars exist,
// but avoids reading the file. For ASCII-dominated source code the
// difference is negligible.
func EstimateTokensFromSize(filePath string, sizeBytes int64) int {
	ratio := charsPerTokenForFile(filePath)
	if sizeBytes <= 0 || ratio <= 0 {
		return 0
	}
	return int(float64(sizeBytes)/ratio + 0.5)
}

// charsPerTokenForFile returns the calibrated ratio for a file's language.
func charsPerTokenForFile(filePath string) float64 {
	ext := strings.ToLower(filepath.Ext(filePath))
	if lang, ok := extToLanguage[ext]; ok {
		if ratio, ok := TokenRatios[lang]; ok {
			return ratio
		}
	}
	return DefaultCharsPerToken
}
