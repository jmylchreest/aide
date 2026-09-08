package grpcapi

import "github.com/jmylchreest/aide/aide/pkg/code"

func ReadCheckTextEstimateToProto(e *code.TextEstimate) *ReadCheckTextEstimate {
	if e == nil {
		return nil
	}
	return &ReadCheckTextEstimate{Bytes: e.Bytes, EstimatedTokens: e.EstimatedTokens, Estimator: e.Estimator}
}

// ProtoToReadCheckTextEstimate preserves absence from older daemons.
func ProtoToReadCheckTextEstimate(e *ReadCheckTextEstimate) *code.TextEstimate {
	if e == nil {
		return nil
	}
	return &code.TextEstimate{Bytes: e.Bytes, EstimatedTokens: e.EstimatedTokens, Estimator: e.Estimator}
}
