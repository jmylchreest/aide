package main

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/jmylchreest/aide/aide/pkg/blueprint"
	"github.com/mattn/go-runewidth"
)

func TestBlueprintListOrderingAndWidths(t *testing.T) {
	blueprints, err := blueprint.ListEmbedded()
	if err != nil {
		t.Fatal(err)
	}
	// Reverse the source order so the renderer must establish its own order.
	for i, j := 0, len(blueprints)-1; i < j; i, j = i+1, j-1 {
		blueprints[i], blueprints[j] = blueprints[j], blueprints[i]
	}
	for _, width := range []int{20, 40, 60, 80, 100, 160, 0} {
		t.Run(fmt.Sprint(width), func(t *testing.T) {
			var out bytes.Buffer
			if err := renderBlueprintList(&out, blueprints, width, false); err != nil {
				t.Fatal(err)
			}
			limit := min(width, 100)
			if limit <= 0 {
				limit = 80
			}
			for _, line := range strings.Split(out.String(), "\n") {
				if runewidth.StringWidth(line) > limit {
					t.Errorf("line exceeds %d columns: %q", limit, line)
				}
			}
			// Whitespace normalization lets us verify wrapped content without fixing
			// the test to particular line breaks.
			normalized := strings.Join(strings.Fields(out.String()), " ")
			if width >= 40 {
				previous := ""
				count := 0
				for _, line := range strings.Split(out.String(), "\n") {
					if line == "" || strings.HasPrefix(line, " ") || strings.Contains(line, "embedded blueprints") {
						continue
					}
					line = strings.Fields(line)[0]
					if line <= previous {
						t.Errorf("%q follows %q", line, previous)
					}
					previous = line
					count++
				}
				if count != len(blueprints) {
					t.Errorf("got %d entries, want %d", count, len(blueprints))
				}
				for _, bp := range blueprints {
					for _, field := range []string{bp.Name, bp.Version, bp.Description, strings.Join(bp.Includes, ", ")} {
						if !strings.Contains(normalized, strings.Join(strings.Fields(field), " ")) {
							t.Errorf("missing content: %q", field)
						}
					}
				}
			}
		})
	}
}
