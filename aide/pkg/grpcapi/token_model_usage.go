package grpcapi

import "github.com/jmylchreest/aide/aide/pkg/memory"

func tokenModelUsageToProto(u *memory.TokenModelUsage) *TokenModelUsage {
	if u == nil {
		return nil
	}
	p := &TokenModelUsage{Version: int32(u.Version), Observations: int64(u.Observations), Conflicts: int64(u.Conflicts), Invalid: int64(u.Invalid)}
	for _, g := range u.BySource {
		pg := &ModelUsageSource{Host: g.Host, Source: g.Source, Model: g.Model, Provider: g.Provider, Observations: int64(g.Observations), SourceTimed: int64(g.SourceTimed), ObservedTimed: int64(g.ObservedTimed), Counters: map[string]*ModelUsageCounter{}}
		for k, c := range g.Counters {
			pg.Counters[k] = &ModelUsageCounter{Tokens: c.Tokens, Observations: int64(c.Observations)}
		}
		p.BySource = append(p.BySource, pg)
	}
	return p
}
func tokenModelUsageFromProto(p *TokenModelUsage) *memory.TokenModelUsage {
	if p == nil {
		return nil
	}
	u := &memory.TokenModelUsage{Version: int(p.Version), Observations: int(p.Observations), Conflicts: int(p.Conflicts), Invalid: int(p.Invalid), BySource: []*memory.ModelUsageSource{}}
	for _, g := range p.BySource {
		if g == nil {
			return nil
		}
		mg := &memory.ModelUsageSource{Host: g.Host, Source: g.Source, Model: g.Model, Provider: g.Provider, Observations: int(g.Observations), SourceTimed: int(g.SourceTimed), ObservedTimed: int(g.ObservedTimed), Counters: map[string]*memory.ModelUsageCounter{}}
		for k, c := range g.Counters {
			if c == nil {
				return nil
			}
			mg.Counters[k] = &memory.ModelUsageCounter{Tokens: c.Tokens, Observations: int(c.Observations)}
		}
		u.BySource = append(u.BySource, mg)
	}
	return u
}
