package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/jmylchreest/aide/aide/pkg/memory"
	"github.com/jmylchreest/aide/aide/pkg/store"
)

func cmdDecision(dbPath string, args []string) error {
	if len(args) < 1 {
		printDecisionUsage()
		return nil
	}

	subcmd := args[0]

	if subcmd == "help" || subcmd == "-h" || subcmd == "--help" {
		printDecisionUsage()
		return nil
	}

	backend, err := NewBackend(dbPath)
	if err != nil {
		return err
	}
	defer backend.Close()

	subargs := args[1:]

	switch subcmd {
	case "set":
		return decisionSet(backend, subargs)
	case "get":
		return decisionGet(backend, subargs)
	case "list":
		return decisionList(backend, subargs)
	case "history":
		return decisionHistory(backend, subargs)
	case "delete":
		return decisionDelete(backend, subargs)
	case "clear":
		return decisionClear(backend)
	default:
		return fmt.Errorf("unknown decision subcommand: %s", subcmd)
	}
}

func printDecisionUsage() {
	fmt.Println(`aide decision - Manage architectural decisions (append-only)

Usage:
  aide decision <subcommand> [arguments]

Subcommands:
  set        Record a decision (latest wins per topic)
  get        Get the current decision for a topic
  list       List all current decisions
  history    Show decision history for a topic
  delete     Delete all decisions for a topic
  clear      Clear all decisions

Options:
  set TOPIC DECISION:
    --rationale=TEXT   Reasoning behind the decision
    --details=TEXT     Extended details or context
    --ref=URL          Reference URL (can be repeated)
    --by=AGENT         Who made the decision

  list:
    --format=json      Output as JSON

  history TOPIC:
    --full             Show details in history output

Examples:
  aide decision set auth-strategy "JWT" --rationale="Stateless"
  aide decision set auth-strategy "Session" --rationale="Changed mind"
  aide decision get auth-strategy
  aide decision history auth-strategy --full
  aide decision list --format=json`)
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
		if err == store.ErrNotFound {
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

func decisionList(b *Backend, args []string) error {
	formatJSON := parseFlag(args, "--format=") == "json"

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

	if formatJSON {
		fmt.Print("[")
		first := true
		for _, d := range latest {
			if !first {
				fmt.Print(",")
			}
			first = false
			rationale := ""
			if d.Rationale != "" {
				rationale = escapeJSON(d.Rationale)
			}
			fmt.Printf(`{"topic":"%s","value":"%s","rationale":"%s","created_at":"%s"}`,
				escapeJSON(d.Topic),
				escapeJSON(d.Decision),
				rationale,
				d.CreatedAt.Format("2006-01-02T15:04:05Z07:00"))
		}
		fmt.Println("]")
	} else {
		for _, d := range latest {
			fmt.Printf("[%s] %s: %s\n", d.CreatedAt.Format("2006-01-02"), d.Topic, d.Decision)
		}
	}
	return nil
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
