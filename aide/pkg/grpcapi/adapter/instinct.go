package adapter

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/jmylchreest/aide/aide/pkg/grpcapi"
	"github.com/jmylchreest/aide/aide/pkg/instinct"
	"github.com/jmylchreest/aide/aide/pkg/observe"
	"github.com/jmylchreest/aide/aide/pkg/store"
)

func (a *InstinctAdapter) rpcCtx() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), RPCTimeout)
}

// InstinctAdapter implements store.InstinctProposalStore by delegating to a
// gRPC InstinctService client. Returned by NewInstinctAdapter when a Client
// is available; CLI consumers that need direct DB access skip this and use
// the local BoltStore.
type InstinctAdapter struct {
	client *grpcapi.Client
}

func NewInstinctAdapter(c *grpcapi.Client) *InstinctAdapter {
	return &InstinctAdapter{client: c}
}

var _ store.InstinctProposalStore = (*InstinctAdapter)(nil)

func (a *InstinctAdapter) AddInstinctProposal(p *instinct.Proposal) error {
	ctx, cancel := a.rpcCtx()
	defer cancel()
	resp, err := a.client.Instinct.Add(ctx, &grpcapi.InstinctAddRequest{
		Proposal: instinctToProto(p),
	})
	if err != nil {
		return err
	}
	if resp.Proposal != nil {
		*p = *protoToInstinct(resp.Proposal)
	}
	return nil
}

func (a *InstinctAdapter) GetInstinctProposal(id string) (*instinct.Proposal, error) {
	ctx, cancel := a.rpcCtx()
	defer cancel()
	resp, err := a.client.Instinct.Get(ctx, &grpcapi.InstinctGetRequest{Id: id})
	if err != nil {
		return nil, err
	}
	if !resp.Found {
		return nil, nil
	}
	return protoToInstinct(resp.Proposal), nil
}

func (a *InstinctAdapter) ListInstinctProposals(f store.InstinctFilter) ([]*instinct.Proposal, error) {
	ctx, cancel := a.rpcCtx()
	defer cancel()
	resp, err := a.client.Instinct.List(ctx, &grpcapi.InstinctListRequest{
		Status:    string(f.Status),
		Shape:     f.Shape,
		SessionId: f.SessionID,
		Limit:     int32(f.Limit),
	})
	if err != nil {
		return nil, err
	}
	out := make([]*instinct.Proposal, 0, len(resp.Proposals))
	for _, p := range resp.Proposals {
		out = append(out, protoToInstinct(p))
	}
	return out, nil
}

func (a *InstinctAdapter) UpdateInstinctProposalStatus(id string, st instinct.Status, reason string, acceptedMemoryID string) (*instinct.Proposal, error) {
	ctx, cancel := a.rpcCtx()
	defer cancel()
	resp, err := a.client.Instinct.UpdateStatus(ctx, &grpcapi.InstinctUpdateStatusRequest{
		Id:               id,
		Status:           string(st),
		Reason:           reason,
		AcceptedMemoryId: acceptedMemoryID,
	})
	if err != nil {
		return nil, err
	}
	return protoToInstinct(resp.Proposal), nil
}

func (a *InstinctAdapter) CleanupInstinctProposals(rejectedTTL time.Duration) (int, int, error) {
	return 0, 0, fmt.Errorf("instinct proposal cleanup not supported via gRPC")
}

func instinctToProto(p *instinct.Proposal) *grpcapi.InstinctProposal {
	if p == nil {
		return nil
	}
	out := &grpcapi.InstinctProposal{
		Id:               p.ID,
		Shape:            p.Shape,
		SessionId:        p.SessionID,
		Summary:          p.Summary,
		Status:           string(p.Status),
		RejectionCount:   int32(p.RejectionCount),
		RejectionReason:  p.RejectionReason,
		AcceptedMemoryId: p.AcceptedMemoryID,
	}
	if !p.ProposedAt.IsZero() {
		out.ProposedAt = timestamppb.New(p.ProposedAt)
	}
	if !p.LastReproposalAt.IsZero() {
		out.LastReproposalAt = timestamppb.New(p.LastReproposalAt)
	}
	if !p.ExpiresAt.IsZero() {
		out.ExpiresAt = timestamppb.New(p.ExpiresAt)
	}
	out.Evidence = &grpcapi.InstinctEvidence{
		ObserveEventIds: p.Evidence.ObserveEventIDs,
		CrossSessionIds: p.Evidence.CrossSessionIDs,
	}
	for _, e := range p.Evidence.Snapshot {
		out.Evidence.Snapshot = append(out.Evidence.Snapshot, observeToProto(e))
	}
	out.ProposedInstinct = &grpcapi.InstinctProposedMemory{
		Category: p.ProposedInstinct.Category,
		Content:  p.ProposedInstinct.Content,
		Tags:     p.ProposedInstinct.Tags,
		Priority: p.ProposedInstinct.Priority,
	}
	return out
}

func protoToInstinct(p *grpcapi.InstinctProposal) *instinct.Proposal {
	if p == nil {
		return nil
	}
	out := &instinct.Proposal{
		ID:               p.Id,
		Shape:            p.Shape,
		SessionID:        p.SessionId,
		Summary:          p.Summary,
		Status:           instinct.Status(p.Status),
		RejectionCount:   int(p.RejectionCount),
		RejectionReason:  p.RejectionReason,
		AcceptedMemoryID: p.AcceptedMemoryId,
	}
	if p.ProposedAt != nil {
		out.ProposedAt = p.ProposedAt.AsTime()
	}
	if p.LastReproposalAt != nil {
		out.LastReproposalAt = p.LastReproposalAt.AsTime()
	}
	if p.ExpiresAt != nil {
		out.ExpiresAt = p.ExpiresAt.AsTime()
	}
	if p.Evidence != nil {
		out.Evidence.ObserveEventIDs = p.Evidence.ObserveEventIds
		out.Evidence.CrossSessionIDs = p.Evidence.CrossSessionIds
		for _, e := range p.Evidence.Snapshot {
			out.Evidence.Snapshot = append(out.Evidence.Snapshot, protoToObserve(e))
		}
	}
	if p.ProposedInstinct != nil {
		out.ProposedInstinct.Category = p.ProposedInstinct.Category
		out.ProposedInstinct.Content = p.ProposedInstinct.Content
		out.ProposedInstinct.Tags = p.ProposedInstinct.Tags
		out.ProposedInstinct.Priority = p.ProposedInstinct.Priority
	}
	return out
}

func observeToProto(e *observe.Event) *grpcapi.ObserveEvent {
	if e == nil {
		return nil
	}
	out := &grpcapi.ObserveEvent{
		Id:          e.ID,
		Kind:        string(e.Kind),
		Name:        e.Name,
		Category:    e.Category,
		Subtype:     e.Subtype,
		DurationMs:  e.DurationMs,
		Tokens:      int32(e.Tokens),
		TokensSaved: int32(e.TokensSaved),
		FilePath:    e.FilePath,
		Parent:      e.Parent,
		SessionId:   e.SessionID,
		Error:       e.Error,
		Attrs:       e.Attrs,
	}
	if !e.Timestamp.IsZero() {
		out.Timestamp = timestamppb.New(e.Timestamp)
	}
	return out
}

func protoToObserve(e *grpcapi.ObserveEvent) *observe.Event {
	if e == nil {
		return nil
	}
	out := &observe.Event{
		ID:          e.Id,
		Kind:        observe.Kind(e.Kind),
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
	if e.Timestamp != nil {
		out.Timestamp = e.Timestamp.AsTime()
	}
	return out
}
