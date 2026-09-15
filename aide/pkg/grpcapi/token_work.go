package grpcapi

import "github.com/jmylchreest/aide/aide/pkg/memory"

func tokenWorkCountersToProto(c *memory.TokenWorkCounters) *TokenWorkCounters {
	if c == nil {
		return nil
	}
	return &TokenWorkCounters{Calls: int64(c.Calls), Returned: int64(c.Returned), ReportedErrors: int64(c.ReportedErrors), UnknownOutcomes: int64(c.UnknownOutcomes), ElapsedMs: c.ElapsedMs, MeasuredDurations: int64(c.MeasuredDurations), MissingDurations: int64(c.MissingDurations), UnassignedSessions: int64(c.UnassignedSessions), ReturnedText: tokenQuantityToProto(&c.ReturnedText), MissingPayload: int64(c.MissingPayload)}
}

func tokenWorkCountersFromProto(c *TokenWorkCounters) memory.TokenWorkCounters {
	if c == nil {
		return memory.TokenWorkCounters{}
	}
	return memory.TokenWorkCounters{Calls: int(c.Calls), Returned: int(c.Returned), ReportedErrors: int(c.ReportedErrors), UnknownOutcomes: int(c.UnknownOutcomes), ElapsedMs: c.ElapsedMs, MeasuredDurations: int(c.MeasuredDurations), MissingDurations: int(c.MissingDurations), UnassignedSessions: int(c.UnassignedSessions), ReturnedText: tokenQuantityFromProto(c.ReturnedText), MissingPayload: int(c.MissingPayload)}
}

func tokenWorkToProto(w *memory.TokenWork) *TokenWork {
	if w == nil {
		return nil
	}
	p := &TokenWork{Version: int32(w.Version), Totals: tokenWorkCountersToProto(&w.TokenWorkCounters), ByTool: map[string]*TokenWorkCounters{}}
	for name, c := range w.ByTool {
		p.ByTool[name] = tokenWorkCountersToProto(c)
	}
	return p
}

func tokenWorkFromProto(p *TokenWork) *memory.TokenWork {
	// Required message presence distinguishes recorded zero from absent
	// measurements. Proto scalar defaults alone cannot establish known empty.
	if p == nil || p.Totals == nil || p.Totals.ReturnedText == nil {
		return nil
	}
	w := &memory.TokenWork{Version: int(p.Version), TokenWorkCounters: tokenWorkCountersFromProto(p.Totals), ByTool: map[string]*memory.TokenWorkCounters{}}
	for name, c := range p.ByTool {
		if c == nil || c.ReturnedText == nil {
			return nil
		}
		row := tokenWorkCountersFromProto(c)
		w.ByTool[name] = &row
	}
	return w
}
