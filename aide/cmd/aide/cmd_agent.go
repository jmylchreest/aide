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

// State keys used by the agent control surface.
//
// All agent control is state-mediated: the orchestrator writes well-known
// keys; the subagent's PreToolUse signal hook reads them between turns.
// Soft enforcement only — see docs/swarm coordination.
const (
	agentKeyParent   = "parent_session"
	agentKeyNS       = "namespace"
	agentKeyHalt     = "halt"
	agentKeyPaused   = "paused"
	agentKeyDeadline = "deadline"
	agentKeyReason   = "halt_reason"
	agentKeyStatus   = "status"
	agentKeyType     = "type"
	agentKeyStarted  = "startedAt"
)

func cmdAgent(dbPath string, args []string) error {
	if len(args) == 0 || args[0] == "help" || args[0] == "-h" || args[0] == "--help" {
		printAgentUsage()
		return nil
	}

	backend, err := NewBackend(dbPath)
	if err != nil {
		return err
	}
	defer backend.Close()

	return dispatchSubcmd("agent", args, printAgentUsage, []subcmd{
		{name: "register", handler: func(a []string) error { return agentRegister(backend, a) }},
		{name: "identify", handler: func(a []string) error { return agentIdentify(backend, a) }},
		{name: "halt", handler: func(a []string) error { return agentHalt(backend, a) }},
		{name: "pause", handler: func(a []string) error { return agentPause(backend, a) }},
		{name: "resume", handler: func(a []string) error { return agentResume(backend, a) }},
		{name: "deadline", handler: func(a []string) error { return agentDeadline(backend, a) }},
		{name: "list", handler: func(a []string) error { return agentList(backend, a) }},
		{name: "signals", handler: func(a []string) error { return agentSignals(backend, a) }},
	})
}

func printAgentUsage() {
	fmt.Println(`aide agent - Coordination control for spawned subagents

Usage:
  aide agent <subcommand> [arguments]

Subcommands:
  register   Record parent→subagent linkage (used by SubagentStart hook)
  identify   Look up agent + parent given a session/agent id
  halt       Halt an agent at its next tool call
  pause      Pause an agent (block tools except message_send)
  resume     Resume a paused agent
  deadline   Set a soft deadline (warns at 80%, halts at 100%)
  list       List registered agents
  signals    Read pending signals for the caller (used by signal hook)

Options:
  register --agent=ID --parent=SESSION_ID [--namespace=NS]
  identify --agent=ID                              (returns parent + namespace as JSON)
  halt     AGENT_ID [--reason="..."]
  pause    AGENT_ID
  resume   AGENT_ID
  deadline AGENT_ID DURATION                       (e.g. 30m, 2h)
  list     [--parent=SESSION_ID] [--json]
  signals  --agent=ID                              (returns {halt, paused, deadline_remaining})

Examples:
  aide agent register --agent=agent-abc --parent=session-xyz
  aide agent halt agent-abc --reason="repeated rustdoc — see new instinct"
  aide agent list --parent=session-xyz
  aide agent signals --agent=agent-abc`)
}

func agentRegister(b *Backend, args []string) error {
	agentID := parseFlag(args, "--agent=")
	parent := parseFlag(args, "--parent=")
	namespace := parseFlag(args, "--namespace=")
	if agentID == "" || parent == "" {
		return fmt.Errorf("usage: aide agent register --agent=ID --parent=SESSION_ID [--namespace=NS]")
	}
	if namespace == "" {
		namespace = "swarm:" + parent
	}
	if err := b.SetState(agentKeyParent, parent, agentID); err != nil {
		return err
	}
	if err := b.SetState(agentKeyNS, namespace, agentID); err != nil {
		return err
	}
	fmt.Printf("Registered agent %s under parent %s (namespace=%s)\n", agentID, parent, namespace)
	return nil
}

func agentIdentify(b *Backend, args []string) error {
	agentID := parseFlag(args, "--agent=")
	if agentID == "" {
		return fmt.Errorf("usage: aide agent identify --agent=ID")
	}
	out := map[string]string{"agent": agentID}
	for _, k := range []string{agentKeyParent, agentKeyNS, agentKeyStatus, agentKeyType} {
		if st, err := b.GetState(k, agentID); err == nil {
			out[k] = st.Value
		} else if !errors.Is(err, store.ErrNotFound) {
			return err
		}
	}
	return printJSON(out)
}

func agentHalt(b *Backend, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: aide agent halt AGENT_ID [--reason=\"...\"]")
	}
	agentID := args[0]
	reason := parseFlag(args[1:], "--reason=")
	if reason == "" {
		reason = "halted by orchestrator"
	}
	if err := b.SetState(agentKeyHalt, "true", agentID); err != nil {
		return err
	}
	if err := b.SetState(agentKeyReason, reason, agentID); err != nil {
		return err
	}
	fmt.Printf("Halted agent %s: %s\n", agentID, reason)
	return nil
}

func agentPause(b *Backend, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: aide agent pause AGENT_ID")
	}
	agentID := args[0]
	if err := b.SetState(agentKeyPaused, "true", agentID); err != nil {
		return err
	}
	fmt.Printf("Paused agent %s\n", agentID)
	return nil
}

func agentResume(b *Backend, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: aide agent resume AGENT_ID")
	}
	agentID := args[0]
	for _, k := range []string{agentKeyPaused, agentKeyHalt, agentKeyReason} {
		fullKey := fmt.Sprintf("agent:%s:%s", agentID, k)
		if err := b.DeleteState(fullKey); err != nil && !errors.Is(err, store.ErrNotFound) {
			return err
		}
	}
	fmt.Printf("Resumed agent %s\n", agentID)
	return nil
}

func agentDeadline(b *Backend, args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("usage: aide agent deadline AGENT_ID DURATION (e.g. 30m)")
	}
	agentID := args[0]
	dur, err := time.ParseDuration(args[1])
	if err != nil {
		return fmt.Errorf("invalid duration %q: %w", args[1], err)
	}
	deadline := time.Now().Add(dur).UTC().Format(time.RFC3339)
	if err := b.SetState(agentKeyDeadline, deadline, agentID); err != nil {
		return err
	}
	fmt.Printf("Deadline for agent %s: %s (in %s)\n", agentID, deadline, dur)
	return nil
}

// agentList walks all state entries and groups by agent_id (the second
// component of "agent:<id>:<key>"). Optionally filters by --parent=.
func agentList(b *Backend, args []string) error {
	parentFilter := parseFlag(args, "--parent=")

	// Pull every per-agent state entry. Backend.ListState("") returns all
	// entries; we pluck the ones with the agent: prefix.
	all, err := b.ListState("")
	if err != nil {
		return err
	}

	type rec struct {
		AgentID string            `json:"agent"`
		Fields  map[string]string `json:"fields"`
	}
	byAgent := map[string]*rec{}
	for _, st := range all {
		if st.Agent == "" {
			continue
		}
		// State.Key looks like "agent:<id>:<field>"; extract field.
		parts := strings.SplitN(st.Key, ":", 3)
		if len(parts) != 3 || parts[0] != "agent" {
			continue
		}
		field := parts[2]
		r, ok := byAgent[st.Agent]
		if !ok {
			r = &rec{AgentID: st.Agent, Fields: map[string]string{}}
			byAgent[st.Agent] = r
		}
		r.Fields[field] = st.Value
	}

	// Apply parent filter.
	out := make([]*rec, 0, len(byAgent))
	for _, r := range byAgent {
		if parentFilter != "" && r.Fields[agentKeyParent] != parentFilter {
			continue
		}
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].AgentID < out[j].AgentID })

	if wantJSON(args) {
		return printJSON(out)
	}

	if len(out) == 0 {
		fmt.Println("No agents registered")
		return nil
	}
	w := newTabWriter()
	fmt.Fprintln(w, "AGENT\tPARENT\tSTATUS\tHALT\tPAUSED\tDEADLINE")
	for _, r := range out {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
			r.AgentID,
			truncate(r.Fields[agentKeyParent], 16),
			r.Fields[agentKeyStatus],
			r.Fields[agentKeyHalt],
			r.Fields[agentKeyPaused],
			r.Fields[agentKeyDeadline],
		)
	}
	return w.Flush()
}

// agentSignals returns a compact JSON snapshot for the signal hook to
// decide whether to block / inject context. Designed to be called on every
// PreToolUse — keep it cheap.
func agentSignals(b *Backend, args []string) error {
	agentID := parseFlag(args, "--agent=")
	if agentID == "" {
		return fmt.Errorf("usage: aide agent signals --agent=ID")
	}
	sig := map[string]any{"agent": agentID}

	get := func(k string) string {
		st, err := b.GetState(k, agentID)
		if err != nil || st == nil {
			return ""
		}
		return st.Value
	}
	sig["halt"] = isTruthyState(get(agentKeyHalt))
	sig["paused"] = isTruthyState(get(agentKeyPaused))
	if reason := get(agentKeyReason); reason != "" {
		sig["reason"] = reason
	}
	if dlStr := get(agentKeyDeadline); dlStr != "" {
		if dl, err := time.Parse(time.RFC3339, dlStr); err == nil {
			remaining := time.Until(dl)
			sig["deadline"] = dlStr
			sig["deadline_remaining_sec"] = int(remaining.Seconds())
		}
	}

	// Surface unread high-priority messages addressed to this agent so the
	// hook can inject them as additionalContext on the next turn.
	msgs, err := b.ListMessages(agentID)
	if err == nil {
		hi := make([]*memory.Message, 0, 2)
		for _, m := range msgs {
			if strings.EqualFold(m.Priority, "high") && !alreadyAcked(m, agentID) {
				hi = append(hi, m)
			}
		}
		if len(hi) > 0 {
			sig["high_priority_messages"] = hi
		}
	}

	return printJSON(sig)
}

func isTruthyState(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "on", "yes":
		return true
	}
	return false
}

func alreadyAcked(m *memory.Message, agentID string) bool {
	for _, r := range m.ReadBy {
		if r == agentID {
			return true
		}
	}
	return false
}
