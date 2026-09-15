package store

import (
	"encoding/json"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/jmylchreest/aide/aide/pkg/memory"
	"github.com/jmylchreest/aide/aide/pkg/observe"
)

type retrievalKey struct{ host, session, actor, epoch string }
type retrievalAccumulator struct {
	window     *memory.RetrievalWindow
	sources    map[string]*memory.RetrievalSource
	targets    map[string]bool
	ranges     map[string]bool
	issues     map[string]bool
	assisted   bool
	boundaryAt time.Time
}
type tokenRetrievals struct {
	report       *memory.TokenRetrievals
	windows      map[retrievalKey]*retrievalAccumulator
	session      string
	since, until time.Time
}

func newTokenRetrievals(session string, since, until time.Time) *tokenRetrievals {
	return &tokenRetrievals{report: &memory.TokenRetrievals{Windows: []*memory.RetrievalWindow{}}, windows: map[retrievalKey]*retrievalAccumulator{}, session: session, since: since, until: until}
}
func (r *tokenRetrievals) selected(e *observe.Event) bool {
	return (r.session == "" || r.session == e.SessionID) && (r.since.IsZero() || !e.Timestamp.Before(r.since)) && (r.until.IsZero() || !e.Timestamp.After(r.until))
}
func retrievalIdentity(e *observe.Event) (retrievalKey, bool) {
	a := e.Attrs
	k := retrievalKey{a["host"], e.SessionID, a["actor_id"], a["context_epoch"]}
	return k, a["context_status"] == "active" && k.epoch != "" && e.SessionID != "unknown" && memory.HasObservationIdentity(e.SessionID, a)
}
func retrievalStatus(e *observe.Event) string {
	if e.Kind != observe.KindToolCall || e.Attrs["accounting_version"] != "1" || e.Attrs["observation_stage"] != "host_result" {
		return ""
	}
	if status := e.Attrs["retrieval_status"]; status != "" {
		return status
	}
	switch e.Name {
	case "Bash":
		return "unclassified_shell"
	case "Read", "Grep", "code_outline", "code_read_symbol":
		return "unverified"
	}
	return ""
}
func retrievalPath(file string) string { return path.Clean(strings.ReplaceAll(file, "\\", "/")) }
func addRetrievalQuantity(q *memory.TokenQuantity, n int64) bool {
	tokens := memory.EstimateTextTokens(n)
	const maxExact = 1<<53 - 1 // JSON consumers must retain exact integer quantities.
	if n > maxExact-q.Bytes || tokens > maxExact-q.EstimatedTokens {
		return false
	}
	q.Bytes += n
	q.EstimatedTokens += tokens
	q.Events++
	return true
}
func (r *tokenRetrievals) add(e *observe.Event) {
	status := retrievalStatus(e)
	if status == "" || !r.selected(e) {
		return
	}
	if e.Error != "" && status != "unclassified_shell" {
		status = "failed"
	}
	key, ok := retrievalIdentity(e)
	if !ok {
		r.report.UnwindowedEvents++
		return
	}
	a := r.windows[key]
	if a == nil {
		if len(r.windows) == 64 {
			r.report.WindowsLimited = true
			return
		}
		w := &memory.RetrievalWindow{Host: key.host, SessionID: key.session, ActorID: key.actor, Epoch: key.epoch, First: e.Timestamp.UTC(), Last: e.Timestamp.UTC(), Boundary: "open_or_unknown", Issues: []string{}, Sources: []*memory.RetrievalSource{}, Steps: []*memory.RetrievalStep{}}
		a = &retrievalAccumulator{window: w, sources: map[string]*memory.RetrievalSource{}, targets: map[string]bool{}, ranges: map[string]bool{}, issues: map[string]bool{}}
		r.windows[key] = a
		r.report.Windows = append(r.report.Windows, w)
	}
	step := a.recordStep(e, status)
	refs := a.sourceReferences(e.Attrs["source_references"])
	a.recordVerification(e, step, refs)
}

func (a *retrievalAccumulator) recordStep(e *observe.Event, status string) *memory.RetrievalStep {
	w := a.window
	w.Events++
	if e.Timestamp.Before(w.First) {
		w.First = e.Timestamp.UTC()
	}
	if e.Timestamp.After(w.Last) {
		w.Last = e.Timestamp.UTC()
	}
	target := e.Attrs["retrieval_target"]
	if target == "" && e.FilePath != "" {
		target = e.FilePath
	}
	step := &memory.RetrievalStep{ID: e.ID, InvocationID: e.Attrs["invocation_id"], At: e.Timestamp.UTC(), Tool: e.Name, Status: status, Target: target}
	if n, ok := memory.MeasuredBytes(e.Attrs, "payload_bytes"); ok {
		step.Text = &memory.TokenQuantity{}
		addRetrievalQuantity(step.Text, n)
		q := &w.Observed
		if status == "unclassified_shell" {
			q = &w.Unattributed
		}
		if !addRetrievalQuantity(q, n) {
			a.issues["quantity_overflow"] = true
		}
	} else {
		w.MissingPayload++
		a.issues["missing_payload"] = true
	}
	if len(w.Steps) < 64 {
		w.Steps = append(w.Steps, step)
	} else {
		w.StepsLimited = true
	}
	switch status {
	case "full_file":
	case "search":
		w.SearchEvents++
	case "failed":
		w.FailedEvents++
	case "unclassified_shell":
		a.issues["unclassified_shell"] = true
	case "pending", "unverified":
		a.issues["unverified_retrieval"] = true
	case "range", "referenced":
	default:
		a.issues["unverified_retrieval"] = true
	}
	if e.Error != "" && status != "failed" {
		w.FailedEvents++
	}
	return step
}

func (a *retrievalAccumulator) sourceReferences(raw string) []*memory.RetrievalSource {
	var refs []*memory.RetrievalSource
	if raw != "" {
		valid := len(raw) <= 65536 && json.Unmarshal([]byte(raw), &refs) == nil && len(refs) > 0 && len(refs) <= 10
		files := map[string]bool{}
		for _, ref := range refs {
			if ref == nil {
				valid = false
				continue
			}
			file := retrievalPath(ref.File)
			if ref.File == "" || len(ref.File) > 4096 || files[file] || len(ref.SHA256) != 64 || strings.Trim(ref.SHA256, "0123456789abcdef") != "" || ref.Bytes < 0 || ref.Bytes > 1<<53-1 {
				valid = false
			}
			ref.File = file
			files[file] = true
		}
		if !valid {
			a.issues["invalid_source_reference"] = true
			refs = nil
		}
	}
	return refs
}

func (a *retrievalAccumulator) recordVerification(e *observe.Event, step *memory.RetrievalStep, refs []*memory.RetrievalSource) {
	w := a.window
	status := step.Status
	verification := e.Attrs["source_verification"]
	renderedRead := w.Host == "opencode" && e.Name == "Read" && e.Attrs["retrieval_method"] == "native_read"
	if status == "range" {
		rangeMatch := verification == "current_range_match" || (renderedRead && verification == "current_rendered_range_match")
		a.recordRange(rangeMatch, refs)
	}
	serverReceipt := verification == "server_receipt" && e.Attrs["retrieval_id"] != "" && (e.Name == "code_outline" || e.Name == "code_read_symbol") && (status == "referenced" || status == "failed")
	fullMatch := verifiedFullRead(status, verification, renderedRead, step, refs)
	if fullMatch {
		w.FullReadEvents++
	}
	trusted := serverReceipt || fullMatch
	if (status == "referenced" && (!serverReceipt || len(refs) == 0)) || (status == "full_file" && !fullMatch) {
		a.issues["unverified_source_version"] = true
	}
	if verification == "server_receipt" && len(refs) > 0 && trusted && (e.Name == "code_outline" || e.Name == "code_read_symbol") {
		a.assisted = true
	}
	if trusted {
		a.recordSources(refs)
	}
	// A verified receipt carries its own file scope. Native ranges and searches
	// still need a baseline for their target; they never mint a new baseline.
	if !trusted || len(refs) == 0 {
		switch {
		case step.Target == "":
			a.issues["unknown_target"] = true
		case len(a.targets) < 256 || a.targets[retrievalPath(step.Target)]:
			a.targets[retrievalPath(step.Target)] = true
		default:
			a.issues["target_limit"] = true
		}
	}
}

// Rendered reads verify complete source content, but numbering and normalized
// line endings make delivered payload bytes distinct from source bytes. Keep
// both quantities; only the raw-source contract requires byte equality.
func verifiedFullRead(status, verification string, renderedRead bool, step *memory.RetrievalStep, refs []*memory.RetrievalSource) bool {
	return status == "full_file" && len(refs) == 1 && step.Text != nil &&
		((verification == "current_file_match" && refs[0].Bytes == step.Text.Bytes) ||
			(renderedRead && verification == "current_rendered_file_match"))
}

func (a *retrievalAccumulator) recordRange(verified bool, refs []*memory.RetrievalSource) {
	if !verified || len(refs) == 0 {
		a.issues["unverified_source_version"] = true
	}
	for _, ref := range refs {
		if len(a.ranges) < 256 || a.ranges[ref.File+"\x00"+ref.SHA256] {
			a.ranges[ref.File+"\x00"+ref.SHA256] = true
		} else {
			a.issues["source_limit"] = true
		}
	}
}

func (a *retrievalAccumulator) recordSources(refs []*memory.RetrievalSource) {
	w := a.window
	for _, ref := range refs {
		key := ref.File + "\x00" + ref.SHA256
		if existing := a.sources[key]; existing != nil {
			if existing.Bytes != ref.Bytes {
				a.issues["conflicting_source_reference"] = true
			}
			continue
		}
		if len(a.sources) == 256 {
			a.issues["source_limit"] = true
			continue
		}
		a.sources[key] = ref
		w.Sources = append(w.Sources, ref)
		if !addRetrievalQuantity(&w.Reference, ref.Bytes) {
			a.issues["quantity_overflow"] = true
		}
	}
}

// Second pass over retained events detects clipped windows and later observed
// boundaries even when those events fall outside the selected date interval.
func (r *tokenRetrievals) context(e *observe.Event) {
	if e.Attrs["observation_stage"] != "host_result" {
		return
	}
	key, ok := retrievalIdentity(e)
	if !ok {
		if retrievalStatus(e) != "" && r.selected(e) {
			for k, a := range r.windows {
				if (key.host == "" || key.host == k.host) && (key.session == "" || key.session == "unknown" || key.session == k.session) && (key.actor == "" || key.actor == k.actor) && (key.epoch == "" || key.epoch == k.epoch) {
					a.issues["context_gap"] = true
				}
			}
		}
		return
	}
	if a := r.windows[key]; a != nil && retrievalStatus(e) != "" && !r.selected(e) {
		a.window.Clipped = true
		a.issues["filtered_window"] = true
	}
	for k, a := range r.windows {
		if k.host != key.host || k.session != key.session || k.actor != key.actor || k.epoch == key.epoch || !e.Timestamp.After(a.window.Last) {
			continue
		}
		if !a.boundaryAt.IsZero() && !e.Timestamp.Before(a.boundaryAt) {
			continue
		}
		a.boundaryAt = e.Timestamp
		a.window.Boundary = "continuity_unknown"
		if e.Attrs["context_continuity"] == "reset" {
			a.window.Boundary = "reset_observed"
		}
	}
}
func (r *tokenRetrievals) result() *memory.TokenRetrievals {
	for _, a := range r.windows {
		w := a.window
		files := map[string]bool{}
		for _, ref := range w.Sources {
			files[ref.File] = true
		}
		for target := range a.targets {
			if !files[target] {
				a.issues["unmatched_target"] = true
			}
		}
		for version := range a.ranges {
			if a.sources[version] == nil {
				a.issues["unverified_source_version"] = true
			}
		}
		if !a.assisted {
			a.issues["no_assisted_reference"] = true
		}
		for issue := range a.issues {
			w.Issues = append(w.Issues, issue)
		}
		sort.Strings(w.Issues)
		if len(w.Issues) == 0 {
			w.Comparison = &memory.TokenChange{BeforeBytes: w.Reference.Bytes, AfterBytes: w.Observed.Bytes, DeltaBytes: w.Reference.Bytes - w.Observed.Bytes, EstimatedTokenDelta: w.Reference.EstimatedTokens - w.Observed.EstimatedTokens, Events: w.Events}
		}
		sort.Slice(w.Sources, func(i, j int) bool {
			if w.Sources[i].File == w.Sources[j].File {
				return w.Sources[i].SHA256 < w.Sources[j].SHA256
			}
			return w.Sources[i].File < w.Sources[j].File
		})
		sort.Slice(w.Steps, func(i, j int) bool {
			if w.Steps[i].At.Equal(w.Steps[j].At) {
				return w.Steps[i].ID < w.Steps[j].ID
			}
			return w.Steps[i].At.Before(w.Steps[j].At)
		})
	}
	sort.Slice(r.report.Windows, func(i, j int) bool { return r.report.Windows[i].Last.After(r.report.Windows[j].Last) })
	return r.report
}
