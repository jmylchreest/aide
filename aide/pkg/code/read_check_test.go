package code

import (
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jmylchreest/aide/aide/pkg/memory"
)

func TestReadCheckCurrentBytes(t *testing.T) {
	for _, ext := range []string{"go", "ts", "txt"} {
		path := filepath.Join(t.TempDir(), "file."+ext)
		if err := os.WriteFile(path, []byte("αβ🙂"), 0600); err != nil {
			t.Fatal(err)
		}
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		indexed := &FileInfo{Tokens: 99999, ModTime: info.ModTime().Add(-time.Hour), SymbolIDs: []string{"x"}}
		got := CheckIndexedFile(path, indexed)
		if !got.Indexed || got.Fresh || !got.OutlineAvailable || got.TextEstimate == nil || got.TextEstimate.Bytes != 8 || got.TextEstimate.EstimatedTokens != memory.EstimateTextTokens(8) || got.TextEstimate.Estimator != memory.TextEstimator || got.EstimatedTokens != 3 {
			t.Fatalf("stale current bytes: %+v", got)
		}
		if err := os.WriteFile(path, nil, 0600); err != nil {
			t.Fatal(err)
		}
		got = CheckIndexedFile(path, indexed)
		if got.TextEstimate == nil || got.TextEstimate.Bytes != 0 || got.TextEstimate.EstimatedTokens != 0 {
			t.Fatalf("empty is known zero: %+v", got)
		}
		if err := os.Remove(path); err != nil {
			t.Fatal(err)
		}
		got = CheckIndexedFile(path, indexed)
		if !got.Indexed || got.Fresh || got.OutlineAvailable || got.TextEstimate != nil || got.EstimatedTokens != 0 {
			t.Fatalf("missing cannot use index tokens: %+v", got)
		}
		if err := os.Mkdir(path, 0700); err != nil {
			t.Fatal(err)
		}
		info, _ = os.Stat(path)
		indexed.ModTime = info.ModTime()
		got = CheckIndexedFile(path, indexed)
		if got.Fresh || got.OutlineAvailable || got.TextEstimate != nil || got.EstimatedTokens != 0 {
			t.Fatalf("directory is not source text: %+v", got)
		}
		if got := CheckIndexedFile(path, nil); got.Indexed || got.TextEstimate != nil {
			t.Fatalf("unindexed: %+v", got)
		}
	}
}

func TestReadCheckEstimateBounds(t *testing.T) {
	for _, n := range []int64{-1, 1 << 53} {
		if estimateCurrentText(n) != nil {
			t.Fatalf("unsafe size %d", n)
		}
	}
	got := estimateCurrentText((1 << 53) - 1)
	if got == nil || got.Bytes != (1<<53)-1 || got.EstimatedTokens != memory.EstimateTextTokens(got.Bytes) {
		t.Fatalf("safe wide value: %+v", got)
	}
	if got.LegacyTokens() != 0 {
		t.Fatal("int32 overflow must be unknown legacy zero")
	}
	if (&TextEstimate{EstimatedTokens: math.MaxInt32}).LegacyTokens() != math.MaxInt32 {
		t.Fatal("int32 boundary lost")
	}
}
