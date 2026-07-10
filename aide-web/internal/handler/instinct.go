package handler

import (
	"context"
	"net/http"
	"net/url"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/go-chi/chi/v5"
	"github.com/jmylchreest/aide/aide/pkg/grpcapi"
	"github.com/jmylchreest/aide/aide/pkg/instinct"
	"github.com/jmylchreest/aide/aide/pkg/memory"
	"github.com/jmylchreest/aide/aide/pkg/store"
)

func newInstinctMemoryFromProposal(p *instinct.Proposal, content string) *memory.Memory {
	return &memory.Memory{
		Category: memory.Category(p.ProposedInstinct.Category),
		Content:  content,
		Tags:     p.ProposedInstinct.Tags,
		Priority: p.ProposedInstinct.Priority,
	}
}

type InstinctEvidenceItem struct {
	ObserveEventIDs []string           `json:"observe_event_ids,omitempty"`
	CrossSessionIDs []string           `json:"cross_session_ids,omitempty"`
	Snapshot        []ObserveEventItem `json:"snapshot,omitempty"`
}

type InstinctProposedMemoryItem struct {
	Category string   `json:"category"`
	Content  string   `json:"content"`
	Tags     []string `json:"tags,omitempty"`
	Priority float32  `json:"priority,omitempty"`
}

type InstinctProposalItem struct {
	ID               string                     `json:"id"`
	Shape            string                     `json:"shape"`
	SessionID        string                     `json:"session_id,omitempty"`
	ProposedAt       string                     `json:"proposed_at"`
	Summary          string                     `json:"summary"`
	Status           string                     `json:"status"`
	RejectionCount   int                        `json:"rejection_count,omitempty"`
	RejectionReason  string                     `json:"rejection_reason,omitempty"`
	AcceptedMemoryID string                     `json:"accepted_memory_id,omitempty"`
	LastReproposalAt string                     `json:"last_reproposal_at,omitempty"`
	ExpiresAt        string                     `json:"expires_at,omitempty"`
	Evidence         InstinctEvidenceItem       `json:"evidence"`
	ProposedInstinct InstinctProposedMemoryItem `json:"proposed_instinct"`
}

type ListInstinctProposalsOutput struct {
	Body struct {
		Proposals []InstinctProposalItem `json:"proposals"`
	}
}

type GetInstinctProposalOutput struct {
	Body InstinctProposalItem
}

type AcceptInstinctProposalOutput struct {
	Body struct {
		ProposalID string `json:"proposal_id"`
		MemoryID   string `json:"memory_id,omitempty"`
		Status     string `json:"status"`
	}
}

type RejectInstinctProposalOutput struct {
	Body struct {
		ProposalID     string `json:"proposal_id"`
		Status         string `json:"status"`
		RejectionCount int    `json:"rejection_count"`
	}
}

func (h *Handler) APIListInstinctProposals(ctx context.Context, input *struct {
	Project   string `path:"project"`
	Status    string `query:"status" doc:"Filter by status (open|accepted|rejected|expired); blank = all"`
	Shape     string `query:"shape"`
	SessionID string `query:"session"`
	Limit     int    `query:"limit" minimum:"1" maximum:"500" default:"100"`
}) (*ListInstinctProposalsOutput, error) {
	inst := h.findInstance(input.Project)
	if inst == nil {
		return nil, huma.Error404NotFound("instance not found")
	}
	ps := inst.InstinctStore()
	if ps == nil {
		return nil, huma.Error503ServiceUnavailable("instance not connected")
	}
	props, err := ps.ListInstinctProposals(store.InstinctFilter{
		Status:    instinct.Status(input.Status),
		Shape:     input.Shape,
		SessionID: input.SessionID,
		Limit:     input.Limit,
	})
	if err != nil {
		return nil, err
	}
	out := &ListInstinctProposalsOutput{}
	out.Body.Proposals = make([]InstinctProposalItem, 0, len(props))
	for _, p := range props {
		out.Body.Proposals = append(out.Body.Proposals, instinctToItem(p))
	}
	return out, nil
}

func (h *Handler) APIGetInstinctProposal(ctx context.Context, input *struct {
	Project string `path:"project"`
	ID      string `path:"id"`
}) (*GetInstinctProposalOutput, error) {
	inst := h.findInstance(input.Project)
	if inst == nil {
		return nil, huma.Error404NotFound("instance not found")
	}
	ps := inst.InstinctStore()
	if ps == nil {
		return nil, huma.Error503ServiceUnavailable("instance not connected")
	}
	p, err := ps.GetInstinctProposal(input.ID)
	if err != nil {
		return nil, err
	}
	if p == nil {
		return nil, huma.Error404NotFound("proposal not found")
	}
	return &GetInstinctProposalOutput{Body: instinctToItem(p)}, nil
}

func (h *Handler) APIAcceptInstinctProposal(ctx context.Context, input *struct {
	Project string `path:"project"`
	ID      string `path:"id"`
	Body    struct {
		ContentOverride string `json:"content_override,omitempty"`
	}
}) (*AcceptInstinctProposalOutput, error) {
	inst := h.findInstance(input.Project)
	if inst == nil {
		return nil, huma.Error404NotFound("instance not found")
	}
	st := inst.Store()
	ps := inst.InstinctStore()
	if st == nil || ps == nil {
		return nil, huma.Error503ServiceUnavailable("instance not connected")
	}
	prop, err := ps.GetInstinctProposal(input.ID)
	if err != nil {
		return nil, err
	}
	if prop == nil {
		return nil, huma.Error404NotFound("proposal not found")
	}
	content := prop.ProposedInstinct.Content
	if input.Body.ContentOverride != "" {
		content = input.Body.ContentOverride
	}
	mem := newInstinctMemoryFromProposal(prop, content)
	if err := st.AddMemory(mem); err != nil {
		return nil, err
	}
	if _, err := ps.UpdateInstinctProposalStatus(input.ID, instinct.StatusAccepted, "", mem.ID); err != nil {
		return nil, err
	}
	out := &AcceptInstinctProposalOutput{}
	out.Body.ProposalID = input.ID
	out.Body.MemoryID = mem.ID
	out.Body.Status = string(instinct.StatusAccepted)
	return out, nil
}

func (h *Handler) APIRejectInstinctProposal(ctx context.Context, input *struct {
	Project string `path:"project"`
	ID      string `path:"id"`
	Body    struct {
		Reason string `json:"reason,omitempty"`
	}
}) (*RejectInstinctProposalOutput, error) {
	inst := h.findInstance(input.Project)
	if inst == nil {
		return nil, huma.Error404NotFound("instance not found")
	}
	ps := inst.InstinctStore()
	if ps == nil {
		return nil, huma.Error503ServiceUnavailable("instance not connected")
	}
	p, err := ps.UpdateInstinctProposalStatus(input.ID, instinct.StatusRejected, input.Body.Reason, "")
	if err != nil {
		return nil, err
	}
	if p == nil {
		return nil, huma.Error404NotFound("proposal not found")
	}
	out := &RejectInstinctProposalOutput{}
	out.Body.ProposalID = input.ID
	out.Body.Status = string(p.Status)
	out.Body.RejectionCount = p.RejectionCount
	return out, nil
}

// APIWatchInstinctProposals streams proposals as SSE.
func (h *Handler) APIWatchInstinctProposals(w http.ResponseWriter, r *http.Request) {
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

	req := &grpcapi.InstinctWatchRequest{
		Status:    q.Get("status"),
		Shape:     q.Get("shape"),
		SessionId: q.Get("session"),
		SinceId:   sinceID,
	}
	stream, err := client.Instinct.Watch(r.Context(), req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	_ = StreamSSE(w, r, func() (*InstinctProposalItem, error) {
		p, err := stream.Recv()
		if err != nil {
			return nil, err
		}
		item := protoInstinctToItem(p)
		return &item, nil
	}, func(item *InstinctProposalItem) string {
		return item.ID
	})
}

func instinctToItem(p *instinct.Proposal) InstinctProposalItem {
	out := InstinctProposalItem{
		ID:               p.ID,
		Shape:            p.Shape,
		SessionID:        p.SessionID,
		ProposedAt:       formatRFC(p.ProposedAt),
		Summary:          p.Summary,
		Status:           string(p.Status),
		RejectionCount:   p.RejectionCount,
		RejectionReason:  p.RejectionReason,
		AcceptedMemoryID: p.AcceptedMemoryID,
		LastReproposalAt: formatRFC(p.LastReproposalAt),
		ExpiresAt:        formatRFC(p.ExpiresAt),
	}
	out.Evidence.ObserveEventIDs = p.Evidence.ObserveEventIDs
	out.Evidence.CrossSessionIDs = p.Evidence.CrossSessionIDs
	for _, e := range p.Evidence.Snapshot {
		out.Evidence.Snapshot = append(out.Evidence.Snapshot, observeEventToItem(e))
	}
	out.ProposedInstinct = InstinctProposedMemoryItem{
		Category: p.ProposedInstinct.Category,
		Content:  p.ProposedInstinct.Content,
		Tags:     p.ProposedInstinct.Tags,
		Priority: p.ProposedInstinct.Priority,
	}
	return out
}

func protoInstinctToItem(p *grpcapi.InstinctProposal) InstinctProposalItem {
	if p == nil {
		return InstinctProposalItem{}
	}
	out := InstinctProposalItem{
		ID:               p.Id,
		Shape:            p.Shape,
		SessionID:        p.SessionId,
		Summary:          p.Summary,
		Status:           p.Status,
		RejectionCount:   int(p.RejectionCount),
		RejectionReason:  p.RejectionReason,
		AcceptedMemoryID: p.AcceptedMemoryId,
	}
	if p.ProposedAt != nil {
		out.ProposedAt = p.ProposedAt.AsTime().UTC().Format(time.RFC3339Nano)
	}
	if p.LastReproposalAt != nil {
		out.LastReproposalAt = p.LastReproposalAt.AsTime().UTC().Format(time.RFC3339Nano)
	}
	if p.ExpiresAt != nil {
		out.ExpiresAt = p.ExpiresAt.AsTime().UTC().Format(time.RFC3339Nano)
	}
	if p.Evidence != nil {
		out.Evidence.ObserveEventIDs = p.Evidence.ObserveEventIds
		out.Evidence.CrossSessionIDs = p.Evidence.CrossSessionIds
		for _, e := range p.Evidence.Snapshot {
			out.Evidence.Snapshot = append(out.Evidence.Snapshot, protoToObserveEventItem(e))
		}
	}
	if p.ProposedInstinct != nil {
		out.ProposedInstinct = InstinctProposedMemoryItem{
			Category: p.ProposedInstinct.Category,
			Content:  p.ProposedInstinct.Content,
			Tags:     p.ProposedInstinct.Tags,
			Priority: p.ProposedInstinct.Priority,
		}
	}
	return out
}

func formatRFC(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339Nano)
}
