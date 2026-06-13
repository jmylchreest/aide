// Package contextshare implements the .aide/context/ replication surface:
// a deterministic, file-per-record projection of the shareable subset of the
// store (decisions, memories, tombstones), designed to be committed to git.
//
// Layout:
//
//	.aide/context/
//	  decisions/<sanitized-topic>-<hash8>/<created-at-unixnano>.md  # one file per decision version, write-once
//	  memories/<ulid>.md                                            # one file per shareable memory
//	  tombstones/<ulid-or-topic-name>.md                            # deletions, TTL'd
//	  manifest.json                                                 # export watermark (the only non-deterministic byte)
//
// Record files are named by their identity (ULID / version timestamp), so
// re-exports of unchanged content are byte-identical and concurrent
// publishers cannot collide on a path. Deletion happens only via tombstone
// files — there is no snapshot-style stale-file cleanup in this layout.
package contextshare

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/jmylchreest/aide/aide/pkg/memory"
)

const (
	// ManifestVersion is the current context tree format version.
	ManifestVersion = 1

	// ManifestName is the manifest file name at the context tree root.
	ManifestName = "manifest.json"

	// DefaultTombstoneTTL is how long tombstones are retained and honoured.
	// Exports older than this are refused by the import stale guard, which is
	// what makes it safe to garbage-collect tombstones after the same window.
	DefaultTombstoneTTL = 90 * 24 * time.Hour

	decisionsDir  = "decisions"
	memoriesDir   = "memories"
	tombstonesDir = "tombstones"
)

// Manifest is the context tree manifest. The watermark records when the tree
// was last exported and is the only non-deterministic content in the tree.
type Manifest struct {
	Version   int    `json:"version"`
	Watermark string `json:"watermark"` // RFC3339Nano
}

// WriteManifest writes the manifest with the given watermark time.
func WriteManifest(root string, watermark time.Time) error {
	data, err := json.Marshal(Manifest{
		Version:   ManifestVersion,
		Watermark: watermark.UTC().Format(time.RFC3339Nano),
	})
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(root, ManifestName), append(data, '\n'), 0o644)
}

// ReadManifest reads and validates the manifest, returning the parsed
// watermark. A missing file surfaces as fs.ErrNotExist.
func ReadManifest(root string) (*Manifest, time.Time, error) {
	data, err := os.ReadFile(filepath.Join(root, ManifestName))
	if err != nil {
		return nil, time.Time{}, err
	}
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, time.Time{}, fmt.Errorf("malformed manifest: %w", err)
	}
	wm, err := time.Parse(time.RFC3339Nano, m.Watermark)
	if err != nil {
		return nil, time.Time{}, fmt.Errorf("malformed manifest watermark %q: %w", m.Watermark, err)
	}
	return &m, wm, nil
}

// sanitizeRe matches everything that is not filename-safe.
var sanitizeRe = regexp.MustCompile(`[^a-zA-Z0-9_-]+`)

// SanitizeTopic converts a decision topic to a safe, length-capped directory
// or file name. Identity still lives in the record frontmatter, so lossy
// sanitisation here only affects the on-disk grouping.
func SanitizeTopic(s string) string {
	safe := sanitizeRe.ReplaceAllString(s, "-")
	safe = strings.Trim(safe, "-")
	if safe == "" {
		safe = "unnamed"
	}
	if len(safe) > 100 {
		safe = safe[:100]
	}
	return safe
}

// TopicName returns the collision-free directory/file name for a decision
// topic: the sanitized topic plus the first 8 hex chars of sha256(raw topic).
// Sanitisation is lossy ("auth strategy" and "auth-strategy" both sanitise to
// "auth-strategy"), so distinct raw topics would otherwise share a path and
// the write-once guards in export could silently drop records or tombstones.
// Identity still lives in record frontmatter; import never parses paths.
func TopicName(topic string) string {
	sum := sha256.Sum256([]byte(topic))
	return SanitizeTopic(topic) + "-" + hex.EncodeToString(sum[:4])
}

// IsShareableMemory determines if a memory is worth sharing via git.
// Memories with scope:global, project:*, or certain categories are shareable.
// Session-specific memories (session:*) without project scope are excluded.
func IsShareableMemory(m *memory.Memory) bool {
	// Always share gotchas, patterns, and decisions.
	// Other categories (learning, issue, discovery, blocker) require explicit tags.
	switch m.Category {
	case "gotcha", "pattern":
		return true
	case memory.CategoryDecision:
		return true
	case memory.CategoryLearning, memory.CategoryIssue, memory.CategoryDiscovery, memory.CategoryBlocker:
		// Fall through to tag-based checks below
	case memory.CategoryAbandoned:
		// Abandoned memories are not shareable by default
	}

	// Check tags for sharing signals
	for _, tag := range m.Tags {
		if tag == "scope:global" {
			return true
		}
		if strings.HasPrefix(tag, "project:") {
			return true
		}
	}

	return false
}

// DecisionPath returns the path of a decision version record under root.
// The file name is the decision's CreatedAt UnixNano — its store identity —
// which makes version files write-once and lineage a directory listing.
// The topic directory carries a hash suffix so distinct topics that sanitise
// to the same name can never collide on a version file path.
func DecisionPath(root string, d *memory.Decision) string {
	return filepath.Join(root, decisionsDir, TopicName(d.Topic),
		strconv.FormatInt(d.CreatedAt.UnixNano(), 10)+".md")
}

// MemoryPath returns the path of a memory record under root.
func MemoryPath(root, id string) string {
	return filepath.Join(root, memoriesDir, id+".md")
}

// TombstonePath returns the path of a tombstone record under root. Memory
// tombstones use the plain ULID (already collision-free); decision-topic
// tombstones use the same hash-suffixed name as decision directories so
// distinct topics that sanitise alike get distinct tombstone files.
func TombstonePath(root string, t *memory.Tombstone) string {
	name := t.ID
	if t.Kind == memory.TombstoneKindDecisionTopic {
		name = TopicName(t.ID)
	}
	return filepath.Join(root, tombstonesDir, name+".md")
}

// --- Record serialisation ---

// MarshalDecision renders a decision version as deterministic markdown with
// YAML frontmatter. Timestamps are full-precision RFC3339Nano in UTC so the
// record round-trips its store identity exactly.
func MarshalDecision(d *memory.Decision) []byte {
	var b strings.Builder
	b.WriteString("---\n")
	fmt.Fprintf(&b, "topic: %s\n", d.Topic)
	fmt.Fprintf(&b, "decision: %s\n", yamlEscape(d.Decision))
	if d.DecidedBy != "" {
		fmt.Fprintf(&b, "decided_by: %s\n", d.DecidedBy)
	}
	fmt.Fprintf(&b, "created_at: %s\n", d.CreatedAt.UTC().Format(time.RFC3339Nano))
	if len(d.References) > 0 {
		b.WriteString("references:\n")
		for _, ref := range d.References {
			fmt.Fprintf(&b, "  - %s\n", ref)
		}
	}
	b.WriteString("---\n\n")

	fmt.Fprintf(&b, "# %s\n\n", d.Topic)
	fmt.Fprintf(&b, "**Decision:** %s\n", d.Decision)
	if d.Rationale != "" {
		fmt.Fprintf(&b, "\n## Rationale\n\n%s\n", d.Rationale)
	}
	if d.Details != "" {
		fmt.Fprintf(&b, "\n## Details\n\n%s\n", d.Details)
	}
	return []byte(b.String())
}

// ParseDecision parses a decision version record.
func ParseDecision(data []byte) (*memory.Decision, error) {
	front, body, err := splitFrontmatter(data)
	if err != nil {
		return nil, err
	}

	d := &memory.Decision{}
	// listKey tracks which frontmatter list key the current "  - " items
	// belong to, so items under a future list field are not misattributed.
	var listKey string
	for _, line := range front {
		switch {
		case strings.HasPrefix(line, "  - "):
			if listKey == "references" {
				d.References = append(d.References, strings.TrimPrefix(line, "  - "))
			}
		case strings.HasPrefix(line, "references:"):
			listKey = "references"
		case strings.HasPrefix(line, "topic:"):
			listKey = ""
			d.Topic = strings.TrimSpace(strings.TrimPrefix(line, "topic:"))
		case strings.HasPrefix(line, "decision:"):
			listKey = ""
			d.Decision = yamlUnescape(strings.TrimSpace(strings.TrimPrefix(line, "decision:")))
		case strings.HasPrefix(line, "decided_by:"):
			listKey = ""
			d.DecidedBy = strings.TrimSpace(strings.TrimPrefix(line, "decided_by:"))
		case strings.HasPrefix(line, "created_at:"):
			listKey = ""
			t, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(strings.TrimPrefix(line, "created_at:")))
			if err != nil {
				return nil, fmt.Errorf("malformed created_at: %w", err)
			}
			d.CreatedAt = t
		default:
			listKey = ""
		}
	}
	if d.Topic == "" {
		return nil, errors.New("missing topic in frontmatter")
	}
	if d.Decision == "" {
		return nil, errors.New("missing decision in frontmatter")
	}
	if d.CreatedAt.IsZero() {
		return nil, errors.New("missing created_at in frontmatter")
	}

	// Body: rationale/details live under their section headings.
	var section string
	var rationale, details []string
	for _, line := range strings.Split(string(body), "\n") {
		switch {
		case strings.HasPrefix(line, "## Rationale"):
			section = "rationale"
		case strings.HasPrefix(line, "## Details"):
			section = "details"
		case strings.HasPrefix(line, "# ") || strings.HasPrefix(line, "**Decision:**"):
			// Topic heading and decision summary are already in frontmatter.
		default:
			switch section {
			case "rationale":
				rationale = append(rationale, line)
			case "details":
				details = append(details, line)
			}
		}
	}
	d.Rationale = strings.TrimSpace(strings.Join(rationale, "\n"))
	d.Details = strings.TrimSpace(strings.Join(details, "\n"))
	return d, nil
}

// MarshalMemory renders a memory record. The body is the content verbatim,
// so the record is usable as plain LLM context without aide.
func MarshalMemory(m *memory.Memory) []byte {
	var b strings.Builder
	b.WriteString("---\n")
	fmt.Fprintf(&b, "id: %s\n", m.ID)
	fmt.Fprintf(&b, "category: %s\n", m.Category)
	if len(m.Tags) > 0 {
		b.WriteString("tags:\n")
		for _, tag := range m.Tags {
			fmt.Fprintf(&b, "  - %s\n", tag)
		}
	}
	fmt.Fprintf(&b, "created_at: %s\n", m.CreatedAt.UTC().Format(time.RFC3339Nano))
	if !m.UpdatedAt.IsZero() {
		fmt.Fprintf(&b, "updated_at: %s\n", m.UpdatedAt.UTC().Format(time.RFC3339Nano))
	}
	b.WriteString("---\n\n")
	b.WriteString(m.Content)
	b.WriteString("\n")
	return []byte(b.String())
}

// ParseMemory parses a memory record. The body round-trips verbatim: exactly
// one separating newline after the frontmatter and one trailing newline are
// stripped, nothing else is touched.
func ParseMemory(data []byte) (*memory.Memory, error) {
	front, body, err := splitFrontmatter(data)
	if err != nil {
		return nil, err
	}

	m := &memory.Memory{}
	// listKey tracks which frontmatter list key the current "  - " items
	// belong to, so items under a future list field are not misattributed.
	var listKey string
	for _, line := range front {
		switch {
		case strings.HasPrefix(line, "  - "):
			if listKey == "tags" {
				m.Tags = append(m.Tags, strings.TrimPrefix(line, "  - "))
			}
		case strings.HasPrefix(line, "tags:"):
			listKey = "tags"
		case strings.HasPrefix(line, "id:"):
			listKey = ""
			m.ID = strings.TrimSpace(strings.TrimPrefix(line, "id:"))
		case strings.HasPrefix(line, "category:"):
			listKey = ""
			m.Category = memory.Category(strings.TrimSpace(strings.TrimPrefix(line, "category:")))
		case strings.HasPrefix(line, "created_at:"):
			listKey = ""
			t, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(strings.TrimPrefix(line, "created_at:")))
			if err != nil {
				return nil, fmt.Errorf("malformed created_at: %w", err)
			}
			m.CreatedAt = t
		case strings.HasPrefix(line, "updated_at:"):
			listKey = ""
			t, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(strings.TrimPrefix(line, "updated_at:")))
			if err != nil {
				return nil, fmt.Errorf("malformed updated_at: %w", err)
			}
			m.UpdatedAt = t
		default:
			listKey = ""
		}
	}
	if m.ID == "" {
		return nil, errors.New("missing id in frontmatter")
	}
	if m.CreatedAt.IsZero() {
		return nil, errors.New("missing created_at in frontmatter")
	}

	content := string(body)
	content = strings.TrimPrefix(content, "\n")
	content = strings.TrimSuffix(content, "\n")
	m.Content = content
	if m.Content == "" {
		return nil, errors.New("empty memory content")
	}
	return m, nil
}

// MarshalTombstone renders a tombstone record (frontmatter only, no body).
func MarshalTombstone(t *memory.Tombstone) []byte {
	var b strings.Builder
	b.WriteString("---\n")
	fmt.Fprintf(&b, "id: %s\n", t.ID)
	fmt.Fprintf(&b, "kind: %s\n", t.Kind)
	fmt.Fprintf(&b, "deleted_at: %s\n", t.DeletedAt.UTC().Format(time.RFC3339Nano))
	b.WriteString("---\n")
	return []byte(b.String())
}

// ParseTombstone parses a tombstone record.
func ParseTombstone(data []byte) (*memory.Tombstone, error) {
	front, _, err := splitFrontmatter(data)
	if err != nil {
		return nil, err
	}

	t := &memory.Tombstone{}
	for _, line := range front {
		switch {
		case strings.HasPrefix(line, "id:"):
			t.ID = strings.TrimSpace(strings.TrimPrefix(line, "id:"))
		case strings.HasPrefix(line, "kind:"):
			t.Kind = strings.TrimSpace(strings.TrimPrefix(line, "kind:"))
		case strings.HasPrefix(line, "deleted_at:"):
			ts, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(strings.TrimPrefix(line, "deleted_at:")))
			if err != nil {
				return nil, fmt.Errorf("malformed deleted_at: %w", err)
			}
			t.DeletedAt = ts
		}
	}
	if t.ID == "" {
		return nil, errors.New("missing id in frontmatter")
	}
	if t.Kind != memory.TombstoneKindMemory && t.Kind != memory.TombstoneKindDecisionTopic {
		return nil, fmt.Errorf("unknown tombstone kind %q", t.Kind)
	}
	if t.DeletedAt.IsZero() {
		return nil, errors.New("missing deleted_at in frontmatter")
	}
	return t, nil
}

// splitFrontmatter splits a record into frontmatter lines and the raw body
// following the closing delimiter. Only the first two `---` lines are
// significant, so bodies containing `---` lines are untouched.
func splitFrontmatter(data []byte) (front []string, body []byte, err error) {
	s := string(data)
	if !strings.HasPrefix(s, "---\n") {
		return nil, nil, errors.New("missing frontmatter opening delimiter")
	}
	rest := s[4:]
	if idx := strings.Index(rest, "\n---\n"); idx >= 0 {
		return strings.Split(rest[:idx], "\n"), []byte(rest[idx+5:]), nil
	}
	// Frontmatter-only records may end with a bare closing delimiter.
	if strings.HasSuffix(rest, "\n---") {
		return strings.Split(strings.TrimSuffix(rest, "\n---"), "\n"), nil, nil
	}
	return nil, nil, errors.New("missing frontmatter closing delimiter")
}

// yamlEscape wraps a string in quotes if it contains YAML-special characters.
func yamlEscape(s string) string {
	if strings.ContainsAny(s, ":#{}[]|>&*!%@`'\"\\,\n") {
		escaped := strings.ReplaceAll(s, `"`, `\"`)
		return `"` + escaped + `"`
	}
	return s
}

// yamlUnescape removes surrounding quotes from a YAML string value.
func yamlUnescape(s string) string {
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		s = s[1 : len(s)-1]
		s = strings.ReplaceAll(s, `\"`, `"`)
	}
	return s
}
