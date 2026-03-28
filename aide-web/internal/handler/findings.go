package handler

import (
	"context"

	"github.com/danielgtaylor/huma/v2"
	"github.com/jmylchreest/aide/aide/pkg/findings"
)

// FindingItem is the JSON representation of a finding.
type FindingItem struct {
	ID       string `json:"id"`
	Analyzer string `json:"analyzer"`
	Severity string `json:"severity"`
	Category string `json:"category"`
	FilePath string `json:"file_path"`
	Line     int    `json:"line"`
	Title    string `json:"title"`
	Accepted bool   `json:"accepted"`
}

// ListFindingsOutput is the response body for APIListFindings.
type ListFindingsOutput struct {
	Body struct {
		Findings []FindingItem `json:"findings"`
	}
}

// APIListFindings returns findings for an instance as JSON.
func (h *Handler) APIListFindings(ctx context.Context, input *struct {
	Project  string `path:"project"`
	Analyzer string `query:"analyzer"`
	Severity string `query:"severity"`
	Limit    int    `query:"limit" minimum:"1" maximum:"500" default:"100"`
}) (*ListFindingsOutput, error) {
	inst := h.findInstance(input.Project)
	if inst == nil {
		return nil, huma.Error404NotFound("instance not found")
	}
	fs := inst.FindingsStore()
	if fs == nil {
		return nil, huma.Error503ServiceUnavailable("instance not connected")
	}

	findingsList, err := fs.ListFindings(findings.SearchOptions{
		Analyzer: input.Analyzer,
		Severity: input.Severity,
		Limit:    input.Limit,
	})
	if err != nil {
		return nil, err
	}

	out := &ListFindingsOutput{}
	for _, f := range findingsList {
		out.Body.Findings = append(out.Body.Findings, FindingItem{
			ID:       f.ID,
			Analyzer: f.Analyzer,
			Severity: f.Severity,
			Category: f.Category,
			FilePath: f.FilePath,
			Line:     f.Line,
			Title:    f.Title,
			Accepted: f.Accepted,
		})
	}
	return out, nil
}

// APIAcceptFindings marks findings as accepted by their IDs.
func (h *Handler) APIAcceptFindings(ctx context.Context, input *struct {
	Project string `path:"project"`
	Body    struct {
		IDs []string `json:"ids" required:"true"`
	}
}) (*struct{}, error) {
	inst := h.findInstance(input.Project)
	if inst == nil {
		return nil, huma.Error404NotFound("instance not found")
	}
	fs := inst.FindingsStore()
	if fs == nil {
		return nil, huma.Error503ServiceUnavailable("instance not connected")
	}
	if _, err := fs.AcceptFindings(input.Body.IDs); err != nil {
		return nil, err
	}
	return nil, nil
}
