package handler

import (
	"context"

	"github.com/danielgtaylor/huma/v2"
	"github.com/jmylchreest/aide/aide/pkg/survey"
	"github.com/jmylchreest/aide/aide/pkg/surveyrun"
)

// SurveyItem is the JSON representation of a survey entry. Metadata carries
// the analyzer-specific payload the richer views need (module size, hub,
// cohesion, members; churn counts; the run_commit freshness stamp).
type SurveyItem struct {
	ID       string            `json:"id"`
	Analyzer string            `json:"analyzer"`
	Kind     string            `json:"kind"`
	Name     string            `json:"name"`
	FilePath string            `json:"file_path"`
	Title    string            `json:"title"`
	Detail   string            `json:"detail,omitempty"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

// ListSurveyOutput is the response body for APIListSurvey.
type ListSurveyOutput struct {
	Body struct {
		Entries []SurveyItem `json:"entries"`
	}
}

// APIListSurvey returns survey entries for an instance as JSON.
func (h *Handler) APIListSurvey(ctx context.Context, input *struct {
	Project  string `path:"project"`
	Analyzer string `query:"analyzer"`
	Kind     string `query:"kind"`
	Limit    int    `query:"limit" minimum:"1" maximum:"500" default:"100"`
}) (*ListSurveyOutput, error) {
	inst := h.findInstance(input.Project)
	if inst == nil {
		return nil, huma.Error404NotFound("instance not found")
	}
	ss := inst.SurveyStore()
	if ss == nil {
		return nil, huma.Error503ServiceUnavailable("instance not connected")
	}

	entries, err := ss.ListEntries(survey.SearchOptions{
		Analyzer: input.Analyzer,
		Kind:     input.Kind,
		Limit:    input.Limit,
	})
	if err != nil {
		return nil, err
	}

	out := &ListSurveyOutput{}
	for _, e := range entries {
		out.Body.Entries = append(out.Body.Entries, SurveyItem{
			ID:       e.ID,
			Analyzer: e.Analyzer,
			Kind:     e.Kind,
			Name:     e.Name,
			FilePath: e.FilePath,
			Title:    e.Title,
			Detail:   e.Detail,
			Metadata: e.Metadata,
		})
	}
	return out, nil
}

// SurveyGraphOutput is the response body for APISurveyGraph.
type SurveyGraphOutput struct {
	Body survey.CallGraph
}

// APISurveyGraph builds a call graph around a symbol from the instance's
// code index, on demand — the graph is never stored.
func (h *Handler) APISurveyGraph(ctx context.Context, input *struct {
	Project   string `path:"project"`
	Symbol    string `query:"symbol" required:"true"`
	Direction string `query:"direction" enum:"both,callers,callees" default:"both"`
	MaxDepth  int    `query:"max_depth" minimum:"1" maximum:"4" default:"1"`
	MaxNodes  int    `query:"max_nodes" minimum:"10" maximum:"200" default:"80"`
}) (*SurveyGraphOutput, error) {
	inst := h.findInstance(input.Project)
	if inst == nil {
		return nil, huma.Error404NotFound("instance not found")
	}
	cs := inst.CodeStore()
	if cs == nil {
		return nil, huma.Error503ServiceUnavailable("instance not connected")
	}

	graph, err := survey.BuildCallGraph(surveyrun.NewCodeGrapher(cs), input.Symbol, survey.GraphOptions{
		Direction: input.Direction,
		MaxDepth:  input.MaxDepth,
		MaxNodes:  input.MaxNodes,
	})
	if err != nil {
		return nil, huma.Error404NotFound(err.Error())
	}
	return &SurveyGraphOutput{Body: *graph}, nil
}
