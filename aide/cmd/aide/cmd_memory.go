package main

import (
	"fmt"
	"sort"
	"strings"

	"github.com/jmylchreest/aide/aide/pkg/memory"
	"github.com/jmylchreest/aide/aide/pkg/store"
)

// cmdMemoryDispatcher routes memory subcommands.
func cmdMemoryDispatcher(dbPath string, args []string) error {
	if len(args) < 1 {
		printMemoryUsage()
		return nil
	}

	subcmd := args[0]
	subargs := args[1:]

	switch subcmd {
	case "add":
		return cmdAdd(dbPath, subargs)
	case "delete":
		return cmdDelete(dbPath, subargs)
	case "tag":
		return cmdTag(dbPath, subargs)
	case "search":
		return cmdSearch(dbPath, subargs)
	case "select":
		return cmdSelect(dbPath, subargs)
	case "list":
		return cmdList(dbPath, subargs)
	case "sessions":
		return cmdSessions(dbPath, subargs)
	case "export":
		return cmdExport(dbPath, subargs)
	case "clear":
		return cmdClearMemories(dbPath)
	case "reindex":
		return cmdReindex(dbPath)
	case "help", "-h", "--help":
		printMemoryUsage()
		return nil
	default:
		return fmt.Errorf("unknown memory subcommand: %s", subcmd)
	}
}

func printMemoryUsage() {
	fmt.Println(`aide memory - Manage memories

Usage:
  aide memory <subcommand> [arguments]

Subcommands:
  add        Add a memory (writes to bbolt + search index)
  delete     Delete a memory by ID (or "all" to clear)
  tag        Edit tags on a memory (--add=X,Y --remove=A,B)
  search     Full-text search (fuzzy, prefix, substring matching)
  select     Exact substring search (for precise matching)
  list       List all memories
  sessions   List memories grouped by session (for context injection)
  export     Export memories to markdown/json
  clear      Clear all memories
  reindex    Rebuild the bleve search index from bolt data

Options:
  list/select/search/sessions:
    --limit=N              Maximum results (default 10 for search, 50 for list)
    --latest               Return only the most recent memory per tag group
    --full                 Show full content instead of truncated
    --exclude-tags=a,b     Exclude memories with these tags (default: forget)
    --all                  Show all memories including forgotten/excluded

  search:
    --min-score=X          Filter by minimum relevance score

  sessions:
    --project=NAME         Filter to project (required)
    --format=TYPE          Output format: text (default) or json

  export:
    --stdout               Output to stdout (for context injection)
    --format=TYPE          Format: markdown (default) or json
    --output=DIR           Output directory (default: .aide/memory/exports)

Examples:
  aide memory add --category=learning "Found auth middleware at src/auth.ts"
  aide memory search "auth" --full
  aide memory search "auth" --min-score=0.5 --limit=20
  aide memory list --tags=preferences --latest   # Most recent per tag
  aide memory list --all                         # Include forgotten memories
  aide memory list --exclude-tags=forget,partial  # Custom exclusions
  aide memory sessions --project=aide --limit=3  # Last 3 sessions for project
  aide memory export --stdout              # Inject into context
  aide memory list --category=learning
  aide memory delete 1234567890
  aide memory tag 1234567890 --add=forget          # Soft-delete (forget) a memory
  aide memory tag 1234567890 --remove=forget        # Unforget a memory
  aide memory tag 1234567890 --add=personal,private  # Add multiple tags`)
}

func cmdAdd(dbPath string, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: aide memory add [--category=TYPE] [--tags=a,b] [--plan=NAME] CONTENT")
	}

	category := string(memory.CategoryLearning)
	var tags []string
	var content string

	for _, arg := range args {
		switch {
		case strings.HasPrefix(arg, "--category="):
			category = strings.TrimPrefix(arg, "--category=")
		case strings.HasPrefix(arg, "--tags="):
			tags = strings.Split(strings.TrimPrefix(arg, "--tags="), ",")
		default:
			content = arg
		}
	}

	if content == "" {
		return fmt.Errorf("content is required")
	}

	backend, err := NewBackend(dbPath)
	if err != nil {
		return err
	}
	defer backend.Close()

	m, err := backend.AddMemory(content, category, tags)
	if err != nil {
		return fmt.Errorf("failed to add memory: %w", err)
	}

	fmt.Printf("Added memory: %s\n", m.ID)
	return nil
}

func cmdDelete(dbPath string, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: aide memory delete <MEMORY_ID | all>")
	}

	id := args[0]

	// "all" clears all memories
	if id == "all" {
		return cmdClearMemories(dbPath)
	}

	backend, err := NewBackend(dbPath)
	if err != nil {
		return err
	}
	defer backend.Close()

	if err := backend.DeleteMemory(id); err != nil {
		return fmt.Errorf("failed to delete memory: %w", err)
	}

	fmt.Printf("Deleted memory: %s\n", id)
	return nil
}

func cmdTag(dbPath string, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: aide memory tag <MEMORY_ID> --add=tag1,tag2 --remove=tag3,tag4")
	}

	id := args[0]
	addStr := parseFlag(args[1:], "--add=")
	removeStr := parseFlag(args[1:], "--remove=")

	if addStr == "" && removeStr == "" {
		return fmt.Errorf("at least one of --add or --remove is required")
	}

	var addTags, removeTags []string
	if addStr != "" {
		addTags = strings.Split(addStr, ",")
	}
	if removeStr != "" {
		removeTags = strings.Split(removeStr, ",")
	}

	backend, err := NewBackend(dbPath)
	if err != nil {
		return err
	}
	defer backend.Close()

	m, err := backend.UpdateMemoryTags(id, addTags, removeTags)
	if err != nil {
		return fmt.Errorf("failed to update tags: %w", err)
	}

	tagsDisplay := "(none)"
	if len(m.Tags) > 0 {
		tagsDisplay = strings.Join(m.Tags, ", ")
	}
	fmt.Printf("Updated memory %s tags: %s\n", m.ID, tagsDisplay)
	return nil
}

func cmdClearMemories(dbPath string) error {
	backend, err := NewBackend(dbPath)
	if err != nil {
		return err
	}
	defer backend.Close()

	count, err := backend.ClearMemories()
	if err != nil {
		return fmt.Errorf("failed to clear memories: %w", err)
	}

	fmt.Printf("Cleared %d memories\n", count)
	return nil
}

func cmdReindex(dbPath string) error {
	cs, err := store.NewCombinedStore(dbPath)
	if err != nil {
		return fmt.Errorf("failed to open store: %w", err)
	}
	defer cs.Close()

	if err := cs.SyncSearchIndex(); err != nil {
		return fmt.Errorf("reindex failed: %w", err)
	}

	count, err := cs.SearchCount()
	if err != nil {
		return fmt.Errorf("failed to get search count: %w", err)
	}

	fmt.Printf("Search index rebuilt: %d memories indexed\n", count)
	return nil
}

func cmdSearch(dbPath string, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: aide memory search QUERY [--limit=N] [--min-score=X] [--full] [--latest]")
	}

	// Collect all non-flag arguments as the query (supports multi-word without quotes)
	var queryParts []string
	for _, arg := range args {
		if !strings.HasPrefix(arg, "--") {
			queryParts = append(queryParts, arg)
		}
	}
	query := strings.Join(queryParts, " ")
	if query == "" {
		return fmt.Errorf("usage: aide memory search QUERY [--limit=N] [--min-score=X] [--full] [--latest]")
	}

	limit := 10
	minScore := 0.0
	showFull := hasFlag(args[1:], "--full")
	latestOnly := hasFlag(args[1:], "--latest")
	excludeOpts := parseExcludeOpts(args[1:])

	if l := parseFlag(args[1:], "--limit="); l != "" {
		fmt.Sscanf(l, "%d", &limit)
	}
	if s := parseFlag(args[1:], "--min-score="); s != "" {
		fmt.Sscanf(s, "%f", &minScore)
	}

	backend, err := NewBackend(dbPath)
	if err != nil {
		return err
	}
	defer backend.Close()

	results, err := backend.SearchMemoriesWithScore(query, limit, minScore, excludeTagsFromOpts(excludeOpts))
	if err != nil {
		return fmt.Errorf("failed to search: %w", err)
	}

	if len(results) == 0 {
		fmt.Println("No matching memories found")
		return nil
	}

	// Extract memories from results for filtering
	var memories []*memory.Memory
	for _, r := range results {
		if r.Memory != nil {
			memories = append(memories, r.Memory)
		}
	}

	// Keep only the most recent per tag group if --latest is specified
	if latestOnly {
		memories = keepLatestPerTagGroup(memories)
	}

	if showFull {
		for _, m := range memories {
			fmt.Printf("[%s] %s (%s):\n%s\n", m.Category, m.ID, m.CreatedAt.Format("2006-01-02 15:04:05"), m.Content)
			if len(m.Tags) > 0 {
				fmt.Printf("Tags: %s\n", strings.Join(m.Tags, ", "))
			}
			fmt.Println("---")
		}
	} else {
		for _, m := range memories {
			fmt.Printf("[%s] %s: %s\n", padCategory(string(m.Category)), m.ID, truncate(m.Content, 60))
		}
	}
	return nil
}

func cmdSelect(dbPath string, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: aide memory select QUERY [--limit=N] [--latest]")
	}

	// Collect all non-flag arguments as the query (supports multi-word without quotes)
	var queryParts []string
	for _, arg := range args {
		if !strings.HasPrefix(arg, "--") {
			queryParts = append(queryParts, arg)
		}
	}
	query := strings.Join(queryParts, " ")
	if query == "" {
		return fmt.Errorf("usage: aide memory select QUERY [--limit=N] [--latest]")
	}

	limit := 100
	latestOnly := hasFlag(args[1:], "--latest")
	showFull := hasFlag(args[1:], "--full")
	excludeOpts := parseExcludeOpts(args[1:])

	if l := parseFlag(args[1:], "--limit="); l != "" {
		fmt.Sscanf(l, "%d", &limit)
	}

	backend, err := NewBackend(dbPath)
	if err != nil {
		return err
	}
	defer backend.Close()

	memories, err := backend.SearchMemories(query, limit)
	if err == nil && excludeOpts != nil {
		memories = memory.FilterMemories(memories, excludeTagsFromOpts(excludeOpts))
	} else if err == nil {
		memories = memory.FilterMemories(memories, memory.DefaultExcludeTags)
	}
	if err != nil {
		return fmt.Errorf("failed to select: %w", err)
	}

	if len(memories) == 0 {
		fmt.Println("No matching memories found")
		return nil
	}

	// Keep only the most recent per tag group if --latest is specified
	if latestOnly {
		memories = keepLatestPerTagGroup(memories)
	}

	if showFull {
		for _, m := range memories {
			fmt.Printf("[%s] %s (%s):\n%s\n", m.Category, m.ID, m.CreatedAt.Format("2006-01-02 15:04:05"), m.Content)
			if len(m.Tags) > 0 {
				fmt.Printf("Tags: %s\n", strings.Join(m.Tags, ", "))
			}
			fmt.Println("---")
		}
	} else {
		for _, m := range memories {
			fmt.Printf("[%s] %s: %s\n", padCategory(string(m.Category)), m.ID, truncate(m.Content, 60))
		}
	}
	return nil
}

func cmdList(dbPath string, args []string) error {
	var category string
	var tagsFilter []string
	limit := 50
	formatJSON := false
	latestOnly := hasFlag(args, "--latest")
	excludeOpts := parseExcludeOpts(args)

	if c := parseFlag(args, "--category="); c != "" {
		category = c
	}
	if t := parseFlag(args, "--tags="); t != "" {
		tagsFilter = strings.Split(t, ",")
	}
	if l := parseFlag(args, "--limit="); l != "" {
		fmt.Sscanf(l, "%d", &limit)
	}
	if f := parseFlag(args, "--format="); f == "json" {
		formatJSON = true
	}

	backend, err := NewBackend(dbPath)
	if err != nil {
		return err
	}
	defer backend.Close()

	memories, err := backend.ListMemories(category, limit, excludeOpts)
	if err != nil {
		return fmt.Errorf("failed to list: %w", err)
	}

	// Filter by tags if specified
	if len(tagsFilter) > 0 {
		var filtered []*memory.Memory
		for _, m := range memories {
			if hasAllTags(m.Tags, tagsFilter) {
				filtered = append(filtered, m)
			}
		}
		memories = filtered
	}

	// Keep only the most recent per tag group if --latest is specified
	if latestOnly {
		memories = keepLatestPerTagGroup(memories)
	}

	if formatJSON {
		// JSON output for programmatic use
		fmt.Print("[")
		for i, m := range memories {
			if i > 0 {
				fmt.Print(",")
			}
			tagsJSON := "[]"
			if len(m.Tags) > 0 {
				tagsJSON = `["` + strings.Join(m.Tags, `","`) + `"]`
			}
			fmt.Printf(`{"id":"%s","category":"%s","content":"%s","tags":%s,"created_at":"%s"}`,
				m.ID,
				m.Category,
				escapeJSON(m.Content),
				tagsJSON,
				m.CreatedAt.Format("2006-01-02T15:04:05Z07:00"))
		}
		fmt.Println("]")
	} else {
		for _, m := range memories {
			fmt.Printf("[%s] %s: %s\n", padCategory(string(m.Category)), m.ID, truncate(m.Content, 60))
		}
	}
	return nil
}

// SessionGroup represents a session with all its memories
type SessionGroup struct {
	SessionID string           `json:"session_id"`
	Memories  []*memory.Memory `json:"memories"`
	LastAt    string           `json:"last_at"` // Most recent memory timestamp
}

// cmdSessions lists memories grouped by session for a project
func cmdSessions(dbPath string, args []string) error {
	project := parseFlag(args, "--project=")
	if project == "" {
		return fmt.Errorf("usage: aide memory sessions --project=NAME [--limit=N] [--format=json]")
	}

	limit := 3
	formatJSON := parseFlag(args, "--format=") == "json"

	if l := parseFlag(args, "--limit="); l != "" {
		fmt.Sscanf(l, "%d", &limit)
	}

	backend, err := NewBackend(dbPath)
	if err != nil {
		return err
	}
	defer backend.Close()

	excludeOpts := parseExcludeOpts(args)

	// Get all memories for this project (high limit to get all)
	memories, err := backend.ListMemories("", 1000, excludeOpts)
	if err != nil {
		return fmt.Errorf("failed to list: %w", err)
	}

	// Filter to project and group by session
	sessionMap := make(map[string]*SessionGroup)
	projectTag := "project:" + project

	for _, m := range memories {
		// Check if memory belongs to this project
		hasProject := false
		var sessionID string

		for _, tag := range m.Tags {
			if tag == projectTag {
				hasProject = true
			}
			if strings.HasPrefix(tag, "session:") {
				sessionID = strings.TrimPrefix(tag, "session:")
			}
		}

		if !hasProject || sessionID == "" {
			continue
		}

		// Add to session group
		group, ok := sessionMap[sessionID]
		if !ok {
			group = &SessionGroup{
				SessionID: sessionID,
				Memories:  make([]*memory.Memory, 0),
			}
			sessionMap[sessionID] = group
		}
		group.Memories = append(group.Memories, m)

		// Track most recent timestamp
		ts := m.CreatedAt.Format("2006-01-02T15:04:05Z07:00")
		if group.LastAt == "" || ts > group.LastAt {
			group.LastAt = ts
		}
	}

	// Convert to slice and sort by LastAt (most recent first)
	sessions := make([]*SessionGroup, 0, len(sessionMap))
	for _, group := range sessionMap {
		sessions = append(sessions, group)
	}

	// Sort by LastAt descending
	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].LastAt > sessions[j].LastAt
	})

	// Limit to requested number of sessions
	if len(sessions) > limit {
		sessions = sessions[:limit]
	}

	if len(sessions) == 0 {
		fmt.Println("No sessions found for project:", project)
		return nil
	}

	if formatJSON {
		// JSON output for programmatic use
		fmt.Print("[")
		for i, sess := range sessions {
			if i > 0 {
				fmt.Print(",")
			}
			fmt.Printf(`{"session_id":"%s","last_at":"%s","memories":[`, sess.SessionID, sess.LastAt)
			for j, m := range sess.Memories {
				if j > 0 {
					fmt.Print(",")
				}
				tagsJSON := "[]"
				if len(m.Tags) > 0 {
					tagsJSON = `["` + strings.Join(m.Tags, `","`) + `"]`
				}
				fmt.Printf(`{"id":"%s","category":"%s","content":"%s","tags":%s,"created_at":"%s"}`,
					m.ID,
					m.Category,
					escapeJSON(m.Content),
					tagsJSON,
					m.CreatedAt.Format("2006-01-02T15:04:05Z07:00"))
			}
			fmt.Print("]}")
		}
		fmt.Println("]")
	} else {
		// Human-readable output
		for _, sess := range sessions {
			fmt.Printf("=== Session %s (%s) ===\n", sess.SessionID, sess.LastAt[:10])
			for _, m := range sess.Memories {
				fmt.Printf("  [%s] %s: %s\n", padCategory(string(m.Category)), m.ID, truncate(m.Content, 50))
			}
			fmt.Println()
		}
	}

	return nil
}

// parseExcludeOpts parses --exclude-tags and --all flags into SearchOptions.
// Returns nil if defaults should apply (exclude "forget").
// Returns opts with IncludeAll=true if --all is set.
// Returns opts with custom ExcludeTags if --exclude-tags is set.
func parseExcludeOpts(args []string) *memory.SearchOptions {
	includeAll := hasFlag(args, "--all")
	excludeTagsStr := parseFlag(args, "--exclude-tags=")

	if !includeAll && excludeTagsStr == "" {
		return nil // Use defaults
	}

	opts := &memory.SearchOptions{}
	if includeAll {
		opts.IncludeAll = true
	} else if excludeTagsStr != "" {
		opts.ExcludeTags = strings.Split(excludeTagsStr, ",")
	}
	return opts
}

// excludeTagsFromOpts extracts the effective exclude tags from parsed options.
// Returns nil (meaning use DefaultExcludeTags) when opts is nil.
// Returns empty slice when --all is set (no exclusions).
func excludeTagsFromOpts(opts *memory.SearchOptions) []string {
	if opts == nil {
		return nil // Signals "use defaults" to callers
	}
	if opts.IncludeAll {
		return []string{} // Explicitly empty = no exclusions
	}
	return opts.ExcludeTags
}

// hasAllTags checks if memory has all required tags
func hasAllTags(memTags, required []string) bool {
	tagSet := make(map[string]bool)
	for _, t := range memTags {
		tagSet[t] = true
	}
	for _, r := range required {
		if !tagSet[r] {
			return false
		}
	}
	return true
}

// keepLatestPerTagGroup filters memories to keep only the most recent per tag group.
// Tag group is defined as the sorted joined string of all tags.
// If a memory has no tags, its group is based on category.
func keepLatestPerTagGroup(memories []*memory.Memory) []*memory.Memory {
	latest := make(map[string]*memory.Memory)

	for _, m := range memories {
		var key string
		if len(m.Tags) > 0 {
			// Sort tags for consistent grouping
			sortedTags := make([]string, len(m.Tags))
			copy(sortedTags, m.Tags)
			sort.Strings(sortedTags)
			key = strings.Join(sortedTags, ",")
		} else {
			key = "category:" + string(m.Category)
		}

		existing, ok := latest[key]
		if !ok || m.CreatedAt.After(existing.CreatedAt) {
			latest[key] = m
		}
	}

	// Convert map back to slice
	result := make([]*memory.Memory, 0, len(latest))
	for _, m := range latest {
		result = append(result, m)
	}
	return result
}

// padCategory pads category to fixed width inside brackets
// Categories: learning(8), session(7), decision(8), gotcha(6), pattern(7)
func padCategory(cat string) string {
	const width = 8
	if len(cat) >= width {
		return cat
	}
	return cat + strings.Repeat(" ", width-len(cat))
}

// escapeJSON escapes a string for JSON output
func escapeJSON(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	s = strings.ReplaceAll(s, "\n", `\n`)
	s = strings.ReplaceAll(s, "\r", `\r`)
	s = strings.ReplaceAll(s, "\t", `\t`)
	return s
}
