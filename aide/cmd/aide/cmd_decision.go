package main

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/jmylchreest/aide/aide/pkg/memory"
	"github.com/jmylchreest/aide/aide/pkg/store"
)

func cmdDecision(dbPath string, args []string) error {
	if len(args) == 0 || args[0] == "help" || args[0] == "-h" || args[0] == "--help" {
		printDecisionUsage()
		return nil
	}

	backend, err := NewBackend(dbPath)
	if err != nil {
		return err
	}
	defer backend.Close()

	return dispatchSubcmd("decision", args, printDecisionUsage, []subcmd{
		{name: "set", handler: func(a []string) error { return decisionSet(backend, a) }},
		{name: "get", handler: func(a []string) error { return decisionGet(backend, a) }},
		{name: "list", handler: func(a []string) error { return decisionList(backend, a) }},
		{name: "history", handler: func(a []string) error { return decisionHistory(backend, a) }},
		{name: "delete", handler: func(a []string) error { return decisionDelete(backend, a) }},
		{name: "clear", handler: func(a []string) error { return decisionClear(backend) }},
		{name: "adopt", handler: func(a []string) error { return decisionAdopt(backend, dbPath, a) }},
	})
}

func printDecisionUsage() {
	fmt.Println(`aide decision - Manage architectural decisions (append-only)

Usage:
  aide decision <subcommand> [arguments]

Subcommands:
  set        Record a decision (latest wins per topic)
  get        Get the current decision for a topic
  list       List this store's current decisions (--origin to widen)
  history    Show decision history for a topic
  delete     Delete all decisions for a topic
  clear      Clear all decisions
  adopt      Copy a subscribed peer's decision into this store

Options:
  set TOPIC DECISION:
    --rationale=TEXT   Reasoning behind the decision
    --details=TEXT     Extended details or context
    --ref=URL          Reference URL (can be repeated)
    --by=AGENT         Who made the decision

  list:
    --format=json      Output as JSON
    --origin=KIND      Also show decisions in force from elsewhere in the
                       estate: 'parent' (anchor chain), 'peer'
                       (subscriptions), or 'all'. Default is this store
                       only. A topic decided nearer always wins, so these
                       are additive — local > parent > peer.

  history TOPIC:
    --full             Show details in history output

  adopt TOPIC:
    --from=PEER        Subscription to adopt from (required when several
                       peers publish the topic)

Adopt copies a peer's current decision for TOPIC into this store as a
new local decision stamped with adoption provenance — the only way peer
content enters a local store; peer layers are otherwise read-only and
never re-exported. Reads the local subscription cache ('aide sync' to
refresh).

Examples:
  aide decision set auth-strategy "JWT" --rationale="Stateless"
  aide decision set auth-strategy "Session" --rationale="Changed mind"
  aide decision get auth-strategy
  aide decision history auth-strategy --full
  aide decision list --format=json
  aide decision adopt api-style --from=platform-team`)
}

func decisionSet(b *Backend, args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("usage: aide decision set TOPIC DECISION [--rationale=TEXT] [--details=TEXT] [--ref=URL...] [--by=AGENT]")
	}

	topic := args[0]
	decision := args[1]
	rationale := parseFlag(args[2:], "--rationale=")
	details := parseFlag(args[2:], "--details=")
	decidedBy := parseFlag(args[2:], "--by=")

	// Collect all --ref= flags
	var references []string
	for _, arg := range args[2:] {
		if strings.HasPrefix(arg, "--ref=") {
			references = append(references, strings.TrimPrefix(arg, "--ref="))
		}
	}

	d, err := b.SetDecision(topic, decision, rationale, details, decidedBy, references)
	if err != nil {
		return fmt.Errorf("failed to set decision: %w", err)
	}

	fmt.Printf("Set decision: %s = %s\n", d.Topic, d.Decision)
	return nil
}

func decisionGet(b *Backend, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: aide decision get TOPIC")
	}

	topic := args[0]

	d, err := b.GetDecision(topic)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			fmt.Println("No decision found for topic:", topic)
			return nil
		}
		return fmt.Errorf("failed to get decision: %w", err)
	}

	fmt.Printf("%s: %s\n", d.Topic, d.Decision)
	if d.Rationale != "" {
		fmt.Printf("  Rationale: %s\n", d.Rationale)
	}
	if d.Details != "" {
		fmt.Printf("  Details:\n%s\n", indentText(d.Details, "    "))
	}
	if len(d.References) > 0 {
		fmt.Println("  References:")
		for _, ref := range d.References {
			fmt.Printf("    - %s\n", ref)
		}
	}
	if d.DecidedBy != "" {
		fmt.Printf("  Decided by: %s\n", d.DecidedBy)
	}
	fmt.Printf("  At: %s\n", d.CreatedAt.Format(time.RFC3339))
	return nil
}

// parseOriginFlag pulls --origin=parent|peer|all out of args, accepting the
// same space and equals forms as --store. An empty return means local-only,
// the default. Mirrors the provenance axis of decision terminology-axes:
// origin is where a record came from, never scope and never topology.
func parseOriginFlag(args []string) (string, error) {
	sel := ""
	for i, a := range args {
		switch {
		case a == "--origin":
			if i+1 >= len(args) {
				return "", fmt.Errorf("--origin requires a value: parent, peer, or all")
			}
			sel = args[i+1]
		case strings.HasPrefix(a, "--origin="):
			sel = a[len("--origin="):]
		default:
			continue
		}
		break
	}
	switch sel {
	case "", "parent", "peer", "all":
		return sel, nil
	default:
		return "", fmt.Errorf("invalid --origin value %q: want parent, peer, or all", sel)
	}
}

func decisionList(b *Backend, args []string) error {
	origin, err := parseOriginFlag(args)
	if err != nil {
		return err
	}

	decisions, err := b.ListDecisions()
	if err != nil {
		return fmt.Errorf("failed to list decisions: %w", err)
	}

	// Group by topic, show latest.
	latest := make(map[string]*memory.Decision)
	for _, d := range decisions {
		if existing, ok := latest[d.Topic]; !ok || d.CreatedAt.After(existing.CreatedAt) {
			latest[d.Topic] = d
		}
	}

	// Local first, then the estate rings in precedence order, so a topic
	// decided nearer always wins — same layering as session init.
	rows := make([]SessionDecision, 0, len(latest))
	seenTopics := make(map[string]bool, len(latest))
	localTopics := make([]string, 0, len(latest))
	for t := range latest {
		localTopics = append(localTopics, t)
	}
	// Topic-sorted like the cascade rings: map order made this output
	// differ run to run, which is noise in diffs and in agent context.
	sort.Strings(localTopics)
	for _, t := range localTopics {
		d := latest[t]
		seenTopics[d.Topic] = true
		rows = append(rows, SessionDecision{
			Topic:     d.Topic,
			Value:     d.Decision,
			Rationale: d.Rationale,
			CreatedAt: d.CreatedAt.Format(time.RFC3339),
		})
	}
	if origin == "parent" || origin == "all" {
		rows = append(rows, cascadeDecisions(b.dbPath, seenTopics)...)
	}
	if origin == "peer" || origin == "all" {
		rows = append(rows, peerDecisions(b.dbPath, seenTopics)...)
	}

	if wantJSON(args) {
		return printJSON(rows)
	}

	if len(rows) == 0 {
		fmt.Println("No decisions found")
		return nil
	}

	// The ORIGIN column only earns its width when something non-local is present.
	showOrigin := false
	for _, r := range rows {
		if r.OriginKind != "" {
			showOrigin = true
			break
		}
	}

	w := newTabWriter()
	if showOrigin {
		fmt.Fprintln(w, "DATE\tORIGIN\tTOPIC\tDECISION")
	} else {
		fmt.Fprintln(w, "DATE\tTOPIC\tDECISION")
	}
	for _, r := range rows {
		date := r.CreatedAt
		if t, err := time.Parse(time.RFC3339, r.CreatedAt); err == nil {
			date = t.Format("2006-01-02")
		}
		if showOrigin {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", date, originLabel(r), r.Topic, truncate(r.Value, 60))
			continue
		}
		fmt.Fprintf(w, "%s\t%s\t%s\n", date, r.Topic, truncate(r.Value, 60))
	}
	return w.Flush()
}

// originLabel renders a decision's provenance as "local", "parent:<name>" or
// "peer:<name>", falling back to the bare kind when the ancestor has no
// resolvable project identity.
func originLabel(d SessionDecision) string {
	if d.OriginKind == "" {
		return "local"
	}
	if d.OriginName == "" {
		return d.OriginKind
	}
	return d.OriginKind + ":" + d.OriginName
}

func decisionHistory(b *Backend, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: aide decision history TOPIC [--full]")
	}

	topic := args[0]
	showFull := hasFlag(args[1:], "--full")

	decisions, err := b.GetDecisionHistory(topic)
	if err != nil {
		return fmt.Errorf("failed to get decision history: %w", err)
	}

	if len(decisions) == 0 {
		fmt.Println("No decisions found for topic:", topic)
		return nil
	}

	fmt.Printf("Decision history for %s:\n", topic)
	for i, d := range decisions {
		fmt.Printf("  %d. [%s] %s\n", i+1, d.CreatedAt.Format(time.RFC3339), d.Decision)
		if d.Rationale != "" {
			fmt.Printf("     Rationale: %s\n", d.Rationale)
		}
		if showFull && d.Details != "" {
			fmt.Printf("     Details:\n%s\n", indentText(d.Details, "       "))
		}
		if len(d.References) > 0 {
			fmt.Println("     References:")
			for _, ref := range d.References {
				fmt.Printf("       - %s\n", ref)
			}
		}
		if d.DecidedBy != "" {
			fmt.Printf("     By: %s\n", d.DecidedBy)
		}
	}
	return nil
}

// indentText prefixes each line of text with the given indent.
func indentText(text, indent string) string {
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		lines[i] = indent + line
	}
	return strings.Join(lines, "\n")
}

func decisionDelete(b *Backend, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: aide decision delete <TOPIC | all>")
	}

	topic := args[0]

	// "all" clears all decisions
	if topic == "all" {
		return decisionClear(b)
	}

	count, err := b.DeleteDecision(topic)
	if err != nil {
		return fmt.Errorf("failed to delete decisions: %w", err)
	}

	if count == 0 {
		fmt.Println("No decisions found for topic:", topic)
	} else {
		fmt.Printf("Deleted %d decision(s) for topic: %s\n", count, topic)
	}
	return nil
}

func decisionClear(b *Backend) error {
	count, err := b.ClearDecisions()
	if err != nil {
		return fmt.Errorf("failed to clear decisions: %w", err)
	}

	fmt.Printf("Cleared %d decisions\n", count)
	return nil
}
