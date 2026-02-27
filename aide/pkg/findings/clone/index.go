package clone

import (
	"sort"
	"sync"
)

// CloneIndex maps rolling hashes to the file locations where they appear.
// Multiple locations for the same hash indicate potential clones.
type CloneIndex struct {
	mu      sync.RWMutex
	entries map[uint64][]Location

	// MaxBucketSize caps the number of locations per hash bucket.
	// Hashes appearing in more locations than this are considered
	// "too common" (boilerplate) and are excluded from clone detection.
	// Zero means unlimited (no cap).
	MaxBucketSize int

	// tokenStore holds per-file token sequences for post-hash verification.
	tokenStore map[string][]Token
}

// Location identifies a hash occurrence in a specific file.
type Location struct {
	FilePath  string
	Lang      string // Language of the file (for cross-language isolation).
	TokenIdx  int
	StartLine int
	EndLine   int
}

// NewCloneIndex creates an empty clone index.
func NewCloneIndex() *CloneIndex {
	return &CloneIndex{
		entries:    make(map[uint64][]Location),
		tokenStore: make(map[string][]Token),
	}
}

// AddFile indexes all rolling hashes for a tokenized file and stores its
// token sequence for later hash verification.
func (idx *CloneIndex) AddFile(filePath string, hashes []HashEntry, lang string, tokens []Token) {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	for _, h := range hashes {
		loc := Location{
			FilePath:  filePath,
			Lang:      lang,
			TokenIdx:  h.TokenIdx,
			StartLine: h.StartLine,
			EndLine:   h.EndLine,
		}
		idx.entries[h.Hash] = append(idx.entries[h.Hash], loc)
	}

	// Store tokens for verification.
	idx.tokenStore[filePath] = tokens
}

// ClonePairsResult holds the output of ClonePairs.
type ClonePairsResult struct {
	Groups         []CloneGroup
	BucketsSkipped int
}

// ClonePairs returns all hash buckets with more than one location,
// indicating potential clone pairs. Only cross-file or distant same-file
// pairs are returned (same-file overlaps within the window are filtered).
//
// Hash buckets exceeding MaxBucketSize are skipped as "too common".
// When languageIsolation is true, only same-language locations are grouped.
func (idx *CloneIndex) ClonePairs(windowSize int, languageIsolation bool) ClonePairsResult {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	var groups []CloneGroup
	bucketsSkipped := 0

	for hash, locs := range idx.entries {
		// Cap: skip overly common hash buckets (boilerplate).
		if idx.MaxBucketSize > 0 && len(locs) > idx.MaxBucketSize {
			bucketsSkipped++
			continue
		}

		if languageIsolation {
			// Split locations by language and process each group separately.
			byLang := make(map[string][]Location)
			for _, loc := range locs {
				byLang[loc.Lang] = append(byLang[loc.Lang], loc)
			}
			for _, langLocs := range byLang {
				if len(langLocs) < 2 {
					continue
				}
				filtered := filterOverlaps(langLocs, windowSize)
				if len(filtered) < 2 {
					continue
				}
				groups = append(groups, CloneGroup{
					Hash:      hash,
					Locations: filtered,
				})
			}
		} else {
			if len(locs) < 2 {
				continue
			}
			filtered := filterOverlaps(locs, windowSize)
			if len(filtered) < 2 {
				continue
			}
			groups = append(groups, CloneGroup{
				Hash:      hash,
				Locations: filtered,
			})
		}
	}

	return ClonePairsResult{
		Groups:         groups,
		BucketsSkipped: bucketsSkipped,
	}
}

// VerifyMatch checks whether two locations actually have identical token
// sequences (not just matching hashes). Returns false on hash collision.
func (idx *CloneIndex) VerifyMatch(a, b Location, windowSize int) bool {
	idx.mu.RLock()
	tokensA := idx.tokenStore[a.FilePath]
	tokensB := idx.tokenStore[b.FilePath]
	idx.mu.RUnlock()

	if tokensA == nil || tokensB == nil {
		return false
	}
	if a.TokenIdx+windowSize > len(tokensA) || b.TokenIdx+windowSize > len(tokensB) {
		return false
	}

	for k := 0; k < windowSize; k++ {
		if tokensA[a.TokenIdx+k].Kind != tokensB[b.TokenIdx+k].Kind {
			return false
		}
	}
	return true
}

// CloneGroup represents a set of locations that share the same token hash.
type CloneGroup struct {
	Hash      uint64
	Locations []Location
}

// filterOverlaps removes locations that overlap within the same file.
// Two locations in the same file overlap if their token indices are within
// windowSize of each other.
func filterOverlaps(locs []Location, windowSize int) []Location {
	// Group by file.
	byFile := make(map[string][]Location)
	for _, loc := range locs {
		byFile[loc.FilePath] = append(byFile[loc.FilePath], loc)
	}

	var result []Location
	for _, fileLocs := range byFile {
		// For same-file locations, keep only non-overlapping ones.
		// Simple greedy: sort by TokenIdx and skip overlapping.
		if len(fileLocs) == 1 {
			result = append(result, fileLocs[0])
			continue
		}

		// Sort by token index.
		sort.Slice(fileLocs, func(i, j int) bool {
			return fileLocs[i].TokenIdx < fileLocs[j].TokenIdx
		})
		result = append(result, fileLocs[0])
		lastIdx := fileLocs[0].TokenIdx
		for i := 1; i < len(fileLocs); i++ {
			if fileLocs[i].TokenIdx-lastIdx >= windowSize {
				result = append(result, fileLocs[i])
				lastIdx = fileLocs[i].TokenIdx
			}
		}
	}

	return result
}

// Size returns the number of unique hashes in the index.
func (idx *CloneIndex) Size() int {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	return len(idx.entries)
}
