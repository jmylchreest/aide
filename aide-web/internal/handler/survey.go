package handler

import (
	"context"

	"github.com/danielgtaylor/huma/v2"
	"github.com/jmylchreest/aide/aide/pkg/survey"
)

// SurveyItem is the JSON representation of a survey entry.
type SurveyItem struct {
	ID       string `json:"id"`
	Analyzer string `json:"analyzer"`
	Kind     string `json:"kind"`
	Name     string `json:"name"`
	FilePath string `json:"file_path"`
	Title    string `json:"title"`
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
		})
	}
	return out, nil
}
