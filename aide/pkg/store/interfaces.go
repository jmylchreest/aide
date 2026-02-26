// Package store provides storage backends for aide.
// This file defines interfaces for pluggable storage implementations.
package store

import (
	"time"

	"github.com/jmylchreest/aide/aide/pkg/code"
	"github.com/jmylchreest/aide/aide/pkg/findings"
	"github.com/jmylchreest/aide/aide/pkg/memory"
)

// MemoryStore provides memory CRUD operations.
type MemoryStore interface {
	AddMemory(m *memory.Memory) error
	GetMemory(id string) (*memory.Memory, error)
	UpdateMemory(m *memory.Memory) error
	DeleteMemory(id string) error
	ListMemories(opts memory.SearchOptions) ([]*memory.Memory, error)
	SearchMemories(query string, limit int) ([]*memory.Memory, error)
	ClearMemories() (int, error)
}

// StateStore provides state key-value operations.
type StateStore interface {
	SetState(st *memory.State) error
	GetState(key string) (*memory.State, error)
	DeleteState(key string) error
	ListState(agentFilter string) ([]*memory.State, error)
	ClearState(agentID string) (int, error)
	CleanupStaleState(maxAge time.Duration) (int, error)
}

// DecisionStore provides decision operations (append-only per topic).
type DecisionStore interface {
	SetDecision(d *memory.Decision) error
	GetDecision(topic string) (*memory.Decision, error)
	ListDecisions() ([]*memory.Decision, error)
	GetDecisionHistory(topic string) ([]*memory.Decision, error)
	DeleteDecision(topic string) (int, error)
	ClearDecisions() (int, error)
}

// MessageStore provides inter-agent message operations.
type MessageStore interface {
	AddMessage(m *memory.Message) error
	GetMessages(agentID string) ([]*memory.Message, error)
	AckMessage(messageID uint64, agentID string) error
	PruneMessages() (int, error)
}

// TaskStore provides task CRUD and lifecycle operations.
type TaskStore interface {
	CreateTask(t *memory.Task) error
	GetTask(id string) (*memory.Task, error)
	ListTasks(status memory.TaskStatus) ([]*memory.Task, error)
	ClaimTask(taskID, agentID string) (*memory.Task, error)
	CompleteTask(taskID, result string) error
	UpdateTask(t *memory.Task) error
	DeleteTask(id string) error
	ClearTasks(status memory.TaskStatus) (int, error)
}

// Store combines all domain-specific store interfaces.
// Implementations must satisfy all sub-interfaces.
type Store interface {
	MemoryStore
	StateStore
	DecisionStore
	MessageStore
	TaskStore
	Close() error
}

// CodeIndexStore provides code symbol indexing and search.
type CodeIndexStore interface {
	AddSymbol(sym *code.Symbol) error
	GetSymbol(id string) (*code.Symbol, error)
	DeleteSymbol(id string) error
	SearchSymbols(query string, opts code.SearchOptions) ([]*CodeSearchResult, error)
	GetFileSymbols(filePath string) ([]*code.Symbol, error)
	GetFileInfo(path string) (*code.FileInfo, error)
	SetFileInfo(info *code.FileInfo) error
	ClearFile(filePath string) error
	AddReference(ref *code.Reference) error
	SearchReferences(opts code.ReferenceSearchOptions) ([]*code.Reference, error)
	ClearFileReferences(filePath string) error
	Stats() (*code.IndexStats, error)
	Clear() error
	Close() error
}

// Verify BoltStore implements Store at compile time.
var _ Store = (*BoltStore)(nil)

// Verify CodeStore implements CodeIndexStore at compile time.
var _ CodeIndexStore = (*CodeStore)(nil)

// FindingsStore manages static analysis findings in a separate database.
type FindingsStore interface {
	AddFinding(f *findings.Finding) error
	GetFinding(id string) (*findings.Finding, error)
	DeleteFinding(id string) error
	SearchFindings(query string, opts findings.SearchOptions) ([]*findings.SearchResult, error)
	ListFindings(opts findings.SearchOptions) ([]*findings.Finding, error)
	GetFileFindings(filePath string) ([]*findings.Finding, error)
	ClearAnalyzer(analyzer string) (int, error)
	ReplaceFindingsForAnalyzer(analyzer string, newFindings []*findings.Finding) error
	ReplaceFindingsForAnalyzerAndFile(analyzer, filePath string, newFindings []*findings.Finding) error
	Stats() (*findings.Stats, error)
	Clear() error
	Close() error
}

var _ FindingsStore = (*FindingsStoreImpl)(nil)
