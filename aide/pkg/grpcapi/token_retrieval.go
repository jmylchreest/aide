package grpcapi

import (
	"github.com/jmylchreest/aide/aide/pkg/memory"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func tokenRetrievalsToProto(r *memory.TokenRetrievals) *TokenRetrievals {
	if r == nil {
		return nil
	}
	p := &TokenRetrievals{WindowsLimited: r.WindowsLimited, UnwindowedEvents: int64(r.UnwindowedEvents)}
	for _, w := range r.Windows {
		pw := &RetrievalWindow{Host: w.Host, SessionId: w.SessionID, ActorId: w.ActorID, Epoch: w.Epoch, First: timestamppb.New(w.First), Last: timestamppb.New(w.Last), Boundary: w.Boundary, Events: int64(w.Events), Observed: tokenQuantityToProto(&w.Observed), Unattributed: tokenQuantityToProto(&w.Unattributed), Reference: tokenQuantityToProto(&w.Reference), Comparison: tokenChangeToProto(w.Comparison), FullReadEvents: int64(w.FullReadEvents), SearchEvents: int64(w.SearchEvents), FailedEvents: int64(w.FailedEvents), MissingPayload: int64(w.MissingPayload), Clipped: w.Clipped, Issues: w.Issues, StepsLimited: w.StepsLimited}
		for _, s := range w.Sources {
			pw.Sources = append(pw.Sources, &RetrievalSource{File: s.File, Sha256: s.SHA256, Bytes: s.Bytes})
		}
		for _, s := range w.Steps {
			pw.Steps = append(pw.Steps, &RetrievalStep{Id: s.ID, InvocationId: s.InvocationID, At: timestamppb.New(s.At), Tool: s.Tool, Status: s.Status, Target: s.Target, Text: tokenQuantityToProto(s.Text)})
		}
		p.Windows = append(p.Windows, pw)
	}
	return p
}
func tokenRetrievalsFromProto(p *TokenRetrievals) *memory.TokenRetrievals {
	if p == nil {
		return nil
	}
	r := &memory.TokenRetrievals{Windows: []*memory.RetrievalWindow{}, WindowsLimited: p.WindowsLimited, UnwindowedEvents: int(p.UnwindowedEvents)}
	for _, w := range p.Windows {
		mw := &memory.RetrievalWindow{Host: w.Host, SessionID: w.SessionId, ActorID: w.ActorId, Epoch: w.Epoch, First: w.First.AsTime(), Last: w.Last.AsTime(), Boundary: w.Boundary, Events: int(w.Events), Observed: tokenQuantityFromProto(w.Observed), Unattributed: tokenQuantityFromProto(w.Unattributed), Reference: tokenQuantityFromProto(w.Reference), FullReadEvents: int(w.FullReadEvents), SearchEvents: int(w.SearchEvents), FailedEvents: int(w.FailedEvents), MissingPayload: int(w.MissingPayload), Clipped: w.Clipped, Issues: append([]string{}, w.Issues...), Sources: []*memory.RetrievalSource{}, Steps: []*memory.RetrievalStep{}, StepsLimited: w.StepsLimited}
		if w.Comparison != nil {
			q := tokenChangeFromProto(w.Comparison)
			mw.Comparison = &q
		}
		for _, s := range w.Sources {
			mw.Sources = append(mw.Sources, &memory.RetrievalSource{File: s.File, SHA256: s.Sha256, Bytes: s.Bytes})
		}
		for _, s := range w.Steps {
			ms := &memory.RetrievalStep{ID: s.Id, InvocationID: s.InvocationId, At: s.At.AsTime(), Tool: s.Tool, Status: s.Status, Target: s.Target}
			if s.Text != nil {
				q := tokenQuantityFromProto(s.Text)
				ms.Text = &q
			}
			mw.Steps = append(mw.Steps, ms)
		}
		r.Windows = append(r.Windows, mw)
	}
	return r
}
