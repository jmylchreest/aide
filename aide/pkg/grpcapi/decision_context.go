package grpcapi

import "github.com/jmylchreest/aide/aide/pkg/memory"

func DecisionContextToProto(c memory.DecisionContext) *DecisionContext {
	p := &DecisionContext{Supersedes: c.Supersedes}
	if c.Applicability != nil {
		p.Applicability = &Applicability{Kind: c.Applicability.Kind, Text: c.Applicability.Text}
		for _, s := range c.Applicability.AnyOf {
			p.Applicability.AnyOf = append(p.Applicability.AnyOf, &ApplicabilitySelector{Language: s.Language, Framework: s.Framework, Tools: s.Tools, Component: s.Component, Concern: s.Concern, Paths: s.Paths})
		}
	}
	return p
}

func DecisionContextFromProto(p *DecisionContext) memory.DecisionContext {
	if p == nil {
		return memory.DecisionContext{}
	}
	c := memory.DecisionContext{Supersedes: p.Supersedes}
	if p.Applicability != nil {
		c.Applicability = &memory.Applicability{Kind: p.Applicability.Kind, Text: p.Applicability.Text}
		for _, s := range p.Applicability.AnyOf {
			c.Applicability.AnyOf = append(c.Applicability.AnyOf, memory.ApplicabilitySelector{Language: s.GetLanguage(), Framework: s.GetFramework(), Tools: s.GetTools(), Component: s.GetComponent(), Concern: s.GetConcern(), Paths: s.GetPaths()})
		}
	}
	return c
}
