package code

import (
	"math"
	"os"

	"github.com/jmylchreest/aide/aide/pkg/memory"
)

// TextEstimate describes a byte-based estimate, not a tokenizer measurement.
type TextEstimate struct {
	Bytes           int64  `json:"bytes"`
	EstimatedTokens int64  `json:"estimated_tokens"`
	Estimator       string `json:"estimator"`
}

// LegacyTokens preserves the old int32 wire field without overflow.
func (e *TextEstimate) LegacyTokens() int {
	if e == nil || e.EstimatedTokens < 0 || e.EstimatedTokens > math.MaxInt32 {
		return 0
	}
	return int(e.EstimatedTokens)
}

func estimateCurrentText(bytes int64) *TextEstimate {
	// JSON consumers must be able to represent the measured byte count exactly.
	if bytes < 0 || bytes > (1<<53)-1 {
		return nil
	}
	return &TextEstimate{Bytes: bytes, EstimatedTokens: memory.EstimateTextTokens(bytes), Estimator: memory.TextEstimator}
}

type ReadCheckResult struct {
	Indexed          bool          `json:"indexed"`
	Fresh            bool          `json:"fresh"`
	Symbols          int           `json:"symbols"`
	OutlineAvailable bool          `json:"outline_available"`
	EstimatedTokens  int           `json:"estimated_tokens"`
	TextEstimate     *TextEstimate `json:"text_estimate"`
}

// CheckIndexedFile reports index presence and current file size without reading
// file contents. Fresh only compares mtimes; it is not content verification.
func CheckIndexedFile(path string, indexed *FileInfo) *ReadCheckResult {
	result := &ReadCheckResult{}
	if indexed == nil {
		return result
	}
	result.Indexed = true
	result.Symbols = len(indexed.SymbolIDs)
	stat, err := os.Stat(path)
	if err != nil || !stat.Mode().IsRegular() {
		return result
	}
	result.OutlineAvailable = result.Symbols > 0
	result.Fresh = indexed.ModTime.Equal(stat.ModTime())
	result.TextEstimate = estimateCurrentText(stat.Size())
	result.EstimatedTokens = result.TextEstimate.LegacyTokens()
	return result
}
