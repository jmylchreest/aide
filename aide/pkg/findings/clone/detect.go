package clone

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/jmylchreest/aide/aide/pkg/aideignore"
	"github.com/jmylchreest/aide/aide/pkg/code"
	"github.com/jmylchreest/aide/aide/pkg/findings"
	"github.com/jmylchreest/aide/aide/pkg/grammar"
)

// Config configures the clone detection analyzer.
type Config struct {
	// WindowSize is the number of tokens in the sliding hash window
	// (default DefaultWindowSize).
	WindowSize int
	// MinCloneLines is the minimum number of source lines for a clone to
	// be reported (default DefaultMinCloneLines).
	MinCloneLines int
	// MinMatchCount is the minimum number of matching hash windows required per
	// clone region. Regions with fewer matches are filtered out
	// (default DefaultMinMatchCount).
	MinMatchCount int
	// MinSimilarity is the minimum similarity ratio (0.0–1.0) for a clone region.
	// Similarity = matchCount / max(windowsInRegionA, windowsInRegionB).
	// (default DefaultMinSimilarity — disabled).
	MinSimilarity float64
	// MaxBucketSize caps how many locations a single hash may appear in before
	// it is considered "too common" (boilerplate) and excluded
	// (default DefaultMaxBucketSize). Set to 0 to disable the cap.
	MaxBucketSize int
	// LanguageIsolation restricts clone detection to same-language file pairs
	// (default DefaultLanguageIsolation).
	LanguageIsolation *bool
	// SevWarningLines is the line-span threshold for warning severity
	// (default DefaultSevWarningLines).
	SevWarningLines int
	// SevCriticalLines is the line-span threshold for critical severity
	// (default DefaultSevCriticalLines).
	SevCriticalLines int
	// Paths to analyze (default: current directory).
	Paths []string
	// ProgressFn is called after each file is tokenized. May be nil.
	ProgressFn func(path string, tokens int)
	// Ignore is the aideignore matcher for filtering files/directories.
	// If nil, built-in defaults are used.
	Ignore *aideignore.Matcher
	// Loader is the grammar loader for tree-sitter languages.
	// If nil, a default CompositeLoader is created.
	Loader grammar.Loader
}

// Result holds the output of a clone detection run.
type Result struct {
	FilesAnalyzed int
	FilesSkipped  int
	CloneGroups   int
	FindingsCount int
	Duration      time.Duration
	// BucketsSkipped is the number of hash buckets that exceeded MaxBucketSize.
	BucketsSkipped int
	// CollisionsFiltered is the number of hash matches rejected by token verification.
	CollisionsFiltered int
}

// defaults returns the effective value for each configurable parameter.
func (cfg *Config) defaults() (windowSize, minLines, minMatchCount, maxBucket, sevWarn, sevCrit int, minSim float64, langIso bool) {
	windowSize = DefaultWindowSize
	if cfg.WindowSize > 0 {
		windowSize = cfg.WindowSize
	}
	minLines = DefaultMinCloneLines
	if cfg.MinCloneLines > 0 {
		minLines = cfg.MinCloneLines
	}
	minMatchCount = DefaultMinMatchCount
	if cfg.MinMatchCount > 0 {
		minMatchCount = cfg.MinMatchCount
	}
	maxBucket = DefaultMaxBucketSize
	if cfg.MaxBucketSize > 0 {
		maxBucket = cfg.MaxBucketSize
	} else if cfg.MaxBucketSize < 0 {
		// Explicitly disabled.
		maxBucket = 0
	}
	sevWarn = DefaultSevWarningLines
	if cfg.SevWarningLines > 0 {
		sevWarn = cfg.SevWarningLines
	}
	sevCrit = DefaultSevCriticalLines
	if cfg.SevCriticalLines > 0 {
		sevCrit = cfg.SevCriticalLines
	}
	minSim = cfg.MinSimilarity
	langIso = DefaultLanguageIsolation
	if cfg.LanguageIsolation != nil {
		langIso = *cfg.LanguageIsolation
	}
	return
}

// tokenizedFile holds the tokenisation result for a single file.
type tokenizedFile struct {
	relPath string
	lang    string
	seq     *TokenSequence
	err     error
}

// DetectClones analyzes source files for code clones using Rabin-Karp hashing.
// It returns findings for each clone group and a result summary.
func DetectClones(cfg Config) ([]*findings.Finding, *Result, error) {
	start := time.Now()
	windowSize, minLines, minMatchCount, maxBucket, sevWarn, sevCrit, minSim, langIso := cfg.defaults()

	paths := cfg.Paths
	if len(paths) == 0 {
		paths = []string{"."}
	}
	if cfg.Loader == nil {
		cfg.Loader = grammar.NewCompositeLoader()
	}

	result := &Result{}
	index := NewCloneIndex()
	index.MaxBucketSize = maxBucket
	hasher := NewRollingHash(windowSize)

	ignore := cfg.Ignore
	if ignore == nil {
		ignore = aideignore.NewFromDefaults()
	}

	// Phase 1: Collect file paths, then tokenize in parallel.
	type fileEntry struct {
		path    string
		relPath string
		lang    string
	}
	var files []fileEntry

	for _, root := range paths {
		absRoot, _ := filepath.Abs(root)
		shouldSkip := ignore.WalkFunc(absRoot)

		err := filepath.Walk(root, func(path string, info os.FileInfo, walkErr error) error {
			if walkErr != nil {
				return nil
			}

			if skip, skipDir := shouldSkip(path, info); skip {
				if skipDir {
					return filepath.SkipDir
				}
				result.FilesSkipped++
				return nil
			}
			if info.IsDir() {
				return nil
			}

			if !code.SupportedFile(path) {
				result.FilesSkipped++
				return nil
			}
			lang := code.DetectLanguage(path, nil)
			if lang == "" {
				result.FilesSkipped++
				return nil
			}

			// Skip very large files.
			if info.Size() > MaxFileSize {
				result.FilesSkipped++
				return nil
			}

			relPath, _ := filepath.Rel(".", path)
			if relPath == "" {
				relPath = path
			}

			files = append(files, fileEntry{path: path, relPath: relPath, lang: lang})
			return nil
		})
		if err != nil {
			return nil, nil, fmt.Errorf("error walking %s: %w", root, err)
		}
	}

	// Parallel tokenisation.
	workers := runtime.GOMAXPROCS(0)
	if workers > 16 {
		workers = 16
	}
	if workers < 1 {
		workers = 1
	}

	results := make([]tokenizedFile, len(files))
	var wg sync.WaitGroup
	ch := make(chan int, len(files))

	for i := range files {
		ch <- i
	}
	close(ch)

	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range ch {
				fe := files[i]
				content, err := os.ReadFile(fe.path)
				if err != nil {
					results[i] = tokenizedFile{relPath: fe.relPath, err: err}
					continue
				}
				seq, err := Tokenize(cfg.Loader, fe.relPath, content, fe.lang)
				results[i] = tokenizedFile{relPath: fe.relPath, lang: fe.lang, seq: seq, err: err}
			}
		}()
	}
	wg.Wait()

	// Build index from tokenised results.
	for _, tf := range results {
		if tf.err != nil || tf.seq == nil || len(tf.seq.Tokens) < windowSize {
			result.FilesSkipped++
			continue
		}

		result.FilesAnalyzed++
		hashes := hasher.ComputeHashes(tf.seq.Tokens)
		index.AddFile(tf.relPath, hashes, tf.lang, tf.seq.Tokens)

		if cfg.ProgressFn != nil {
			cfg.ProgressFn(tf.relPath, len(tf.seq.Tokens))
		}
	}

	// Phase 2: Find clone groups.
	pairsResult := index.ClonePairs(windowSize, langIso)
	groups := pairsResult.Groups
	result.CloneGroups = len(groups)
	result.BucketsSkipped = pairsResult.BucketsSkipped

	// Phase 3: Build verified match windows for each file pair,
	// then split into contiguous regions.

	// Collect all verified window matches per canonical file pair.
	type filePairKey struct {
		fileA string
		fileB string
	}
	pairWindows := make(map[filePairKey][]windowMatch)

	collisions := 0
	for _, g := range groups {
		for i := 0; i < len(g.Locations); i++ {
			for j := i + 1; j < len(g.Locations); j++ {
				a := g.Locations[i]
				b := g.Locations[j]

				// Verify that the token sequences actually match (not just hash collision).
				if !index.VerifyMatch(a, b, windowSize) {
					collisions++
					continue
				}

				// Canonical order.
				if a.FilePath > b.FilePath || (a.FilePath == b.FilePath && a.StartLine > b.StartLine) {
					a, b = b, a
				}

				key := filePairKey{fileA: a.FilePath, fileB: b.FilePath}
				pairWindows[key] = append(pairWindows[key], windowMatch{
					tokenIdxA: a.TokenIdx,
					tokenIdxB: b.TokenIdx,
					startA:    a.StartLine,
					endA:      a.EndLine,
					startB:    b.StartLine,
					endB:      b.EndLine,
				})
			}
		}
	}
	result.CollisionsFiltered = collisions

	// Split each file pair's windows into contiguous clone regions, then
	// group by unique source location. This produces one finding per
	// deduplicated code block (like PMD CPD / jscpd) instead of one per
	// pair-side.

	grouped := make(map[regionKey]*groupedRegion)

	for key, windows := range pairWindows {
		regions := splitIntoRegions(windows, windowSize)

		for _, region := range regions {
			// Apply minimum match count filter.
			if region.matchCount < minMatchCount {
				continue
			}

			// Apply similarity ratio filter.
			if minSim > 0 {
				spanA := region.endA - region.startA + 1
				spanB := region.endB - region.startB + 1
				maxSpan := spanA
				if spanB > maxSpan {
					maxSpan = spanB
				}
				sim := float64(region.matchCount) / float64(maxSpan)
				if sim < minSim {
					continue
				}
			}

			// Register both sides of the pair. For each side, the other
			// side is a clone location.
			sides := [2]struct {
				file, otherFile          string
				start, end, oStart, oEnd int
			}{
				{key.fileA, key.fileB, region.startA, region.endA, region.startB, region.endB},
				{key.fileB, key.fileA, region.startB, region.endB, region.startA, region.endA},
			}

			for _, s := range sides {
				if s.end-s.start+1 < minLines {
					continue
				}

				rk := regionKey{file: s.file, startLine: s.start, endLine: s.end}
				gr := grouped[rk]
				if gr == nil {
					gr = &groupedRegion{
						file:      s.file,
						startLine: s.start,
						endLine:   s.end,
					}
					grouped[rk] = gr
				}
				if region.matchCount > gr.matchCount {
					gr.matchCount = region.matchCount
				}

				// Deduplicate: avoid adding the same clone location twice.
				loc := cloneLocation{File: s.otherFile, StartLine: s.oStart, EndLine: s.oEnd}
				found := false
				for _, existing := range gr.locations {
					if existing == loc {
						found = true
						break
					}
				}
				if !found {
					gr.locations = append(gr.locations, loc)
				}
			}
		}
	}

	// Merge overlapping regions in the same file. Two regions overlap if
	// one starts before the other ends. Merging prevents the same code
	// block from appearing as multiple findings when pair boundaries
	// differ slightly.
	merged := mergeOverlappingRegions(grouped)

	// Emit one finding per unique merged region.
	var allFindings []*findings.Finding

	for _, gr := range merged {
		lineSpan := gr.endLine - gr.startLine + 1

		severity := findings.SevInfo
		if lineSpan >= sevWarn {
			severity = findings.SevWarning
		}
		if lineSpan >= sevCrit {
			severity = findings.SevCritical
		}

		// Build human-readable detail listing all clone locations.
		nLocs := len(gr.locations)
		var detail string
		if nLocs == 1 {
			loc := gr.locations[0]
			detail = fmt.Sprintf(
				"Duplicated code block (lines %d–%d) also found in %s:%d–%d. %d matching hash windows.",
				gr.startLine, gr.endLine, loc.File, loc.StartLine, loc.EndLine, gr.matchCount)
		} else {
			var sb strings.Builder
			fmt.Fprintf(&sb, "Duplicated code block (lines %d–%d) found in %d other locations:\n",
				gr.startLine, gr.endLine, nLocs)
			for _, loc := range gr.locations {
				fmt.Fprintf(&sb, "  • %s:%d–%d\n", loc.File, loc.StartLine, loc.EndLine)
			}
			fmt.Fprintf(&sb, "%d matching hash windows.", gr.matchCount)
			detail = sb.String()
		}

		// Serialise clone locations into metadata.
		var locParts []string
		for _, loc := range gr.locations {
			locParts = append(locParts, fmt.Sprintf("%s:%d-%d", loc.File, loc.StartLine, loc.EndLine))
		}

		f := &findings.Finding{
			Analyzer: findings.AnalyzerClones,
			Severity: severity,
			Category: "duplicate",
			FilePath: gr.file,
			Line:     gr.startLine,
			EndLine:  gr.endLine,
			Title:    fmt.Sprintf("Code clone detected (~%d lines, %d location%s)", lineSpan, nLocs, pluralS(nLocs)),
			Detail:   detail,
			Metadata: map[string]string{
				"clone_locations": strings.Join(locParts, ";"),
				"clone_count":     fmt.Sprintf("%d", nLocs),
				"line_span":       fmt.Sprintf("%d", lineSpan),
				"match_count":     fmt.Sprintf("%d", gr.matchCount),
			},
			CreatedAt: time.Now(),
		}
		allFindings = append(allFindings, f)
	}

	result.FindingsCount = len(allFindings)
	result.Duration = time.Since(start)

	return allFindings, result, nil
}

// windowMatch represents a single verified hash-window match between two files.
type windowMatch struct {
	tokenIdxA int
	tokenIdxB int
	startA    int
	endA      int
	startB    int
	endB      int
}

// cloneRegion represents a contiguous region of matching windows between
// two files. Instead of merging all windows into one mega-range, adjacent
// windows are grouped so that gaps produce separate regions.
type cloneRegion struct {
	startA     int // Start source line in file A.
	endA       int // End source line in file A.
	startB     int // Start source line in file B.
	endB       int // End source line in file B.
	matchCount int // Number of matching hash windows in this region.
}

// splitIntoRegions groups window matches into contiguous clone regions.
//
// Two consecutive windows are considered "contiguous" if the gap between
// them (in token indices on BOTH sides) is at most windowSize. This means
// the sliding windows overlap or are adjacent. A gap larger than windowSize
// starts a new region.
func splitIntoRegions(windows []windowMatch, windowSize int) []cloneRegion {
	if len(windows) == 0 {
		return nil
	}

	// Sort by token index on side A, breaking ties by side B.
	sort.Slice(windows, func(i, j int) bool {
		if windows[i].tokenIdxA != windows[j].tokenIdxA {
			return windows[i].tokenIdxA < windows[j].tokenIdxA
		}
		return windows[i].tokenIdxB < windows[j].tokenIdxB
	})

	// Deduplicate: same (tokenIdxA, tokenIdxB) from different hash buckets.
	deduped := windows[:0:0]
	seen := make(map[[2]int]bool, len(windows))
	for _, w := range windows {
		key := [2]int{w.tokenIdxA, w.tokenIdxB}
		if !seen[key] {
			seen[key] = true
			deduped = append(deduped, w)
		}
	}
	windows = deduped

	if len(windows) == 0 {
		return nil
	}

	// Group contiguous windows into regions.
	var regions []cloneRegion
	cur := cloneRegion{
		startA:     windows[0].startA,
		endA:       windows[0].endA,
		startB:     windows[0].startB,
		endB:       windows[0].endB,
		matchCount: 1,
	}
	prevA := windows[0].tokenIdxA
	prevB := windows[0].tokenIdxB

	for _, w := range windows[1:] {
		gapA := w.tokenIdxA - prevA
		gapB := w.tokenIdxB - prevB

		// Contiguous if both sides have gaps within windowSize.
		// Also require side B to move forward (not jump backwards to a
		// different region).
		if gapA <= windowSize && gapB >= 0 && gapB <= windowSize {
			// Extend current region.
			if w.endA > cur.endA {
				cur.endA = w.endA
			}
			if w.startA < cur.startA {
				cur.startA = w.startA
			}
			if w.endB > cur.endB {
				cur.endB = w.endB
			}
			if w.startB < cur.startB {
				cur.startB = w.startB
			}
			cur.matchCount++
		} else {
			// Gap detected — start a new region.
			regions = append(regions, cur)
			cur = cloneRegion{
				startA:     w.startA,
				endA:       w.endA,
				startB:     w.startB,
				endB:       w.endB,
				matchCount: 1,
			}
		}
		prevA = w.tokenIdxA
		prevB = w.tokenIdxB
	}
	regions = append(regions, cur)

	return regions
}

// regionKey identifies a unique source region in one file.
type regionKey struct {
	file      string
	startLine int
	endLine   int
}

// groupedRegion accumulates all clone locations for a unique source region.
type groupedRegion struct {
	file       string
	startLine  int
	endLine    int
	matchCount int // Best (highest) match count from any pair.
	locations  []cloneLocation
}

// cloneLocation records where else a region was found.
type cloneLocation struct {
	File      string
	StartLine int
	EndLine   int
}

// mergedRegion is the output of mergeOverlappingRegions — a single unique
// code region with all the places it was found duplicated.
type mergedRegion struct {
	file       string
	startLine  int
	endLine    int
	matchCount int
	locations  []cloneLocation
}

// mergeOverlappingRegions collapses overlapping grouped regions in the same
// file into a single region. Two regions overlap if one starts before the
// other ends. Their clone location lists are unioned and deduplicated.
func mergeOverlappingRegions(grouped map[regionKey]*groupedRegion) []*mergedRegion {
	// Group all regions by file.
	byFile := make(map[string][]*groupedRegion)
	for _, gr := range grouped {
		byFile[gr.file] = append(byFile[gr.file], gr)
	}

	var result []*mergedRegion

	for file, regions := range byFile {
		// Sort by start line, then by end line descending (wider first).
		sort.Slice(regions, func(i, j int) bool {
			if regions[i].startLine != regions[j].startLine {
				return regions[i].startLine < regions[j].startLine
			}
			return regions[i].endLine > regions[j].endLine
		})

		// Sweep-line merge of overlapping intervals.
		cur := &mergedRegion{
			file:       file,
			startLine:  regions[0].startLine,
			endLine:    regions[0].endLine,
			matchCount: regions[0].matchCount,
			locations:  copyLocations(regions[0].locations),
		}

		for _, r := range regions[1:] {
			if r.startLine <= cur.endLine+1 {
				// Overlapping or adjacent — merge.
				if r.endLine > cur.endLine {
					cur.endLine = r.endLine
				}
				if r.matchCount > cur.matchCount {
					cur.matchCount = r.matchCount
				}
				// Union the location lists.
				for _, loc := range r.locations {
					if !hasLocation(cur.locations, loc) {
						cur.locations = append(cur.locations, loc)
					}
				}
			} else {
				// No overlap — flush current and start new.
				result = append(result, cur)
				cur = &mergedRegion{
					file:       file,
					startLine:  r.startLine,
					endLine:    r.endLine,
					matchCount: r.matchCount,
					locations:  copyLocations(r.locations),
				}
			}
		}
		result = append(result, cur)
	}

	// Sort output deterministically: by file, then start line.
	sort.Slice(result, func(i, j int) bool {
		if result[i].file != result[j].file {
			return result[i].file < result[j].file
		}
		return result[i].startLine < result[j].startLine
	})

	return result
}

// copyLocations returns a copy of the location slice.
func copyLocations(locs []cloneLocation) []cloneLocation {
	out := make([]cloneLocation, len(locs))
	copy(out, locs)
	return out
}

// hasLocation returns true if loc is already in the list.
func hasLocation(list []cloneLocation, loc cloneLocation) bool {
	for _, existing := range list {
		if existing == loc {
			return true
		}
	}
	return false
}

// pluralS returns "s" if n != 1, for English pluralisation.
func pluralS(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}
