package clone

// RabinKarp implements a rolling hash for token sequences.
//
// The hash function uses the polynomial rolling hash:
//   h(s[0..k-1]) = s[0]*base^(k-1) + s[1]*base^(k-2) + ... + s[k-1]
// All arithmetic is performed modulo a large prime to prevent overflow.

const (
	// hashBase is the base for the polynomial hash.
	// Chosen to be a prime larger than the expected token vocabulary.
	hashBase uint64 = 257

	// hashMod is the modulus for the hash. A large prime to reduce collisions.
	hashMod uint64 = 1_000_000_007
)

// RollingHash computes Rabin-Karp rolling hashes over a token sequence.
type RollingHash struct {
	window  int    // Size of the sliding window.
	basePow uint64 // base^(window-1) mod hashMod, precomputed for slide.
}

// NewRollingHash creates a rolling hash for the given window size.
func NewRollingHash(windowSize int) *RollingHash {
	// Precompute base^(window-1) mod hashMod.
	pow := uint64(1)
	for i := 0; i < windowSize-1; i++ {
		pow = (pow * hashBase) % hashMod
	}
	return &RollingHash{
		window:  windowSize,
		basePow: pow,
	}
}

// HashToken computes a numeric value for a token kind string.
// Uses a simple DJB2-like hash on the kind string.
func HashToken(kind string) uint64 {
	h := uint64(5381)
	for i := 0; i < len(kind); i++ {
		h = ((h << 5) + h + uint64(kind[i])) % hashMod
	}
	return h
}

// ComputeHashes returns all rolling hashes and their starting token indices
// for a token sequence using the configured window size.
func (rh *RollingHash) ComputeHashes(tokens []Token) []HashEntry {
	n := len(tokens)
	if n < rh.window {
		return nil
	}

	results := make([]HashEntry, 0, n-rh.window+1)

	// Compute initial hash for tokens[0..window-1].
	var h uint64
	for i := 0; i < rh.window; i++ {
		h = (h*hashBase + HashToken(tokens[i].Kind)) % hashMod
	}

	results = append(results, HashEntry{
		Hash:      h,
		TokenIdx:  0,
		StartLine: tokens[0].Line,
		EndLine:   tokens[rh.window-1].Line,
	})

	// Slide the window.
	for i := 1; i <= n-rh.window; i++ {
		// Remove contribution of tokens[i-1], add tokens[i+window-1].
		old := HashToken(tokens[i-1].Kind)
		next := HashToken(tokens[i+rh.window-1].Kind)

		h = (h + hashMod - (old*rh.basePow)%hashMod) % hashMod
		h = (h*hashBase + next) % hashMod

		results = append(results, HashEntry{
			Hash:      h,
			TokenIdx:  i,
			StartLine: tokens[i].Line,
			EndLine:   tokens[i+rh.window-1].Line,
		})
	}

	return results
}

// HashEntry stores a hash and its position in the token sequence.
type HashEntry struct {
	Hash      uint64
	TokenIdx  int // Index into the token sequence.
	StartLine int // Source line of the first token in the window.
	EndLine   int // Source line of the last token in the window.
}
