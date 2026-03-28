package handler

import (
	"context"
	"fmt"
	"html/template"

	"github.com/jmylchreest/aide/aide/pkg/findings"
	"github.com/jmylchreest/aide/aide/pkg/memory"
	"github.com/jmylchreest/aide/aide/pkg/survey"
)

// SearchResult is a single cross-instance search hit.
type SearchResult struct {
	Instance string `json:"instance"`
	Type     string `json:"type"`
	Title    string `json:"title"`
	Detail   string `json:"detail"`
	Link     string `json:"link,omitempty"`
}

// SearchOutput is the response body for APISearch.
type SearchOutput struct {
	Body struct {
		Results []SearchResult `json:"results"`
	}
}

// APISearch fans out a search query across all connected instances.
func (h *Handler) APISearch(ctx context.Context, input *struct {
	Query string `query:"q" required:"true"`
}) (*SearchOutput, error) {
	out := &SearchOutput{}
	out.Body.Results = h.doSearch(input.Query)
	return out, nil
}

func (h *Handler) doSearch(query string) []SearchResult {
	var results []SearchResult
	for _, inst := range h.manager.ConnectedInstances() {
		name := inst.ProjectName()
		if s := inst.Store(); s != nil {
			if mems, err := s.SearchMemories(query, 10); err == nil {
				for _, m := range mems {
					results = append(results, SearchResult{
						Instance: name,
						Type:     "memory",
						Title:    fmt.Sprintf("[%s] %s", m.Category, truncate(m.Content, 80)),
						Detail:   m.Content,
						Link:     fmt.Sprintf("/instances/%s/memories?q=%s", name, template.URLQueryEscaper(m.ID)),
					})
				}
			}
			if decs, err := s.ListDecisions(); err == nil {
				for _, d := range decs {
					if contains(d.Topic, query) || contains(d.Decision, query) {
						results = append(results, SearchResult{
							Instance: name,
							Type:     "decision",
							Title:    d.Topic,
							Detail:   d.Decision,
							Link:     fmt.Sprintf("/instances/%s/decisions?q=%s", name, template.URLQueryEscaper(d.Topic)),
						})
					}
				}
			}
			if tasks, err := s.ListTasks(memory.TaskStatus("")); err == nil {
				for _, t := range tasks {
					if contains(t.Title, query) || contains(t.Description, query) {
						results = append(results, SearchResult{
							Instance: name,
							Type:     "task",
							Title:    fmt.Sprintf("[%s] %s", t.Status, t.Title),
							Detail:   t.Description,
							Link:     fmt.Sprintf("/instances/%s/tasks?q=%s", name, template.URLQueryEscaper(t.ID)),
						})
					}
				}
			}
		}
		if fs := inst.FindingsStore(); fs != nil {
			if ff, err := fs.SearchFindings(query, findings.SearchOptions{Limit: 5}); err == nil {
				for _, f := range ff {
					results = append(results, SearchResult{
						Instance: name,
						Type:     "finding",
						Title:    fmt.Sprintf("[%s] %s", f.Finding.Severity, f.Finding.Title),
						Detail:   fmt.Sprintf("%s:%d — %s", f.Finding.FilePath, f.Finding.Line, f.Finding.Detail),
						Link:     fmt.Sprintf("/instances/%s/findings?q=%s", name, template.URLQueryEscaper(f.Finding.ID)),
					})
				}
			}
		}
		if ss := inst.SurveyStore(); ss != nil {
			if entries, err := ss.SearchEntries(query, survey.SearchOptions{Limit: 5}); err == nil {
				for _, e := range entries {
					results = append(results, SearchResult{
						Instance: name,
						Type:     "survey",
						Title:    fmt.Sprintf("[%s] %s", e.Entry.Kind, e.Entry.Name),
						Detail:   e.Entry.Title,
						Link:     fmt.Sprintf("/instances/%s/survey?q=%s", name, template.URLQueryEscaper(e.Entry.ID)),
					})
				}
			}
		}
	}
	return results
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

func contains(haystack, needle string) bool {
	return len(needle) > 0 && len(haystack) > 0 &&
		containsFold(haystack, needle)
}

func containsFold(s, substr string) bool {
	// Simple case-insensitive contains
	sl := len(s)
	subl := len(substr)
	if subl > sl {
		return false
	}
	for i := 0; i <= sl-subl; i++ {
		if equalFold(s[i:i+subl], substr) {
			return true
		}
	}
	return false
}

func equalFold(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		ca, cb := a[i], b[i]
		if ca >= 'A' && ca <= 'Z' {
			ca += 'a' - 'A'
		}
		if cb >= 'A' && cb <= 'Z' {
			cb += 'a' - 'A'
		}
		if ca != cb {
			return false
		}
	}
	return true
}
