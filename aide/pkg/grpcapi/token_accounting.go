package grpcapi

import "github.com/jmylchreest/aide/aide/pkg/memory"

func tokenQuantityToProto(q *memory.TokenQuantity) *TokenQuantity {
	if q == nil {
		return nil
	}
	return &TokenQuantity{Bytes: q.Bytes, EstimatedTokens: q.EstimatedTokens, Events: int64(q.Events)}
}

func tokenQuantityFromProto(q *TokenQuantity) memory.TokenQuantity {
	if q == nil {
		return memory.TokenQuantity{}
	}
	return memory.TokenQuantity{Bytes: q.Bytes, EstimatedTokens: q.EstimatedTokens, Events: int(q.Events)}
}

func TokenAccountingToProto(a *memory.TokenAccounting) *TokenAccounting {
	if a == nil {
		return nil
	}
	p := &TokenAccounting{Version: int32(a.Version), Estimator: a.Estimator, ByStage: map[string]*TokenQuantity{}, Arguments: tokenQuantityToProto(&a.Arguments), LegacyEvents: int64(a.LegacyEvents), MissingPayload: int64(a.MissingPayload), MissingIdentity: int64(a.MissingIdentity)}
	for k, v := range a.ByStage {
		p.ByStage[k] = tokenQuantityToProto(v)
	}
	return p
}

func TokenAccountingFromProto(p *TokenAccounting) *memory.TokenAccounting {
	if p == nil {
		return nil
	}
	a := &memory.TokenAccounting{Version: int(p.Version), Estimator: p.Estimator, ByStage: map[string]*memory.TokenQuantity{}, Arguments: tokenQuantityFromProto(p.Arguments), LegacyEvents: int(p.LegacyEvents), MissingPayload: int(p.MissingPayload), MissingIdentity: int(p.MissingIdentity)}
	for k, v := range p.ByStage {
		q := tokenQuantityFromProto(v)
		a.ByStage[k] = &q
	}
	return a
}
