// Package store provides storage backends for aide.
// This file defines interfaces for pluggable storage implementations.
package store

import (
	"time"

	"github.com/jmylchreest/aide/aide/pkg/code"
	"github.com/jmylchreest/aide/aide/pkg/findings"
	"github.com/jmylchreest/aide/aide/pkg/instinct"
	"github.com/jmylchreest/aide/aide/pkg/memory"
	"github.com/jmylchreest/aide/aide/pkg/observe"
	"github.com/jmylchreest/aide/aide/pkg/survey"
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
	TouchMemory(ids []string) (int, error) // Increment AccessCount and update LastAccessed
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
	PruneCompletedTasks(maxAge time.Duration) (int, error)
}

// TokenEventStore provides a TokenEvent-shaped read view over observe events.
// The write surface (AddTokenEvent) was retired — callers should record
// observe events directly via ObserveEventStore.AddObserveEvent.
type TokenEventStore interface {
	ListTokenEvents(sessionID string, limit int, since, until time.Time) ([]*memory.TokenEvent, error)
	TokenStats(sessionID string, since, until time.Time) (*memory.TokenStats, error)
	CleanupTokenEvents(maxAge time.Duration) (int, error)
}

// ObserveEventStore provides the unified observability event API.
type ObserveEventStore interface {
	AddObserveEvent(e *observe.Event) error
	ListObserveEvents(f ObserveFilter) ([]*observe.Event, error)
	CleanupObserveEvents(maxAge time.Duration) (int, error)
}

// InstinctProposalStore is a standalone interface (not part of Store) so
// the gRPC StoreAdapter can stay free of instinct-only RPCs. Code that
// needs proposal access takes this interface explicitly.
type InstinctProposalStore interface {
	AddInstinctProposal(p *instinct.Proposal) error
	GetInstinctProposal(id string) (*instinct.Proposal, error)
	ListInstinctProposals(f InstinctFilter) ([]*instinct.Proposal, error)
	UpdateInstinctProposalStatus(id string, status instinct.Status, reason string, acceptedMemoryID string) (*instinct.Proposal, error)
	CleanupInstinctProposals(rejectedTTL time.Duration) (int, int, error)
}

// TombstoneStore is a standalone interface (not part of Store) so the gRPC
// StoreAdapter is not forced to grow tombstone RPCs. Tombstones are recorded
// server-side by DeleteMemory/DeleteDecision, so capture works over gRPC;
// direct read/write access is only available with a local DB handle.
type TombstoneStore interface {
	AddTombstone(t *memory.Tombstone) error
	GetTombstone(kind, id string) (*memory.Tombstone, error)
	ListTombstones() ([]*memory.Tombstone, error)
	DeleteTombstone(kind, id string) error
}

// Store combines all domain-specific store interfaces.
// Implementations must satisfy all sub-interfaces.
type Store interface {
	MemoryStore
	StateStore
	DecisionStore
	MessageStore
	TaskStore
	TokenEventStore
	ObserveEventStore
	Close() error
}

// CodeIndexStore provides code symbol indexing and search.
//
// IndexFileBatch is the only path interface consumers should use to write
// new symbols / references / file metadata — it commits all of a file's
// records in a single bbolt tx and a single Bleve batch. The per-record
// AddSymbol / AddReference / SetFileInfo methods remain on the concrete
// *CodeStore (for in-package tests with direct access) but are intentionally
// NOT part of the interface contract: every callable interface consumer was
// migrated to IndexFileBatch, and exposing the per-record API would invite
// regressions back to the per-record-fsync hot path.
type CodeIndexStore interface {
	GetSymbol(id string) (*code.Symbol, error)
	DeleteSymbol(id string) error
	SearchSymbols(query string, opts code.SearchOptions) ([]*CodeSearchResult, error)
	GetFileSymbols(filePath string) ([]*code.Symbol, error)
	GetContainingSymbol(filePath string, line int) (*code.Symbol, error)
	GetFileInfo(path string) (*code.FileInfo, error)
	ListAllFileInfo() ([]*code.FileInfo, error)
	ClearFile(filePath string) error
	IndexFileBatch(filePath string, symbols []*code.Symbol, refs []*code.Reference, mtime time.Time, sizeBytes int64) error
	SearchReferences(opts code.ReferenceSearchOptions) ([]*code.Reference, error)
	GetFileReferences(filePath string) ([]*code.Reference, error)
	ClearFileReferences(filePath string) error
	TopReferencedSymbols(limit int, kind string) ([]*code.SymbolRefCount, error)
	ListAllSymbols(limit int) ([]*code.Symbol, error)
	ListAllReferences(limit int) ([]*code.Reference, error)
	Stats() (*code.IndexStats, error)
	Clear() error
	Close() error
}

// Verify BoltStore implements Store at compile time.
var _ Store = (*BoltStore)(nil)

// Verify both local stores implement TombstoneStore at compile time.
var (
	_ TombstoneStore = (*BoltStore)(nil)
	_ TombstoneStore = (*CombinedStore)(nil)
)

// Verify CodeStore implements CodeIndexStore at compile time.
var _ CodeIndexStore = (*CodeStore)(nil)

// FindingsStore manages static analysis findings in a separate database.
type FindingsStore interface {
	AddFinding(f *findings.Finding) error
	GetFinding(id string) (*findings.Finding, error)
	DeleteFinding(id string) error
	AcceptFindings(ids []string) (int, error)
	AcceptFindingsByFilter(opts findings.SearchOptions) (int, error)
	SearchFindings(query string, opts findings.SearchOptions) ([]*findings.SearchResult, error)
	ListFindings(opts findings.SearchOptions) ([]*findings.Finding, error)
	GetFileFindings(filePath string) ([]*findings.Finding, error)
	ClearAnalyzer(analyzer string) (int, error)
	ReplaceFindingsForAnalyzer(analyzer string, newFindings []*findings.Finding) error
	ReplaceFindingsForAnalyzerAndFile(analyzer, filePath string, newFindings []*findings.Finding) error
	Stats(opts findings.SearchOptions) (*findings.Stats, error)
	Clear() error
	Close() error
}

var _ FindingsStore = (*FindingsStoreImpl)(nil)

// SurveyStore manages codebase survey entries in a separate database.
type SurveyStore interface {
	AddEntry(e *survey.Entry) error
	GetEntry(id string) (*survey.Entry, error)
	DeleteEntry(id string) error
	SearchEntries(query string, opts survey.SearchOptions) ([]*survey.SearchResult, error)
	ListEntries(opts survey.SearchOptions) ([]*survey.Entry, error)
	GetFileEntries(filePath string) ([]*survey.Entry, error)
	ClearAnalyzer(analyzer string) (int, error)
	ReplaceEntriesForAnalyzer(analyzer string, newEntries []*survey.Entry) error
	ReplaceEntriesForAnalyzerAndFile(analyzer, filePath string, newEntries []*survey.Entry) error
	Stats(opts survey.SearchOptions) (*survey.Stats, error)
	Clear() error
	Close() error
}

var _ SurveyStore = (*SurveyStoreImpl)(nil)
