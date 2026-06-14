package handler

import (
	"context"
	"net/http"
	"net/url"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/go-chi/chi/v5"
	"github.com/jmylchreest/aide/aide/pkg/grpcapi"
	"github.com/jmylchreest/aide/aide/pkg/observe"
	"github.com/jmylchreest/aide/aide/pkg/store"
)

type ObserveEventItem struct {
	ID          string            `json:"id"`
	Timestamp   string            `json:"timestamp"`
	Kind        string            `json:"kind"`
	Name        string            `json:"name"`
	Category    string            `json:"category,omitempty"`
	Subtype     string            `json:"subtype,omitempty"`
	DurationMs  int64             `json:"duration_ms,omitempty"`
	Tokens      int               `json:"tokens,omitempty"`
	TokensSaved int               `json:"tokens_saved,omitempty"`
	FilePath    string            `json:"file_path,omitempty"`
	Parent      string            `json:"parent,omitempty"`
	SessionID   string            `json:"session_id,omitempty"`
	Error       string            `json:"error,omitempty"`
	Attrs       map[string]string `json:"attrs,omitempty"`
}

type ListObserveEventsOutput struct {
	Body struct {
		Events []ObserveEventItem `json:"events"`
	}
}

func (h *Handler) APIListObserveEvents(ctx context.Context, input *struct {
	Project   string `path:"project"`
	Kind      string `query:"kind"`
	Name      string `query:"name"`
	Category  string `query:"category"`
	SessionID string `query:"session"`
	Limit     int    `query:"limit" minimum:"1" maximum:"1000" default:"200"`
	Since     string `query:"since" doc:"RFC3339 lower bound (inclusive)"`
	Until     string `query:"until" doc:"RFC3339 upper bound (inclusive)"`
}) (*ListObserveEventsOutput, error) {
	inst := h.findInstance(input.Project)
	if inst == nil {
		return nil, huma.Error404NotFound("instance not found")
	}
	st := inst.Store()
	if st == nil {
		return nil, huma.Error503ServiceUnavailable("instance not connected")
	}

	since, until, err := parseTimeRange(input.Since, input.Until)
	if err != nil {
		return nil, huma.Error400BadRequest(err.Error())
	}

	filter := store.ObserveFilter{
		Kind:      observe.Kind(input.Kind),
		Name:      input.Name,
		Category:  input.Category,
		SessionID: input.SessionID,
		Since:     since,
		Until:     until,
		Limit:     input.Limit,
	}
	events, err := st.ListObserveEvents(filter)
	if err != nil {
		if isUnimplemented(err) {
			return nil, huma.Error501NotImplemented("this instance's aide daemon predates observe support — upgrade aide in that project")
		}
		return nil, err
	}

	out := &ListObserveEventsOutput{}
	out.Body.Events = make([]ObserveEventItem, 0, len(events))
	for _, e := range events {
		out.Body.Events = append(out.Body.Events, observeEventToItem(e))
	}
	return out, nil
}

// APIWatchObserveEvents streams observe events as SSE. EventSource sends
// Last-Event-ID on reconnect; preferred over the since_id query param.
func (h *Handler) APIWatchObserveEvents(w http.ResponseWriter, r *http.Request) {
	project, _ := url.PathUnescape(chi.URLParam(r, "project"))
	inst := h.findInstance(project)
	if inst == nil {
		http.Error(w, "instance not found", http.StatusNotFound)
		return
	}
	client := inst.Client()
	if client == nil {
		http.Error(w, "instance not connected", http.StatusServiceUnavailable)
		return
	}

	q := r.URL.Query()
	sinceID := r.Header.Get("Last-Event-ID")
	if sinceID == "" {
		sinceID = q.Get("since_id")
	}

	req := &grpcapi.ObserveWatchRequest{
		Kind:      q.Get("kind"),
		Name:      q.Get("name"),
		Category:  q.Get("category"),
		SessionId: q.Get("session"),
		SinceId:   sinceID,
	}

	stream, err := client.Observe.WatchEvents(r.Context(), req)
	if err != nil {
		if isUnimplemented(err) {
			http.Error(w, "instance daemon predates observe support — upgrade aide in that project", http.StatusNotImplemented)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	_ = StreamSSE(w, r, func() (*ObserveEventItem, error) {
		ev, err := stream.Recv()
		if err != nil {
			return nil, err
		}
		item := protoToObserveEventItem(ev)
		return &item, nil
	}, func(item *ObserveEventItem) string {
		return item.ID
	})
}

func observeEventToItem(e *observe.Event) ObserveEventItem {
	return ObserveEventItem{
		ID:          e.ID,
		Timestamp:   e.Timestamp.UTC().Format(time.RFC3339Nano),
		Kind:        string(e.Kind),
		Name:        e.Name,
		Category:    e.Category,
		Subtype:     e.Subtype,
		DurationMs:  e.DurationMs,
		Tokens:      e.Tokens,
		TokensSaved: e.TokensSaved,
		FilePath:    e.FilePath,
		Parent:      e.Parent,
		SessionID:   e.SessionID,
		Error:       e.Error,
		Attrs:       e.Attrs,
	}
}

func protoToObserveEventItem(e *grpcapi.ObserveEvent) ObserveEventItem {
	var ts string
	if e.Timestamp != nil {
		ts = e.Timestamp.AsTime().UTC().Format(time.RFC3339Nano)
	}
	return ObserveEventItem{
		ID:          e.Id,
		Timestamp:   ts,
		Kind:        e.Kind,
		Name:        e.Name,
		Category:    e.Category,
		Subtype:     e.Subtype,
		DurationMs:  e.DurationMs,
		Tokens:      int(e.Tokens),
		TokensSaved: int(e.TokensSaved),
		FilePath:    e.FilePath,
		Parent:      e.Parent,
		SessionID:   e.SessionId,
		Error:       e.Error,
		Attrs:       e.Attrs,
	}
}

