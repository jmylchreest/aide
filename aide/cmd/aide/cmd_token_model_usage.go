package main

import (
	"fmt"
	"github.com/jmylchreest/aide/aide/pkg/memory"
	"sort"
	"strings"
)

func formatModelUsage(u *memory.TokenModelUsage, details bool) string {
	if u == nil || u.Version != 1 {
		return "  Host-reported model usage: unavailable from this server.\n"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "  Host-reported model usage (partial coverage): %d captured records; %d conflicting; %d invalid excluded.\n", u.Observations, u.Conflicts, u.Invalid)
	if !details {
		return b.String()
	}
	for _, g := range u.BySource {
		fmt.Fprintf(&b, "    %s / %s", g.Host, g.Source)
		if g.Model != "" {
			fmt.Fprintf(&b, " / %s", g.Model)
		}
		if g.Provider != "" {
			fmt.Fprintf(&b, " (%s)", g.Provider)
		}
		fmt.Fprintf(&b, ": %d records; %d source-timed; %d observation-timed\n", g.Observations, g.SourceTimed, g.ObservedTimed)
		keys := make([]string, 0, len(g.Counters))
		for k := range g.Counters {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			c := g.Counters[k]
			fmt.Fprintf(&b, "      %s: %d tokens; %d/%d records", k, c.Tokens, c.Observations, g.Observations)
			if k == "reported_output_tokens" {
				b.WriteString(" (reasoning inclusion unknown)")
			}
			b.WriteByte('\n')
		}
	}
	b.WriteString("  Captured records only, not complete session usage. Missing counters are unknown, not zero.\n")
	b.WriteString("  Input includes cache; normalized output includes reasoning. Subsets and reported totals must not be added together.\n")
	b.WriteString("  Source groups remain separate. These counters do not establish cost, savings or task quality.\n")
	return b.String()
}
