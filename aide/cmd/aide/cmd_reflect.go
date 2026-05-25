package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/jmylchreest/aide/aide/pkg/config"
	"github.com/jmylchreest/aide/aide/pkg/instinct"
	"github.com/jmylchreest/aide/aide/pkg/memory"
	"github.com/jmylchreest/aide/aide/pkg/observe"
	"github.com/jmylchreest/aide/aide/pkg/store"
	"github.com/oklog/ulid/v2"
)

func cmdReflect(dbPath string, args []string) error {
	return dispatchSubcmd("reflect", args, printReflectUsage, []subcmd{
		{name: "run", handler: func(a []string) error { return reflectRun(dbPath, a) }},
		{name: "list", handler: func(a []string) error { return reflectList(dbPath, a) }},
		{name: "candidates", handler: func(a []string) error { return reflectCandidates(dbPath, a) }},
		{name: "test", handler: func(a []string) error { return reflectTest(a) }},
		{name: "current-session", handler: func(a []string) error { return reflectCurrentSession(dbPath) }},
		{name: "accept", handler: func(a []string) error { return reflectAccept(dbPath, a) }},
		{name: "reject", handler: func(a []string) error { return reflectReject(dbPath, a) }},
	})
}

// repetitionConfigFromUser merges user-supplied reflect.repetition.* settings
// over the package defaults. Zero-valued numeric fields keep the default;
// a non-nil IgnoreCommands fully replaces the default list (callers wanting
// to extend should pass the defaults plus their additions).
func repetitionConfigFromUser() instinct.RepetitionConfig {
	def := instinct.DefaultRepetitionConfig()
	cfg := config.Get()
	if cfg == nil {
		return def
	}
	out := def
	if cfg.Reflect.Repetition.MinCount > 0 {
		out.MinCount = cfg.Reflect.Repetition.MinCount
	}
	if cfg.Reflect.Repetition.WindowMinutes > 0 {
		out.WindowMinutes = cfg.Reflect.Repetition.WindowMinutes
	}
	if cfg.Reflect.Repetition.IgnoreCommands != nil {
		out.IgnoreCommands = cfg.Reflect.Repetition.IgnoreCommands
	}
	return out
}

// resolveSessionID returns the explicit --session value when set, otherwise
// the AIDE_SESSION_ID env var, otherwise the session_id of the most recent
// observe event. Empty string only if no observe events exist at all.
func resolveSessionID(backend *Backend, args []string) (string, error) {
	if s := parseFlag(args, "--session="); s != "" {
		return s, nil
	}
	if s := os.Getenv("AIDE_SESSION_ID"); s != "" {
		return s, nil
	}
	events, err := backend.Store().ListObserveEvents(store.ObserveFilter{Limit: 1})
	if err != nil {
		return "", err
	}
	if len(events) == 0 {
		return "", fmt.Errorf("no observe events recorded yet; pass --session=<id> explicitly")
	}
	return events[0].SessionID, nil
}

// reflectCurrentSession prints the resolved current session ID — useful for
// shell pipelines like `aide reflect run --session=$(aide reflect current-session)`
// and as a diagnostic for "what would --session default to right now?".
func reflectCurrentSession(dbPath string) error {
	backend, err := NewBackend(dbPath)
	if err != nil {
		return err
	}
	defer backend.Close()
	sid, err := resolveSessionID(backend, nil)
	if err != nil {
		return err
	}
	fmt.Println(sid)
	return nil
}

func printReflectUsage() {
	fmt.Println(`aide reflect - Extract instinct proposals from observe events

Usage:
  aide reflect <subcommand> [arguments]

Subcommands:
  run              Run the parser catalogue against a session's events and write
                   new proposals. Idempotent: existing proposals (open or
                   rejected) are deduped by (shape + content_hash).
  list             List existing proposals (filter by status with --status=).
  candidates       Emit raw candidate data (e.g. user-prompt events with nearby
                   edit context) suitable for LLM judgement in a skill body.
  test             Dry-run detectors against synthetic events. No persistence.
  current-session  Print the session ID --session would default to.
  accept           Promote a proposal to a memory (category=instinct).
                   Usage: aide reflect accept <id> [--content=OVERRIDE]
                                                   [--supersedes=ID1,ID2,...]
                   Supersession:
                     - Structural (auto): existing instinct memories with the
                       same instinct_key:* tag are marked superseded.
                     - Semantic (manual): pass --supersedes with IDs the skill
                       (or you) identified as conflicting. Each gets the
                       superseded tag + superseded_by:<new_id>. The new memory
                       carries supersedes:<csv> pointing back.
  reject           Mark a proposal rejected (keeps the record for suppression).
                   Usage: aide reflect reject <id> [--reason=TEXT]

Session resolution: when --session is omitted, the command falls back to
$AIDE_SESSION_ID, then to the session_id of the most recent observe event.

Options:
  run:
    --session=ID                 Session ID (optional; see resolution above)
    --limit=N                    Max observe events to consider (default 5000)
    --classifications-json=JSON  Optional JSON array of {id, intent} entries
                                 from an LLM judgement pass. Switches the
                                 runner to LLM mode (runs RequiresLLM parsers).
    --classifications-file=PATH  Same, but read JSON from a file.
  list:
    --status=open                Filter by status (open|accepted|rejected|expired)
    --limit=N                    Max proposals to list
  candidates:
    --session=ID                 Session ID (optional; see resolution above)
  test:
    --pattern=NAME               Pattern to exercise: repetition | convergence
                                 (default: run all). Synthesises a canonical
                                 event sequence in memory, runs the detector,
                                 prints the proposals it would emit. Does NOT
                                 touch the proposal store — pure dry-run.

Environment:
  AIDE_REFLECT=1         Master enable for the reflect Stop hook. Accepts any
                         truthy value: 1, true, on, yes. The CLI ignores it;
                         the hook checks before calling.

Examples:
  aide reflect run --session=abc123
  aide reflect candidates --session=abc123
  aide reflect run --session=abc123 \
    --classifications-json='[{"id":"01ABC","intent":"corrective"}]'
  aide reflect list --status=open
  aide reflect test --pattern=repetition
  aide reflect test --pattern=convergence`)
}

func reflectRun(dbPath string, args []string) error {
	// Gate: env AIDE_REFLECT > .aide/config/aide.json reflect.enabled > false.
	// When disabled, emit a stable no-op JSON line so callers (hooks) can
	// detect the skip without parsing log output.
	if !config.ResolveReflectEnabled(config.Get()) {
		fmt.Println(`{"skipped":"reflect disabled (set AIDE_REFLECT=1 or reflect.enabled=true in .aide/config/aide.json)"}`)
		return nil
	}

	limit := 5000
	if l := parseFlag(args, "--limit="); l != "" {
		n, err := strconv.Atoi(l)
		if err != nil {
			return fmt.Errorf("invalid --limit value %q: %w", l, err)
		}
		limit = n
	}

	classifications, err := loadClassifications(args)
	if err != nil {
		return err
	}

	backend, err := NewBackend(dbPath)
	if err != nil {
		return err
	}
	defer backend.Close()

	sessionID, err := resolveSessionID(backend, args)
	if err != nil {
		return err
	}

	ps := backend.InstinctStore()
	if ps == nil {
		return fmt.Errorf("instinct proposal store unavailable")
	}

	events, err := backend.Store().ListObserveEvents(store.ObserveFilter{
		SessionID: sessionID,
		Limit:     limit,
	})
	if err != nil {
		return fmt.Errorf("list observe events: %w", err)
	}

	existing, err := ps.ListInstinctProposals(store.InstinctFilter{})
	if err != nil {
		return fmt.Errorf("list existing proposals: %w", err)
	}
	relevant := make([]instinct.Proposal, 0, len(existing))
	for _, p := range existing {
		if p.Status == instinct.StatusOpen || p.Status == instinct.StatusRejected {
			relevant = append(relevant, *p)
		}
	}

	mode := instinct.RunDeterministic
	if len(classifications) > 0 {
		mode = instinct.RunWithLLM
	}

	runner := instinct.NewRunner(instinct.Repetition{Config: repetitionConfigFromUser()}, instinct.Convergence{})
	newProps := runner.Run(sessionID, events, nil, relevant, instinct.RunOpts{
		Mode:            mode,
		Classifications: classifications,
	})

	for i := range newProps {
		if err := ps.AddInstinctProposal(&newProps[i]); err != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to add proposal %s: %v\n", newProps[i].ID, err)
		}
	}

	type runResult struct {
		SessionID        string `json:"session_id"`
		EventsConsidered int    `json:"events_considered"`
		ProposalsWritten int    `json:"proposals_written"`
		Shapes           map[string]int `json:"shapes,omitempty"`
	}
	result := runResult{
		SessionID:        sessionID,
		EventsConsidered: len(events),
		ProposalsWritten: len(newProps),
		Shapes:           make(map[string]int),
	}
	for _, p := range newProps {
		result.Shapes[p.Shape]++
	}
	out, _ := json.MarshalIndent(result, "", "  ")
	fmt.Println(string(out))
	return nil
}

// loadClassifications reads classifications either from --classifications-json
// (inline) or --classifications-file (path). Returns nil map when neither flag
// is set so the runner falls back to deterministic mode.
func loadClassifications(args []string) (map[string]instinct.Classification, error) {
	raw := parseFlag(args, "--classifications-json=")
	if raw == "" {
		if path := parseFlag(args, "--classifications-file="); path != "" {
			data, err := os.ReadFile(path)
			if err != nil {
				return nil, fmt.Errorf("read classifications file %q: %w", path, err)
			}
			raw = string(data)
		}
	}
	if raw == "" {
		return nil, nil
	}
	var entries []struct {
		ID         string  `json:"id"`
		Intent     string  `json:"intent"`
		Confidence float32 `json:"confidence,omitempty"`
		Reason     string  `json:"reason,omitempty"`
	}
	if err := json.Unmarshal([]byte(raw), &entries); err != nil {
		return nil, fmt.Errorf("parse classifications JSON: %w", err)
	}
	out := make(map[string]instinct.Classification, len(entries))
	for _, e := range entries {
		if e.ID == "" {
			continue
		}
		out[e.ID] = instinct.Classification{
			Intent:     e.Intent,
			Confidence: e.Confidence,
			Reason:     e.Reason,
		}
	}
	return out, nil
}

// reflectCandidates emits user-prompt observe events that fall within a
// convergence-relevant window (between two edits on the same file). Skill
// bodies read this, judge intent per prompt, then feed the results back via
// reflectRun's --classifications flags.
func reflectCandidates(dbPath string, args []string) error {
	backend, err := NewBackend(dbPath)
	if err != nil {
		return err
	}
	defer backend.Close()

	sessionID, err := resolveSessionID(backend, args)
	if err != nil {
		return err
	}

	events, err := backend.Store().ListObserveEvents(store.ObserveFilter{
		SessionID: sessionID,
		Limit:     5000,
	})
	if err != nil {
		return err
	}

	type candidate struct {
		ID             string `json:"id"`
		Timestamp      string `json:"timestamp"`
		Text           string `json:"text"`
		PrecedingEdit  string `json:"preceding_edit,omitempty"`
		FollowingEdit  string `json:"following_edit,omitempty"`
		FilePath       string `json:"file_path,omitempty"`
	}

	out := struct {
		SessionID string      `json:"session_id"`
		Guidance  string      `json:"guidance"`
		Asks      []string    `json:"asks"`
		Prompts   []candidate `json:"prompts"`
	}{
		SessionID: sessionID,
		Guidance: "For each prompt, judge whether it was correcting the assistant " +
			"(intent=corrective), affirming the assistant's last action (intent=positive), " +
			"or neither (intent=neutral). Consider the surrounding edit context.",
		Asks: []string{"intent"},
	}

	// Walk forward, pairing prompts with the most recent mutation and the
	// next mutation on the same file (mirrors convergence's window logic).
	for i, e := range events {
		if e.Kind != "hook" || (e.Name != "user_prompt" && e.Name != "UserPromptSubmit") {
			continue
		}
		text := ""
		if e.Attrs != nil {
			for _, k := range []string{"text", "prompt", "content"} {
				if v := e.Attrs[k]; v != "" {
					text = v
					break
				}
			}
		}
		if text == "" {
			continue
		}
		c := candidate{
			ID:        e.ID,
			Timestamp: e.Timestamp.Format(time.RFC3339Nano),
			Text:      text,
		}
		// Most recent mutation before this prompt (within 6 events)
		for j := i - 1; j >= 0 && j >= i-6; j-- {
			pe := events[j]
			if pe.Kind == "tool_call" && (pe.Name == "Edit" || pe.Name == "Write" || pe.Name == "NotebookEdit") {
				c.PrecedingEdit = pe.Name + " " + pe.FilePath
				c.FilePath = pe.FilePath
				break
			}
		}
		// Next mutation after this prompt (within 6 events)
		for j := i + 1; j < len(events) && j <= i+6; j++ {
			pe := events[j]
			if pe.Kind == "tool_call" && (pe.Name == "Edit" || pe.Name == "Write" || pe.Name == "NotebookEdit") {
				c.FollowingEdit = pe.Name + " " + pe.FilePath
				break
			}
		}
		// Only emit if there's at least a surrounding edit — otherwise the
		// prompt isn't relevant to convergence detection.
		if c.PrecedingEdit == "" && c.FollowingEdit == "" {
			continue
		}
		out.Prompts = append(out.Prompts, c)
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(&out)
}

// reflectTest exercises detectors against in-memory synthetic event sequences.
// Pure dry-run: no DB access, no persistence. Useful for manually verifying a
// detector fires (or doesn't) against a known input, and as living
// documentation of what canonical triggers look like.
func reflectTest(args []string) error {
	pattern := parseFlag(args, "--pattern=")
	patterns := []string{"repetition", "convergence"}
	if pattern != "" {
		patterns = []string{pattern}
	}

	type fixture struct {
		Name     string
		Events   []*observe.Event
		Parser   instinct.Parser
		WithLLM  bool
		Classify map[string]instinct.Classification
	}

	all := map[string]fixture{
		"repetition": {
			Name:   "repetition (5 × `cat`)",
			Parser: instinct.Repetition{},
			Events: synthRepetition(),
		},
		"convergence": {
			Name:   "convergence (Edit → corrective prompt → Edit on same file)",
			Parser: instinct.Convergence{},
			Events: synthConvergence(),
		},
	}

	for _, p := range patterns {
		f, ok := all[p]
		if !ok {
			return fmt.Errorf("unknown pattern %q (try: repetition, convergence)", p)
		}

		fmt.Printf("=== %s ===\n", f.Name)
		fmt.Printf("Synthesised %d events for session test-%s\n\n", len(f.Events), p)

		runner := instinct.NewRunner(f.Parser)
		props := runner.Run("test-"+p, f.Events, nil, nil, instinct.RunOpts{
			Mode: instinct.RunDeterministic,
		})

		if len(props) == 0 {
			fmt.Println("  (no proposals)")
		} else {
			for i, pr := range props {
				fmt.Printf("  [%d] shape=%s\n", i+1, pr.Shape)
				fmt.Printf("      summary: %s\n", pr.Summary)
				fmt.Printf("      content: %s\n", pr.ProposedInstinct.Content)
				fmt.Printf("      evidence: %d events\n", len(pr.Evidence.ObserveEventIDs))
			}
		}
		fmt.Println()
	}
	return nil
}

// synthRepetition emits 5 Bash `cat` calls at 30-second intervals — a stable
// minimum input that exceeds Repetition's default MinCount=4.
func synthRepetition() []*observe.Event {
	now := time.Now()
	out := make([]*observe.Event, 0, 5)
	for i := 0; i < 5; i++ {
		out = append(out, &observe.Event{
			ID:        ulid.Make().String(),
			Timestamp: now.Add(time.Duration(i) * 30 * time.Second),
			Kind:      "tool_call",
			Name:      "Bash",
			Category:  "execute",
			SessionID: "test-repetition",
			Attrs:     map[string]string{"command": fmt.Sprintf("cat file%d.txt", i)},
		})
	}
	return out
}

// synthConvergence emits a minimal trigger sequence: Edit on a file →
// user-prompt event carrying a corrective marker → Edit on the same file.
func synthConvergence() []*observe.Event {
	now := time.Now()
	file := "src/example.ts"
	return []*observe.Event{
		{
			ID:        ulid.Make().String(),
			Timestamp: now,
			Kind:      "tool_call",
			Name:      "Edit",
			Category:  "modify",
			Subtype:   "file",
			FilePath:  file,
			SessionID: "test-convergence",
		},
		{
			ID:        ulid.Make().String(),
			Timestamp: now.Add(10 * time.Second),
			Kind:      "hook",
			Name:      "user_prompt",
			Category:  "input",
			SessionID: "test-convergence",
			Attrs:     map[string]string{"text": "no, don't add async — keep it sync"},
		},
		{
			ID:        ulid.Make().String(),
			Timestamp: now.Add(20 * time.Second),
			Kind:      "tool_call",
			Name:      "Edit",
			Category:  "modify",
			Subtype:   "file",
			FilePath:  file,
			SessionID: "test-convergence",
		},
	}
}

// reflectAccept promotes a proposal to a memory (category=instinct) and marks
// the proposal accepted. ID is taken from the first non-flag positional arg.
func reflectAccept(dbPath string, args []string) error {
	id := firstPositional(args)
	if id == "" {
		return fmt.Errorf("proposal id required: aide reflect accept <id> [--content=...] [--supersedes=ID1,ID2]")
	}
	contentOverride := parseFlag(args, "--content=")
	manualSupersedes := splitCSV(parseFlag(args, "--supersedes="))

	backend, err := NewBackend(dbPath)
	if err != nil {
		return err
	}
	defer backend.Close()

	ps := backend.InstinctStore()
	if ps == nil {
		return fmt.Errorf("instinct proposal store unavailable")
	}

	prop, err := ps.GetInstinctProposal(id)
	if err != nil {
		return err
	}
	if prop == nil {
		return fmt.Errorf("proposal %s not found", id)
	}

	content := prop.ProposedInstinct.Content
	if contentOverride != "" {
		content = contentOverride
	}

	// Supersession has two sources:
	//   1. STRUCTURAL — instinct memories that share the same instinct_key
	//      tag (cheap, deterministic dedupe of old-instinct → new-instinct
	//      of the same shape/subject). Catches the case where a session
	//      re-proposes the same pattern after detector tweaks.
	//   2. SEMANTIC — IDs passed via --supersedes=<csv>. The reflect skill
	//      is expected to populate this after asking the LLM to find
	//      memories that conflict with the new instinct's content (e.g. a
	//      manual "always run rustdoc" memory being superseded by a new
	//      "rustdoc runs repeatedly — cache" instinct). Works for arbitrary
	//      memories, not just instinct-tagged ones.
	// Both sources are unioned. Each superseded record gets tagged
	// `superseded` + `superseded_by:<new_id>`. The new memory carries a
	// single `supersedes:<csv>` tag listing all predecessors.
	structuralSet, err := findSupersededInstincts(backend.Store(), prop.ProposedInstinct.Tags)
	if err != nil {
		return fmt.Errorf("search prior instincts: %w", err)
	}
	superseded, err := unionSuperseded(backend.Store(), structuralSet, manualSupersedes)
	if err != nil {
		return fmt.Errorf("resolve manual supersedes: %w", err)
	}

	newTags := append([]string(nil), prop.ProposedInstinct.Tags...)
	if len(superseded) > 0 {
		ids := make([]string, 0, len(superseded))
		for _, m := range superseded {
			ids = append(ids, m.ID)
		}
		newTags = append(newTags, "supersedes:"+strings.Join(ids, ","))
	}

	mem := &memory.Memory{
		Category: memory.Category(prop.ProposedInstinct.Category),
		Content:  content,
		Tags:     newTags,
		Priority: prop.ProposedInstinct.Priority,
	}
	if err := backend.Store().AddMemory(mem); err != nil {
		return fmt.Errorf("create memory: %w", err)
	}

	for _, old := range superseded {
		old.Tags = appendUnique(old.Tags, "superseded", "superseded_by:"+mem.ID)
		if err := backend.Store().UpdateMemory(old); err != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to mark memory %s as superseded: %v\n", old.ID, err)
		}
	}

	if _, err := ps.UpdateInstinctProposalStatus(id, instinct.StatusAccepted, "", mem.ID); err != nil {
		return fmt.Errorf("update proposal status: %w", err)
	}

	result := map[string]any{
		"proposal_id": id,
		"memory_id":   mem.ID,
		"status":      string(instinct.StatusAccepted),
	}
	if len(superseded) > 0 {
		ids := make([]string, 0, len(superseded))
		for _, m := range superseded {
			ids = append(ids, m.ID)
		}
		result["superseded"] = ids
	}
	out, _ := json.MarshalIndent(result, "", "  ")
	fmt.Println(string(out))
	return nil
}

// findSupersededInstincts returns memories that share an instinct_key tag
// with newTags. Used by accept to mark prior instincts superseded.
func findSupersededInstincts(st store.Store, newTags []string) ([]*memory.Memory, error) {
	key := ""
	for _, t := range newTags {
		if strings.HasPrefix(t, "instinct_key:") {
			key = t
			break
		}
	}
	if key == "" {
		return nil, nil
	}
	// Search broadly across all memories; filter by key + category client-side.
	// IncludeAll bypasses the default "forget" exclusion so we also see things
	// already marked superseded (we never want to re-supersede those).
	mems, err := st.ListMemories(memory.SearchOptions{
		Category:   memory.CategoryInstinct,
		Tags:       []string{key},
		Limit:      100,
		IncludeAll: true,
	})
	if err != nil {
		return nil, err
	}
	out := make([]*memory.Memory, 0, len(mems))
	for _, m := range mems {
		if hasTag(m.Tags, "superseded") {
			continue
		}
		out = append(out, m)
	}
	return out, nil
}

// unionSuperseded combines structural (instinct_key tag) matches with manual
// IDs from --supersedes, dedupes, and fetches any manual records not already
// in the structural set. Manual IDs that don't resolve are returned as
// (best-effort) — missing IDs are silently dropped after a stderr warning.
func unionSuperseded(st store.Store, structural []*memory.Memory, manualIDs []string) ([]*memory.Memory, error) {
	seen := make(map[string]struct{}, len(structural)+len(manualIDs))
	out := make([]*memory.Memory, 0, len(structural)+len(manualIDs))
	for _, m := range structural {
		seen[m.ID] = struct{}{}
		out = append(out, m)
	}
	for _, id := range manualIDs {
		if _, dup := seen[id]; dup {
			continue
		}
		m, err := st.GetMemory(id)
		if err != nil || m == nil {
			fmt.Fprintf(os.Stderr, "warning: --supersedes id %q not found\n", id)
			continue
		}
		seen[id] = struct{}{}
		out = append(out, m)
	}
	return out, nil
}

// splitCSV parses a comma-separated list, trimming whitespace and skipping
// empties. Returns nil for empty input.
func splitCSV(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}

func hasTag(tags []string, want string) bool {
	for _, t := range tags {
		if t == want {
			return true
		}
	}
	return false
}

func appendUnique(tags []string, more ...string) []string {
	seen := make(map[string]struct{}, len(tags)+len(more))
	for _, t := range tags {
		seen[t] = struct{}{}
	}
	for _, t := range more {
		if _, ok := seen[t]; !ok {
			tags = append(tags, t)
			seen[t] = struct{}{}
		}
	}
	return tags
}

// reflectReject marks a proposal rejected (with optional reason).
func reflectReject(dbPath string, args []string) error {
	id := firstPositional(args)
	if id == "" {
		return fmt.Errorf("proposal id required: aide reflect reject <id> [--reason=...]")
	}
	reason := parseFlag(args, "--reason=")

	backend, err := NewBackend(dbPath)
	if err != nil {
		return err
	}
	defer backend.Close()

	ps := backend.InstinctStore()
	if ps == nil {
		return fmt.Errorf("instinct proposal store unavailable")
	}

	p, err := ps.UpdateInstinctProposalStatus(id, instinct.StatusRejected, reason, "")
	if err != nil {
		return err
	}
	if p == nil {
		return fmt.Errorf("proposal %s not found", id)
	}

	out, _ := json.MarshalIndent(map[string]any{
		"proposal_id":     id,
		"status":          string(p.Status),
		"rejection_count": p.RejectionCount,
	}, "", "  ")
	fmt.Println(string(out))
	return nil
}

// firstPositional returns the first arg that isn't a --flag. Used by accept
// and reject which take an ID as a bare positional.
func firstPositional(args []string) string {
	for _, a := range args {
		if !strings.HasPrefix(a, "--") {
			return a
		}
	}
	return ""
}

func reflectList(dbPath string, args []string) error {
	statusFilter := parseFlag(args, "--status=")
	limit := 50
	if l := parseFlag(args, "--limit="); l != "" {
		if n, err := strconv.Atoi(l); err == nil {
			limit = n
		}
	}

	backend, err := NewBackend(dbPath)
	if err != nil {
		return err
	}
	defer backend.Close()

	ps := backend.InstinctStore()
	if ps == nil {
		return fmt.Errorf("instinct proposal store unavailable")
	}

	props, err := ps.ListInstinctProposals(store.InstinctFilter{
		Status: instinct.Status(strings.TrimSpace(statusFilter)),
		Limit:  limit,
	})
	if err != nil {
		return err
	}
	out, _ := json.MarshalIndent(props, "", "  ")
	fmt.Println(string(out))
	return nil
}
