package grpcapi

import (
	"github.com/jmylchreest/aide/aide/pkg/memory"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func tokenChangeToProto(q *memory.TokenChange) *TokenChange {
	if q == nil {
		return nil
	}
	return &TokenChange{BeforeBytes: q.BeforeBytes, AfterBytes: q.AfterBytes, DeltaBytes: q.DeltaBytes, EstimatedTokenDelta: q.EstimatedTokenDelta, Events: int64(q.Events)}
}
func tokenChangeFromProto(q *TokenChange) memory.TokenChange {
	if q == nil {
		return memory.TokenChange{}
	}
	return memory.TokenChange{BeforeBytes: q.BeforeBytes, AfterBytes: q.AfterBytes, DeltaBytes: q.DeltaBytes, EstimatedTokenDelta: q.EstimatedTokenDelta, Events: int(q.Events)}
}
func tokenTransformationsToProto(a *memory.TokenTransformations) *TokenTransformations {
	if a == nil {
		return nil
	}
	p := &TokenTransformations{ByStage: map[string]*TokenChange{}, WindowsLimited: a.WindowsLimited, UnwindowedEvents: int64(a.UnwindowedEvents), InvalidEvents: int64(a.InvalidEvents)}
	for stage, q := range a.ByStage {
		p.ByStage[stage] = tokenChangeToProto(q)
	}
	for _, w := range a.Windows {
		p.Windows = append(p.Windows, &TokenChangeWindow{Host: w.Host, SessionId: w.SessionID, ActorId: w.ActorID, Epoch: w.Epoch, Stage: w.Stage, First: timestamppb.New(w.First), Last: timestamppb.New(w.Last), Change: tokenChangeToProto(&w.Change)})
	}
	return p
}
func tokenTransformationsFromProto(p *TokenTransformations) *memory.TokenTransformations {
	if p == nil {
		return nil
	}
	a := memory.NewTokenTransformations()
	a.WindowsLimited = p.WindowsLimited
	a.UnwindowedEvents = int(p.UnwindowedEvents)
	a.InvalidEvents = int(p.InvalidEvents)
	for stage, q := range p.ByStage {
		mq := tokenChangeFromProto(q)
		a.ByStage[stage] = &mq
	}
	for _, w := range p.Windows {
		a.Windows = append(a.Windows, &memory.TokenChangeWindow{Host: w.Host, SessionID: w.SessionId, ActorID: w.ActorId, Epoch: w.Epoch, Stage: w.Stage, First: w.First.AsTime(), Last: w.Last.AsTime(), Change: tokenChangeFromProto(w.Change)})
	}
	return a
}
