package grpcapi

import "github.com/jmylchreest/aide/aide/pkg/memory"

func ProvenanceToProto(c *memory.CheckoutProvenance) *CheckoutProvenance {
	if c == nil {
		return nil
	}
	return &CheckoutProvenance{Id: c.ID, Branch: c.Branch, Commit: c.Commit}
}

func ProvenanceFromProto(c *CheckoutProvenance) *memory.CheckoutProvenance {
	if c == nil {
		return nil
	}
	return &memory.CheckoutProvenance{ID: c.Id, Branch: c.Branch, Commit: c.Commit}
}
