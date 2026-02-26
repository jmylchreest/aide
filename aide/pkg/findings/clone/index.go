package clone

import "sync"

// CloneIndex maps rolling hashes to the file locations where they appear.
// Multiple locations for the same hash indicate potential clones.
type CloneIndex struct {
	mu      sync.RWMutex
	entries map[uint64][]Location
}

// Location identifies a hash occurrence in a specific file.
type Location struct {
	FilePath  string
	TokenIdx  int
	StartLine int
	EndLine   int
}

// NewCloneIndex creates an empty clone index.
func NewCloneIndex() *CloneIndex {
	return &CloneIndex{
		entries: make(map[uint64][]Location),
	}
}

// AddFile indexes all rolling hashes for a tokenized file.
func (idx *CloneIndex) AddFile(filePath string, hashes []HashEntry) {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	for _, h := range hashes {
		loc := Location{
			FilePath:  filePath,
			TokenIdx:  h.TokenIdx,
			StartLine: h.StartLine,
			EndLine:   h.EndLine,
		}
		idx.entries[h.Hash] = append(idx.entries[h.Hash], loc)
	}
}

// ClonePairs returns all hash buckets with more than one location,
// indicating potential clone pairs. Only cross-file or distant same-file
// pairs are returned (same-file overlaps within the window are filtered).
func (idx *CloneIndex) ClonePairs(windowSize int) []CloneGroup {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	var groups []CloneGroup

	for hash, locs := range idx.entries {
		if len(locs) < 2 {
			continue
		}

		// Filter out overlapping same-file pairs.
		filtered := filterOverlaps(locs, windowSize)
		if len(filtered) < 2 {
			continue
		}

		groups = append(groups, CloneGroup{
			Hash:      hash,
			Locations: filtered,
		})
	}

	return groups
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
		sortLocationsByIdx(fileLocs)
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

// sortLocationsByIdx sorts locations by token index (insertion sort — typically small slices).
func sortLocationsByIdx(locs []Location) {
	for i := 1; i < len(locs); i++ {
		j := i
		for j > 0 && locs[j].TokenIdx < locs[j-1].TokenIdx {
			locs[j], locs[j-1] = locs[j-1], locs[j]
			j--
		}
	}
}

// Size returns the number of unique hashes in the index.
func (idx *CloneIndex) Size() int {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	return len(idx.entries)
}
