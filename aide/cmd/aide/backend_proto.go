package main

import (
	"github.com/jmylchreest/aide/aide/pkg/code"
	"github.com/jmylchreest/aide/aide/pkg/grpcapi"
	"github.com/jmylchreest/aide/aide/pkg/memory"
)

// =============================================================================
// Proto conversion helpers (gRPC response → domain types)
// =============================================================================

func protoToCodeSearchResults(ps []*grpcapi.Symbol) []*CodeSearchResult {
	result := make([]*CodeSearchResult, len(ps))
	for i, p := range ps {
		result[i] = &CodeSearchResult{
			Symbol: protoToSymbol(p),
			Score:  0,
		}
	}
	return result
}

func protoToSymbol(p *grpcapi.Symbol) *code.Symbol {
	if p == nil {
		return nil
	}
	return &code.Symbol{
		ID:         p.Id,
		Name:       p.Name,
		Kind:       p.Kind,
		Signature:  p.Signature,
		DocComment: p.DocComment,
		FilePath:   p.FilePath,
		StartLine:  int(p.StartLine),
		EndLine:    int(p.EndLine),
		Language:   p.Language,
		CreatedAt:  p.CreatedAt.AsTime(),
	}
}

func protoToSymbols(ps []*grpcapi.Symbol) []*code.Symbol {
	result := make([]*code.Symbol, len(ps))
	for i, p := range ps {
		result[i] = protoToSymbol(p)
	}
	return result
}

func protoToMemory(p *grpcapi.Memory) *memory.Memory {
	if p == nil {
		return nil
	}
	m := &memory.Memory{
		ID:          p.Id,
		Category:    memory.Category(p.Category),
		Content:     p.Content,
		Tags:        p.Tags,
		Priority:    p.Priority,
		Plan:        p.Plan,
		Agent:       p.Agent,
		Namespace:   p.Namespace,
		AccessCount: p.AccessCount,
		CreatedAt:   p.CreatedAt.AsTime(),
		UpdatedAt:   p.UpdatedAt.AsTime(),
	}
	if p.LastAccessed != nil {
		m.LastAccessed = p.LastAccessed.AsTime()
	}
	return m
}

func protoToMemories(ps []*grpcapi.Memory) []*memory.Memory {
	result := make([]*memory.Memory, len(ps))
	for i, p := range ps {
		result[i] = protoToMemory(p)
	}
	return result
}

func protoToState(p *grpcapi.State) *memory.State {
	if p == nil {
		return nil
	}
	return &memory.State{
		Key:       p.Key,
		Value:     p.Value,
		Agent:     p.Agent,
		UpdatedAt: p.UpdatedAt.AsTime(),
	}
}

func protoToStates(ps []*grpcapi.State) []*memory.State {
	result := make([]*memory.State, len(ps))
	for i, p := range ps {
		result[i] = protoToState(p)
	}
	return result
}

func protoToDecision(p *grpcapi.Decision) *memory.Decision {
	if p == nil {
		return nil
	}
	return &memory.Decision{
		Topic:      p.Topic,
		Decision:   p.Decision,
		Rationale:  p.Rationale,
		Details:    p.Details,
		References: p.References,
		DecidedBy:  p.DecidedBy,
		CreatedAt:  p.CreatedAt.AsTime(),
	}
}

func protoToDecisions(ps []*grpcapi.Decision) []*memory.Decision {
	result := make([]*memory.Decision, len(ps))
	for i, p := range ps {
		result[i] = protoToDecision(p)
	}
	return result
}

func protoToMessage(p *grpcapi.Message) *memory.Message {
	if p == nil {
		return nil
	}
	return &memory.Message{
		ID:        p.Id,
		From:      p.From,
		To:        p.To,
		Content:   p.Content,
		Type:      p.Type,
		ReadBy:    p.ReadBy,
		CreatedAt: p.CreatedAt.AsTime(),
		ExpiresAt: p.ExpiresAt.AsTime(),
	}
}

func protoToMessages(ps []*grpcapi.Message) []*memory.Message {
	result := make([]*memory.Message, len(ps))
	for i, p := range ps {
		result[i] = protoToMessage(p)
	}
	return result
}

func protoToTask(p *grpcapi.Task) *memory.Task {
	if p == nil {
		return nil
	}
	return &memory.Task{
		ID:          p.Id,
		Title:       p.Title,
		Description: p.Description,
		Status:      memory.TaskStatus(p.Status),
		ClaimedBy:   p.ClaimedBy,
		Worktree:    p.Worktree,
		Result:      p.Result,
		CreatedAt:   p.CreatedAt.AsTime(),
		ClaimedAt:   p.ClaimedAt.AsTime(),
		CompletedAt: p.CompletedAt.AsTime(),
	}
}

func protoToTasks(ps []*grpcapi.Task) []*memory.Task {
	result := make([]*memory.Task, len(ps))
	for i, p := range ps {
		result[i] = protoToTask(p)
	}
	return result
}
