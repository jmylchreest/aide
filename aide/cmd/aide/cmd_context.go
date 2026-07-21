package main

import (
	"fmt"
	"sort"
	"strings"

	"github.com/jmylchreest/aide/aide/pkg/config"
	"github.com/jmylchreest/aide/aide/pkg/memory"
	"github.com/jmylchreest/aide/aide/pkg/subscription"
)

// cmdContext groups cross-project context operations. Today that is
// adopt — the deliberate promotion of a peer decision into this project.
func cmdContext(dbPath string, args []string) error {
	return dispatchSubcmd("context", args, printContextUsage, []subcmd{
		{name: "adopt", handler: func(a []string) error { return cmdContextAdopt(dbPath, a) }},
	})
}

// cmdContextAdopt copies a subscribed peer's decision into the local
// store, re-stamped as a local decision with adoption provenance. Reads
// only the local cache — never the network — so adoption is deterministic
// against what `aide sync` last fetched.
func cmdContextAdopt(dbPath string, args []string) error {
	var topic string
	for _, a := range args {
		if len(a) > 0 && a[0] != '-' {
			topic = a
			break
		}
	}
	if topic == "" {
		return fmt.Errorf("usage: aide context adopt TOPIC [--from=PEER]")
	}
	from := parseFlag(args, "--from=")

	subs := config.Get().Subscriptions
	if len(subs) == 0 {
		return fmt.Errorf("no subscriptions configured — nothing to adopt from")
	}

	root := projectRoot(dbPath)
	type hit struct {
		peer string
		d    *memory.Decision
	}
	var hits []hit
	for _, sub := range subs {
		if from != "" && sub.Name != from {
			continue
		}
		shareRoot, err := subscription.CachedRoot(root, sub)
		if err != nil {
			if from != "" {
				return err
			}
			continue
		}
		latest, err := subscription.ReadDecisions(shareRoot)
		if err != nil {
			if from != "" {
				return fmt.Errorf("peer %s unreadable: %w", sub.Name, err)
			}
			continue
		}
		if d, ok := latest[topic]; ok {
			hits = append(hits, hit{peer: sub.Name, d: d})
		}
	}

	switch len(hits) {
	case 0:
		if from != "" {
			return fmt.Errorf("peer %s has no decision for topic %q", from, topic)
		}
		return fmt.Errorf("no subscribed peer has a decision for topic %q (run `aide sync` first?)", topic)
	case 1:
	default:
		peers := make([]string, len(hits))
		for i, h := range hits {
			peers[i] = h.peer
		}
		sort.Strings(peers)
		return fmt.Errorf("topic %q exists in multiple peers (%s) — disambiguate with --from=PEER", topic, strings.Join(peers, ", "))
	}

	src := hits[0]
	decidedBy := fmt.Sprintf("adopted from peer %s", src.peer)
	if src.d.DecidedBy != "" {
		decidedBy = fmt.Sprintf("%s (originally by %s)", decidedBy, src.d.DecidedBy)
	}

	backend, err := NewBackend(dbPath)
	if err != nil {
		return err
	}
	defer backend.Close()
	adopted, err := backend.SetDecision(topic, src.d.Decision, src.d.Rationale, src.d.Details, decidedBy, src.d.References)
	if err != nil {
		return err
	}
	fmt.Printf("Adopted %q from peer %s: %s\n", adopted.Topic, src.peer, firstLine(adopted.Decision))
	return nil
}

func printContextUsage() {
	fmt.Println(`aide context - Cross-project context operations

Usage:
  aide context adopt TOPIC [--from=PEER]

adopt copies a subscribed peer's current decision for TOPIC into this
project's store as a new local decision, stamped with adoption
provenance. This is the only way peer content enters a local store —
peer layers are otherwise read-only and never re-exported. Reads the
local subscription cache; run 'aide sync' to refresh it first.`)
}
