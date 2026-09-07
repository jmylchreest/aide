package grpcapi

import (
	"github.com/jmylchreest/aide/aide/pkg/memory"
	"google.golang.org/protobuf/types/known/timestamppb"
)

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
	if a.Activity != nil {
		p.Activity = &TokenActivity{IntervalSeconds: a.Activity.IntervalSeconds}
		for _, b := range a.Activity.Buckets {
			pb := &TokenActivityBucket{Start: timestamppb.New(b.Start), Unmeasured: int64(b.Unmeasured), ByStage: map[string]*TokenQuantity{}}
			for stage, q := range b.ByStage {
				pb.ByStage[stage] = tokenQuantityToProto(q)
			}
			p.Activity.Buckets = append(p.Activity.Buckets, pb)
		}
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
	if p.Activity != nil {
		a.Activity = &memory.TokenActivity{IntervalSeconds: p.Activity.IntervalSeconds, Buckets: []*memory.TokenActivityBucket{}}
		for _, b := range p.Activity.Buckets {
			bucket := &memory.TokenActivityBucket{Start: b.Start.AsTime(), Unmeasured: int(b.Unmeasured), ByStage: map[string]*memory.TokenQuantity{}}
			for stage, q := range b.ByStage {
				mq := tokenQuantityFromProto(q)
				bucket.ByStage[stage] = &mq
			}
			a.Activity.Buckets = append(a.Activity.Buckets, bucket)
		}
	}
	return a
}
