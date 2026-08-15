package code

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jmylchreest/aide/aide/pkg/grammar"
)

// All fixtures in this file are small, deterministic source snippets
// embedded inline. The parser loader is built with auto-download disabled,
// so only compiled-in grammars are used (no network) and no real source
// tree is parsed.

const benchGoSource = `package benchpkg

import (
	"fmt"
	"strings"
)

// User represents a synthetic user record.
type User struct {
	ID   int
	Name string
}

// Greeter produces greetings for users.
type Greeter struct {
	prefix string
}

// NewGreeter builds a Greeter with the given prefix.
func NewGreeter(prefix string) *Greeter {
	if prefix == "" {
		prefix = "hello"
	}
	return &Greeter{prefix: prefix}
}

// Greet returns a greeting for the user.
func (g *Greeter) Greet(u *User) string {
	if u == nil {
		return g.prefix + ", stranger"
	}
	return fmt.Sprintf("%s, %s", g.prefix, u.Name)
}

// ValidName reports whether the name is non-empty and trimmed.
func ValidName(name string) bool {
	name = strings.TrimSpace(name)
	return name != ""
}

// SumAll adds every value in the slice.
func SumAll(values []int) int {
	total := 0
	for _, v := range values {
		if v < 0 {
			continue
		}
		total += v
	}
	return total
}
`

const benchTSSource = `export interface PaymentRequest {
  id: string;
  amount: number;
  currency: string;
}

export class PaymentError extends Error {
  constructor(message: string, public readonly code: number) {
    super(message);
  }
}

function validateAmount(amount: number): boolean {
  if (!Number.isFinite(amount)) {
    return false;
  }
  return amount > 0;
}

export async function processPayment(req: PaymentRequest): Promise<string> {
  if (!validateAmount(req.amount)) {
    throw new PaymentError("invalid amount", 400);
  }
  const normalized = req.currency.toUpperCase();
  const receipt = req.id + ":" + normalized + ":" + req.amount;
  return receipt;
}

export function summarise(receipts: string[]): number {
  let count = 0;
  for (const r of receipts) {
    if (r.length > 0) {
      count++;
    }
  }
  return count;
}
`

var (
	benchTokenSink  int
	benchSymbolSink []*Symbol
)

// BenchmarkTokenizeFile measures the calibrated token-counting cost for a
// representative Go and TypeScript source file.
func BenchmarkTokenizeFile(b *testing.B) {
	b.Run("Go", func(b *testing.B) {
		n := len(benchGoSource)
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			benchTokenSink = EstimateTokens("pkg/bench/bench.go", n)
		}
	})

	b.Run("TypeScript", func(b *testing.B) {
		n := len(benchTSSource)
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			benchTokenSink = EstimateTokens("pkg/bench/bench.ts", n)
		}
	})
}

// BenchmarkParseFile measures the per-file parse path (read + language
// detect + tree-sitter tag extraction) through Parser.ParseFile.
func BenchmarkParseFile(b *testing.B) {
	loader := grammar.NewCompositeLoader(grammar.WithAutoDownload(false))
	p := NewParser(loader)
	b.Cleanup(p.Close)

	dir := b.TempDir()
	goPath := filepath.Join(dir, "bench.go")
	tsPath := filepath.Join(dir, "bench.ts")
	if err := os.WriteFile(goPath, []byte(benchGoSource), 0o644); err != nil {
		b.Fatalf("WriteFile go: %v", err)
	}
	if err := os.WriteFile(tsPath, []byte(benchTSSource), 0o644); err != nil {
		b.Fatalf("WriteFile ts: %v", err)
	}

	// Sanity-check: both grammars are compiled-in; parsing must find symbols.
	if syms, err := p.ParseFile(goPath); err != nil || len(syms) == 0 {
		b.Fatalf("fixture check failed for go: syms=%d err=%v", len(syms), err)
	}
	if syms, err := p.ParseFile(tsPath); err != nil || len(syms) == 0 {
		b.Fatalf("fixture check failed for typescript: syms=%d err=%v", len(syms), err)
	}

	b.Run("Go", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			syms, err := p.ParseFile(goPath)
			if err != nil {
				b.Fatalf("ParseFile(go): %v", err)
			}
			benchSymbolSink = syms
		}
	})

	b.Run("TypeScript", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			syms, err := p.ParseFile(tsPath)
			if err != nil {
				b.Fatalf("ParseFile(ts): %v", err)
			}
			benchSymbolSink = syms
		}
	})
}
