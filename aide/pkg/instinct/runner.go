package instinct

import (
	"crypto/sha1"
	"encoding/hex"
	"sort"
	"time"

	"github.com/jmylchreest/aide/aide/pkg/observe"
)

const (
	DefaultMaxPerSession   = 3
	DefaultExpiryDays      = 30
	DefaultRejectionTTLDay = 90
)

// Runner orchestrates a set of parsers over one session's worth of events.
type Runner struct {
	parsers []Parser
}

func NewRunner(parsers ...Parser) *Runner {
	return &Runner{parsers: parsers}
}

// RunOpts bundles per-invocation options for Runner.Run.
type RunOpts struct {
	Mode            RunMode
	Classifications map[string]Classification
}

// Run invokes every enabled parser, then dedupes the combined output against
// previously-stored proposals (so an open or recently-rejected proposal of
// the same shape+content isn't re-emitted).
//
// When opts.Mode == RunDeterministic, parsers whose Capabilities().RequiresLLM
// is true are skipped — used by the CLI Stop hook where no LLM is available.
//
// existing is the slice of proposals already in the store with status open
// or rejected; the runner skips any candidate that hashes the same.
func (r *Runner) Run(sessionID string, events, crossSession []*observe.Event, existing []Proposal, opts RunOpts) []Proposal {
	ctx := ParserContext{
		Now:              time.Now(),
		SessionID:        sessionID,
		CrossSessionPast: crossSession,
		Classifications:  opts.Classifications,
	}
	seen := make(map[string]struct{}, len(existing))
	for _, e := range existing {
		seen[hashProposal(e.Shape, e.ProposedInstinct.Content)] = struct{}{}
	}

	out := make([]Proposal, 0)
	perShape := make(map[string]int)
	for _, p := range r.parsers {
		if opts.Mode == RunDeterministic && p.Capabilities().RequiresLLM {
			continue
		}
		cfg := p.DefaultConfig()
		candidates := p.Detect(events, cfg, ctx)
		for _, c := range candidates {
			h := hashProposal(c.Shape, c.ProposedInstinct.Content)
			if _, dupe := seen[h]; dupe {
				continue
			}
			if perShape[c.Shape] >= DefaultMaxPerSession {
				continue
			}
			seen[h] = struct{}{}
			perShape[c.Shape]++

			if c.SessionID == "" {
				c.SessionID = sessionID
			}
			if c.ProposedAt.IsZero() {
				c.ProposedAt = ctx.Now
			}
			if c.Status == "" {
				c.Status = StatusOpen
			}
			if c.ExpiresAt.IsZero() {
				c.ExpiresAt = ctx.Now.AddDate(0, 0, DefaultExpiryDays)
			}
			out = append(out, c)
		}
	}

	sort.SliceStable(out, func(i, j int) bool {
		return out[i].Shape < out[j].Shape
	})
	return out
}

func hashProposal(shape, content string) string {
	h := sha1.Sum([]byte(shape + "\x00" + content))
	return hex.EncodeToString(h[:])
}
