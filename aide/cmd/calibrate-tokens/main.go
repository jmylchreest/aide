// calibrate-tokens samples files from a project and uses the Anthropic
// count_tokens API to compute accurate chars-per-token ratios per language.
//
// Supports two modes:
//
//	Direct Anthropic API (recommended, free):
//	  ANTHROPIC_API_KEY=sk-... go run ./cmd/calibrate-tokens [directory]
//
//	OpenRouter fallback (uses max_tokens=1 completion, costs ~input tokens per file):
//	  OPENROUTER_API_KEY=sk-or-... go run ./cmd/calibrate-tokens [directory]
//
// The output is a calibration table that can be used to improve token
// estimation accuracy in aide's code indexer.
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Maximum files to sample per language group.
const maxPerLanguage = 20

// Maximum file size to send (avoid huge files).
const maxFileSize = 100 * 1024 // 100KB

// API endpoints.
const (
	anthropicCountURL = "https://api.anthropic.com/v1/messages/count_tokens"
	openrouterMsgURL  = "https://openrouter.ai/api/v1/messages"
)

// Model to use for token counting.
const model = "claude-sonnet-4-20250514"

// Language groups by file extension.
var languageMap = map[string]string{
	// Go
	".go": "go",
	// TypeScript / JavaScript
	".ts": "typescript", ".tsx": "typescript",
	".js": "javascript", ".jsx": "javascript",
	// Python
	".py": "python",
	// Rust
	".rs": "rust",
	// Java
	".java": "java",
	// C / C++
	".c": "c", ".h": "c",
	".cpp": "cpp", ".cc": "cpp", ".hpp": "cpp",
	// Ruby
	".rb": "ruby",
	// Shell
	".sh": "shell", ".bash": "shell", ".zsh": "shell",
	// Config / Data
	".json": "json", ".yaml": "yaml", ".yml": "yaml",
	".toml": "toml",
	// Markup / Prose
	".md": "markdown", ".txt": "text",
	// HTML / CSS
	".html": "html", ".htm": "html",
	".css": "css", ".scss": "css",
	// Proto
	".proto": "proto",
	// SQL
	".sql": "sql",
	// Zig
	".zig": "zig",
}

// Skip directories.
var skipDirs = map[string]bool{
	".git": true, "node_modules": true, "vendor": true,
	".aide": true, "dist": true, "build": true,
	"__pycache__": true, "target": true, ".next": true,
}

type apiMode int

const (
	modeAnthropic apiMode = iota
	modeOpenRouter
)

type fileEntry struct {
	path     string
	language string
	size     int64
}

type result struct {
	Path          string  `json:"path"`
	Language      string  `json:"language"`
	Bytes         int64   `json:"bytes"`
	Chars         int     `json:"chars"`
	Tokens        int     `json:"tokens"`
	CharsPerToken float64 `json:"chars_per_token"`
	BytesPerToken float64 `json:"bytes_per_token"`
}

type langSummary struct {
	Language      string  `json:"language"`
	Files         int     `json:"files"`
	TotalChars    int     `json:"total_chars"`
	TotalTokens   int     `json:"total_tokens"`
	CharsPerToken float64 `json:"chars_per_token"`
	BytesPerToken float64 `json:"bytes_per_token"`
	StdDev        float64 `json:"std_dev"`
}

func main() {
	// Determine API mode.
	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	mode := modeAnthropic

	if apiKey == "" {
		apiKey = os.Getenv("OPENROUTER_API_KEY")
		if apiKey == "" {
			fmt.Fprintln(os.Stderr, "Either ANTHROPIC_API_KEY or OPENROUTER_API_KEY is required")
			fmt.Fprintln(os.Stderr, "")
			fmt.Fprintln(os.Stderr, "Usage:")
			fmt.Fprintln(os.Stderr, "  ANTHROPIC_API_KEY=sk-...    go run ./cmd/calibrate-tokens [directory]  # Free, uses count_tokens")
			fmt.Fprintln(os.Stderr, "  OPENROUTER_API_KEY=sk-or-... go run ./cmd/calibrate-tokens [directory]  # Uses max_tokens=1 completion")
			os.Exit(1)
		}
		mode = modeOpenRouter
	}

	dir := "."
	if len(os.Args) > 1 {
		dir = os.Args[1]
	}

	absDir, err := filepath.Abs(dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "bad directory: %v\n", err)
		os.Exit(1)
	}

	if mode == modeAnthropic {
		fmt.Fprintf(os.Stderr, "Mode: Anthropic count_tokens API (free, exact)\n")
	} else {
		fmt.Fprintf(os.Stderr, "Mode: OpenRouter max_tokens=1 fallback (costs input tokens per file)\n")
	}
	fmt.Fprintf(os.Stderr, "Scanning %s for files...\n", absDir)

	// Collect candidate files grouped by language.
	byLang := make(map[string][]fileEntry)

	_ = filepath.Walk(absDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			if skipDirs[info.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if info.Size() == 0 || info.Size() > maxFileSize {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		lang, ok := languageMap[ext]
		if !ok {
			return nil
		}
		byLang[lang] = append(byLang[lang], fileEntry{
			path:     path,
			language: lang,
			size:     info.Size(),
		})
		return nil
	})

	// Sample up to maxPerLanguage files per language, preferring a range of sizes.
	var sampled []fileEntry
	for lang, files := range byLang {
		sort.Slice(files, func(i, j int) bool { return files[i].size < files[j].size })
		n := len(files)
		if n > maxPerLanguage {
			// Take evenly spaced samples across the size range.
			step := float64(n) / float64(maxPerLanguage)
			selected := make([]fileEntry, 0, maxPerLanguage)
			for i := 0; i < maxPerLanguage; i++ {
				idx := int(float64(i) * step)
				if idx >= n {
					idx = n - 1
				}
				selected = append(selected, files[idx])
			}
			files = selected
		}
		sampled = append(sampled, files...)
		fmt.Fprintf(os.Stderr, "  %s: %d files sampled (of %d)\n", lang, len(files), n)
	}

	fmt.Fprintf(os.Stderr, "\nCounting tokens for %d files...\n", len(sampled))

	// Process each file.
	var results []result
	client := &http.Client{Timeout: 30 * time.Second}

	for i, entry := range sampled {
		content, err := os.ReadFile(entry.path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  skip %s: %v\n", entry.path, err)
			continue
		}

		relPath, _ := filepath.Rel(absDir, entry.path)

		var tokens int
		if mode == modeAnthropic {
			tokens, err = countTokensAnthropic(client, apiKey, string(content))
		} else {
			tokens, err = countTokensOpenRouter(client, apiKey, string(content))
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "  error %s: %v\n", relPath, err)
			continue
		}

		chars := len([]rune(string(content)))
		cpt := float64(chars) / float64(tokens)
		bpt := float64(entry.size) / float64(tokens)

		results = append(results, result{
			Path:          relPath,
			Language:      entry.language,
			Bytes:         entry.size,
			Chars:         chars,
			Tokens:        tokens,
			CharsPerToken: math.Round(cpt*100) / 100,
			BytesPerToken: math.Round(bpt*100) / 100,
		})

		if (i+1)%10 == 0 {
			fmt.Fprintf(os.Stderr, "  processed %d/%d files\n", i+1, len(sampled))
		}

		// Rate limiting.
		if mode == modeOpenRouter {
			time.Sleep(200 * time.Millisecond) // more conservative for completions
		} else {
			time.Sleep(50 * time.Millisecond)
		}
	}

	// Aggregate per language.
	langData := make(map[string][]result)
	for _, r := range results {
		langData[r.Language] = append(langData[r.Language], r)
	}

	var summaries []langSummary
	for lang, rs := range langData {
		totalChars := 0
		totalTokens := 0
		totalBytes := int64(0)
		for _, r := range rs {
			totalChars += r.Chars
			totalTokens += r.Tokens
			totalBytes += r.Bytes
		}
		avgCPT := float64(totalChars) / float64(totalTokens)

		// Compute std dev of chars_per_token.
		var sumSqDiff float64
		for _, r := range rs {
			diff := r.CharsPerToken - avgCPT
			sumSqDiff += diff * diff
		}
		stdDev := math.Sqrt(sumSqDiff / float64(len(rs)))

		summaries = append(summaries, langSummary{
			Language:      lang,
			Files:         len(rs),
			TotalChars:    totalChars,
			TotalTokens:   totalTokens,
			CharsPerToken: math.Round(avgCPT*100) / 100,
			BytesPerToken: math.Round(float64(totalBytes)/float64(totalTokens)*100) / 100,
			StdDev:        math.Round(stdDev*100) / 100,
		})
	}

	sort.Slice(summaries, func(i, j int) bool {
		return summaries[i].Language < summaries[j].Language
	})

	// Output.
	fmt.Println("# Token Calibration Results")
	fmt.Println()
	fmt.Printf("Directory: %s\n", absDir)
	fmt.Printf("Files sampled: %d\n", len(results))
	if mode == modeAnthropic {
		fmt.Println("Method: Anthropic count_tokens API (exact)")
	} else {
		fmt.Println("Method: OpenRouter max_tokens=1 fallback (from usage.input_tokens)")
	}
	fmt.Printf("Model: %s\n", model)
	fmt.Printf("Date: %s\n", time.Now().Format("2006-01-02"))
	fmt.Println()
	fmt.Println("## Per-Language Summary")
	fmt.Println()
	fmt.Printf("%-14s %6s %10s %10s %8s %8s %7s\n",
		"Language", "Files", "Chars", "Tokens", "Ch/Tok", "By/Tok", "StdDev")
	fmt.Println(strings.Repeat("-", 72))

	for _, s := range summaries {
		fmt.Printf("%-14s %6d %10d %10d %8.2f %8.2f %7.2f\n",
			s.Language, s.Files, s.TotalChars, s.TotalTokens,
			s.CharsPerToken, s.BytesPerToken, s.StdDev)
	}

	// Output Go constant suggestion.
	fmt.Println()
	fmt.Println("## Suggested Go Constants")
	fmt.Println()
	fmt.Println("```go")
	fmt.Println("// TokenRatios maps language to chars-per-token for estimation.")
	fmt.Println("// Calibrated against Anthropic count_tokens API.")
	fmt.Println("var TokenRatios = map[string]float64{")
	for _, s := range summaries {
		fmt.Printf("\t%s %*.2f,\n", fmt.Sprintf("%q:", s.Language), 14-len(s.Language), s.CharsPerToken)
	}
	fmt.Println("}")
	fmt.Println("```")

	// Dump raw JSON for further analysis.
	fmt.Println()
	fmt.Println("## Raw Data (JSON)")
	fmt.Println()
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(map[string]interface{}{
		"summaries": summaries,
		"files":     results,
	})
}

// countTokensAnthropic uses the free count_tokens endpoint (Anthropic API key required).
func countTokensAnthropic(client *http.Client, apiKey, content string) (int, error) {
	type msg struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}
	reqBody, _ := json.Marshal(map[string]interface{}{
		"model":    model,
		"messages": []msg{{Role: "user", Content: content}},
	})

	req, err := http.NewRequest("POST", anthropicCountURL, bytes.NewReader(reqBody))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, err
	}

	if resp.StatusCode != 200 {
		return 0, fmt.Errorf("API error %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		InputTokens int `json:"input_tokens"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return 0, err
	}
	return result.InputTokens, nil
}

// countTokensOpenRouter uses a max_tokens=1 completion to get usage.input_tokens.
// This incurs a small cost (input tokens + 1 output token per call).
func countTokensOpenRouter(client *http.Client, apiKey, content string) (int, error) {
	type msg struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}
	reqBody, _ := json.Marshal(map[string]interface{}{
		"model":      "anthropic/" + model,
		"max_tokens": 1,
		"messages":   []msg{{Role: "user", Content: "Count only. " + content}},
	})

	req, err := http.NewRequest("POST", openrouterMsgURL, bytes.NewReader(reqBody))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, err
	}

	if resp.StatusCode != 200 {
		return 0, fmt.Errorf("API error %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Usage struct {
			InputTokens int `json:"input_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return 0, err
	}

	if result.Usage.InputTokens == 0 {
		return 0, fmt.Errorf("no input_tokens in response")
	}
	return result.Usage.InputTokens, nil
}
