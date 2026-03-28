package handler

import (
	"context"

	"github.com/danielgtaylor/huma/v2"
	"github.com/jmylchreest/aide/aide/pkg/grpcapi"
)

// StatusOutput is the response body for APIGetStatus.
type StatusOutput struct {
	Body struct {
		Status  string `json:"status"`
		Version string `json:"version,omitempty"`
	}
}

// APIGetStatus returns the connection status for a single instance.
func (h *Handler) APIGetStatus(ctx context.Context, input *struct {
	Project string `path:"project"`
}) (*StatusOutput, error) {
	inst := h.findInstance(input.Project)
	if inst == nil {
		return nil, huma.Error404NotFound("instance not found")
	}

	out := &StatusOutput{}
	out.Body.Status = string(inst.Status())
	out.Body.Version = inst.Version()
	return out, nil
}

// DetailedStatusOutput is the response body for APIGetDetailedStatus.
type DetailedStatusOutput struct {
	Body struct {
		Version       string              `json:"version"`
		Uptime        string              `json:"uptime"`
		ServerRunning bool                `json:"server_running"`
		Watcher       *WatcherStatus      `json:"watcher,omitempty"`
		CodeIndexer   *CodeIndexerStatus  `json:"code_indexer,omitempty"`
		Findings      *FindingsSummary    `json:"findings,omitempty"`
		Survey        *SurveySummary      `json:"survey,omitempty"`
		Stores        []StoreInfo         `json:"stores,omitempty"`
		Grammars      []GrammarInfo       `json:"grammars,omitempty"`
	}
}

type WatcherStatus struct {
	Enabled     bool     `json:"enabled"`
	Paths       []string `json:"paths"`
	DirsWatched int32    `json:"dirs_watched"`
	Debounce    string   `json:"debounce"`
	Pending     int32    `json:"pending"`
	Subscribers []string `json:"subscribers"`
}

type CodeIndexerStatus struct {
	Available  bool   `json:"available"`
	Status     string `json:"status"`
	Symbols    int32  `json:"symbols"`
	References int32  `json:"references"`
	Files      int32  `json:"files"`
}

type FindingsSummary struct {
	Available  bool                       `json:"available"`
	Total      int32                      `json:"total"`
	ByAnalyzer map[string]int32           `json:"by_analyzer"`
	BySeverity map[string]int32           `json:"by_severity"`
	Analyzers  map[string]*AnalyzerStatus `json:"analyzers"`
}

type AnalyzerStatus struct {
	Status       string `json:"status"`
	Scope        string `json:"scope"`
	LastRun      string `json:"last_run"`
	Findings     int32  `json:"findings"`
	LastDuration string `json:"last_duration"`
}

type SurveySummary struct {
	Available  bool             `json:"available"`
	Total      int32            `json:"total"`
	ByAnalyzer map[string]int32 `json:"by_analyzer"`
	ByKind     map[string]int32 `json:"by_kind"`
}

type StoreInfo struct {
	Name string `json:"name"`
	Path string `json:"path"`
	Size int64  `json:"size"` // bytes
}

type GrammarInfo struct {
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
	BuiltIn bool   `json:"built_in"`
}

// APIGetDetailedStatus returns full daemon status via gRPC.
func (h *Handler) APIGetDetailedStatus(ctx context.Context, input *struct {
	Project string `path:"project"`
}) (*DetailedStatusOutput, error) {
	inst := h.findInstance(input.Project)
	if inst == nil {
		return nil, huma.Error404NotFound("instance not found")
	}
	client := inst.Client()
	if client == nil {
		return nil, huma.Error503ServiceUnavailable("instance not connected")
	}

	resp, err := client.Status.GetStatus(ctx, &grpcapi.StatusRequest{})
	if err != nil {
		return nil, err
	}

	out := &DetailedStatusOutput{}
	out.Body.Version = resp.Version
	out.Body.Uptime = resp.Uptime
	out.Body.ServerRunning = resp.ServerRunning

	if w := resp.Watcher; w != nil {
		out.Body.Watcher = &WatcherStatus{
			Enabled:     w.Enabled,
			Paths:       w.Paths,
			DirsWatched: w.DirsWatched,
			Debounce:    w.Debounce,
			Pending:     w.PendingFiles,
			Subscribers: w.Subscribers,
		}
	}

	if c := resp.CodeIndexer; c != nil {
		out.Body.CodeIndexer = &CodeIndexerStatus{
			Available:  c.Available,
			Status:     c.Status,
			Symbols:    c.Symbols,
			References: c.References,
			Files:      c.Files,
		}
	}

	if f := resp.Findings; f != nil {
		fs := &FindingsSummary{
			Available:  f.Available,
			Total:      f.Total,
			ByAnalyzer: f.ByAnalyzer,
			BySeverity: f.BySeverity,
			Analyzers:  make(map[string]*AnalyzerStatus),
		}
		for name, a := range f.Analyzers {
			fs.Analyzers[name] = &AnalyzerStatus{
				Status:       a.Status,
				Scope:        a.Scope,
				LastRun:      a.LastRun,
				Findings:     a.Findings,
				LastDuration: a.LastDuration,
			}
		}
		out.Body.Findings = fs
	}

	if s := resp.Survey; s != nil {
		out.Body.Survey = &SurveySummary{
			Available:  s.Available,
			Total:      s.Total,
			ByAnalyzer: s.ByAnalyzer,
			ByKind:     s.ByKind,
		}
	}

	// Stores (from gRPC)
	for _, s := range resp.Stores {
		out.Body.Stores = append(out.Body.Stores, StoreInfo{
			Name: s.Name,
			Path: s.Path,
			Size: s.Size,
		})
	}

	// Grammars (from gRPC)
	for _, g := range resp.Grammars {
		out.Body.Grammars = append(out.Body.Grammars, GrammarInfo{
			Name:    g.Name,
			Version: g.Version,
			BuiltIn: g.BuiltIn,
		})
	}

	return out, nil
}
