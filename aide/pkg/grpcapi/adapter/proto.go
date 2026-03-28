// Package adapter provides gRPC-backed store adapters that implement aide's
// store interfaces by delegating to a gRPC client. This allows secondary
// processes (MCP client mode, aide-web) to access aide's data stores without
// opening BoltDB directly.
package adapter

import (
	"time"

	"github.com/jmylchreest/aide/aide/pkg/code"
	"github.com/jmylchreest/aide/aide/pkg/findings"
	"github.com/jmylchreest/aide/aide/pkg/grpcapi"
	"github.com/jmylchreest/aide/aide/pkg/memory"
	"github.com/jmylchreest/aide/aide/pkg/survey"
)

// ProtoToMemory converts a protobuf Memory to the domain Memory type.
func ProtoToMemory(p *grpcapi.Memory) *memory.Memory {
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

// ProtoToMemories converts a slice of protobuf Memories.
func ProtoToMemories(ps []*grpcapi.Memory) []*memory.Memory {
	result := make([]*memory.Memory, len(ps))
	for i, p := range ps {
		result[i] = ProtoToMemory(p)
	}
	return result
}

// ProtoToState converts a protobuf State to the domain State type.
func ProtoToState(p *grpcapi.State) *memory.State {
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

// ProtoToStates converts a slice of protobuf States.
func ProtoToStates(ps []*grpcapi.State) []*memory.State {
	result := make([]*memory.State, len(ps))
	for i, p := range ps {
		result[i] = ProtoToState(p)
	}
	return result
}

// ProtoToDecision converts a protobuf Decision to the domain Decision type.
func ProtoToDecision(p *grpcapi.Decision) *memory.Decision {
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

// ProtoToDecisions converts a slice of protobuf Decisions.
func ProtoToDecisions(ps []*grpcapi.Decision) []*memory.Decision {
	result := make([]*memory.Decision, len(ps))
	for i, p := range ps {
		result[i] = ProtoToDecision(p)
	}
	return result
}

// ProtoToMessage converts a protobuf Message to the domain Message type.
func ProtoToMessage(p *grpcapi.Message) *memory.Message {
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

// ProtoToMessages converts a slice of protobuf Messages.
func ProtoToMessages(ps []*grpcapi.Message) []*memory.Message {
	result := make([]*memory.Message, len(ps))
	for i, p := range ps {
		result[i] = ProtoToMessage(p)
	}
	return result
}

// ProtoToTask converts a protobuf Task to the domain Task type.
func ProtoToTask(p *grpcapi.Task) *memory.Task {
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

// ProtoToTasks converts a slice of protobuf Tasks.
func ProtoToTasks(ps []*grpcapi.Task) []*memory.Task {
	result := make([]*memory.Task, len(ps))
	for i, p := range ps {
		result[i] = ProtoToTask(p)
	}
	return result
}

// ProtoToSymbol converts a protobuf Symbol to the domain Symbol type.
func ProtoToSymbol(p *grpcapi.Symbol) *code.Symbol {
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

// ProtoToSymbols converts a slice of protobuf Symbols.
func ProtoToSymbols(ps []*grpcapi.Symbol) []*code.Symbol {
	result := make([]*code.Symbol, len(ps))
	for i, p := range ps {
		result[i] = ProtoToSymbol(p)
	}
	return result
}

// ProtoToReference converts a protobuf CodeReference to the domain Reference type.
func ProtoToReference(p *grpcapi.CodeReference) *code.Reference {
	if p == nil {
		return nil
	}
	return &code.Reference{
		ID:         p.Id,
		SymbolName: p.SymbolName,
		Kind:       p.Kind,
		FilePath:   p.FilePath,
		Line:       int(p.Line),
		Column:     int(p.Column),
		Context:    p.Context,
		Language:   p.Language,
		CreatedAt:  p.CreatedAt.AsTime(),
	}
}

// ProtoToReferences converts a slice of protobuf CodeReferences.
func ProtoToReferences(ps []*grpcapi.CodeReference) []*code.Reference {
	result := make([]*code.Reference, len(ps))
	for i, p := range ps {
		result[i] = ProtoToReference(p)
	}
	return result
}

// ProtoToFinding converts a protobuf Finding to the domain Finding type.
func ProtoToFinding(pf *grpcapi.Finding) *findings.Finding {
	if pf == nil {
		return nil
	}
	var createdAt time.Time
	if pf.CreatedAt != nil {
		createdAt = pf.CreatedAt.AsTime()
	}
	return &findings.Finding{
		ID:        pf.Id,
		Analyzer:  pf.Analyzer,
		Severity:  pf.Severity,
		Category:  pf.Category,
		FilePath:  pf.FilePath,
		Line:      int(pf.Line),
		EndLine:   int(pf.EndLine),
		Title:     pf.Title,
		Detail:    pf.Detail,
		Metadata:  pf.Metadata,
		CreatedAt: createdAt,
	}
}

// ProtoToSurveyEntry converts a protobuf SurveyEntry to the domain Entry type.
func ProtoToSurveyEntry(pe *grpcapi.SurveyEntry) *survey.Entry {
	if pe == nil {
		return nil
	}
	var createdAt time.Time
	if pe.CreatedAt != nil {
		createdAt = pe.CreatedAt.AsTime()
	}
	return &survey.Entry{
		ID:        pe.Id,
		Analyzer:  pe.Analyzer,
		Kind:      pe.Kind,
		Name:      pe.Name,
		FilePath:  pe.FilePath,
		Title:     pe.Title,
		Detail:    pe.Detail,
		Metadata:  pe.Metadata,
		CreatedAt: createdAt,
	}
}
