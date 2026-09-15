package main

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/jmylchreest/aide/aide/pkg/code"
	"github.com/jmylchreest/aide/aide/pkg/store"
)

// Suggestions are request-local indexed candidates, never evidence that a caller
// has read source. Cache file lookups only for this response, including failures.
type codeReadSuggestions struct {
	root    string
	files   map[string][]*code.Symbol
	reads   map[string]CodeReadSymbolInput
	limited bool
}

const maxDiscoveryReads = 10

func newCodeReadSuggestions(root string) *codeReadSuggestions {
	return &codeReadSuggestions{root: root, files: make(map[string][]*code.Symbol), reads: make(map[string]CodeReadSymbolInput)}
}

func (d *codeReadSuggestions) add(sym *code.Symbol) {
	if sym == nil || sym.Name == "" || sym.FilePath == "" || sym.StartLine < 1 {
		return
	}
	in := CodeReadSymbolInput{CheckoutInput: CheckoutInput{CheckoutRoot: d.root}, Symbol: sym.Name, Kind: sym.Kind, File: sym.FilePath, StartLine: sym.StartLine}
	encoded, _ := json.Marshal(in)
	if _, exists := d.reads[string(encoded)]; !exists && len(d.reads) >= maxDiscoveryReads {
		d.limited = true
		return
	}
	d.reads[string(encoded)] = in
}

func (d *codeReadSuggestions) references(cs store.CodeIndexStore, refs []*code.Reference) {
	for _, ref := range refs {
		syms, cached := d.files[ref.FilePath]
		if !cached {
			if len(d.files) >= maxDiscoveryReads {
				d.limited = true
				continue
			}
			var err error
			syms, err = cs.GetFileSymbols(ref.FilePath)
			if err != nil {
				syms = nil
			}
			d.files[ref.FilePath] = syms
		}
		var best *code.Symbol
		for _, sym := range syms {
			if sym == nil || sym.StartLine < 1 || ref.Line < sym.StartLine || ref.Line > sym.EndLine {
				continue
			}
			if best == nil || narrowerSymbol(sym, best) {
				best = sym
			}
		}
		d.add(best)
	}
}

func narrowerSymbol(a, b *code.Symbol) bool {
	if spanA, spanB := a.EndLine-a.StartLine, b.EndLine-b.StartLine; spanA != spanB {
		return spanA < spanB
	}
	if a.StartLine != b.StartLine {
		return a.StartLine < b.StartLine
	}
	if a.Name != b.Name {
		return a.Name < b.Name
	}
	return a.Kind < b.Kind
}

func (d *codeReadSuggestions) text() string {
	if len(d.reads) == 0 && !d.limited {
		return ""
	}
	reads := make([]CodeReadSymbolInput, 0, len(d.reads))
	for _, in := range d.reads {
		reads = append(reads, in)
	}
	sort.Slice(reads, func(i, j int) bool {
		a, b := reads[i], reads[j]
		if a.File != b.File {
			return a.File < b.File
		}
		if a.StartLine != b.StartLine {
			return a.StartLine < b.StartLine
		}
		if a.Symbol != b.Symbol {
			return a.Symbol < b.Symbol
		}
		return a.Kind < b.Kind
	})
	var out strings.Builder
	out.WriteString("\n## Read selected source\n\nIndexed selectors; choose only missing source. No additional search or outline is required. Reuse bodies already available in your current context; reread for changes or missing context. If a location moved, retry file/name without start_line.\n\n")
	for _, in := range reads {
		encoded, _ := json.Marshal(in)
		fmt.Fprintf(&out, "code_read_symbol %s\n", encoded)
	}
	if d.limited {
		out.WriteString("Read selectors limited to 10 definitions and 10 indexed file lookups; other candidate locations remain above.\n")
	}
	return out.String()
}
