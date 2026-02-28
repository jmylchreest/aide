// Package grpcapi provides the gRPC server implementation for aide.
package grpcapi

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/jmylchreest/aide/aide/internal/version"
	"github.com/jmylchreest/aide/aide/pkg/code"
	"github.com/jmylchreest/aide/aide/pkg/findings"
	"github.com/jmylchreest/aide/aide/pkg/grammar"
	"github.com/jmylchreest/aide/aide/pkg/memory"
	"github.com/jmylchreest/aide/aide/pkg/store"
	"github.com/jmylchreest/aide/aide/pkg/watcher"
	"github.com/oklog/ulid/v2"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// generateID creates a new ULID for use as an entity ID.
func generateID() string {
	return ulid.Make().String()
}

// SocketPathFromDB returns the Unix socket path derived from the database path.
// The socket is placed in the project's .aide directory (sibling to the memory dir).
func SocketPathFromDB(dbPath string) string {
	// dbPath is typically <project>/.aide/memory/memory.db
	// We want <project>/.aide/aide.sock
	aideDir := filepath.Dir(filepath.Dir(dbPath))
	return filepath.Join(aideDir, "aide.sock")
}

// Server manages the gRPC server and all service implementations.
type Server struct {
	store         store.Store
	codeStore     store.CodeIndexStore
	findingsStore store.FindingsStore
	dbPath        string
	grpcServer    *grpc.Server
	socketPath    string
	startTime     time.Time
	grammarLoader grammar.Loader

	// storeMu protects codeStore and findingsStore which may be set after
	// the gRPC server starts (e.g. lazy code store init).
	storeMu sync.RWMutex

	// Status-related fields (set by the MCP server process)
	mu             sync.RWMutex
	watcher        *watcher.Watcher
	findingsRunner *findings.Runner
	mcpTools       []*StatusMCPTool
	toolCountFunc  func() map[string]int64
}

// NewServer creates a new gRPC server.
func NewServer(st store.Store, dbPath, socketPath string, loader grammar.Loader) *Server {
	return &Server{
		store:         st,
		dbPath:        dbPath,
		socketPath:    socketPath,
		startTime:     time.Now(),
		grammarLoader: loader,
	}
}

// SetCodeStore sets the code store for code indexing services.
func (s *Server) SetCodeStore(cs store.CodeIndexStore) {
	s.storeMu.Lock()
	defer s.storeMu.Unlock()
	s.codeStore = cs
}

// SetFindingsStore sets the findings store for findings services.
func (s *Server) SetFindingsStore(fs store.FindingsStore) {
	s.storeMu.Lock()
	defer s.storeMu.Unlock()
	s.findingsStore = fs
}

// GetCodeStore returns the current code store (thread-safe).
func (s *Server) GetCodeStore() store.CodeIndexStore {
	s.storeMu.RLock()
	defer s.storeMu.RUnlock()
	return s.codeStore
}

// GetFindingsStore returns the current findings store (thread-safe).
func (s *Server) GetFindingsStore() store.FindingsStore {
	s.storeMu.RLock()
	defer s.storeMu.RUnlock()
	return s.findingsStore
}

// SetWatcher sets the watcher for status reporting.
func (s *Server) SetWatcher(w *watcher.Watcher) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.watcher = w
}

// SetFindingsRunner sets the findings runner for status reporting.
func (s *Server) SetFindingsRunner(r *findings.Runner) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.findingsRunner = r
}

// SetMCPTools sets the list of registered MCP tools.
func (s *Server) SetMCPTools(tools []*StatusMCPTool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.mcpTools = tools
}

// SetToolCountFunc sets the function used to retrieve tool execution counts.
func (s *Server) SetToolCountFunc(f func() map[string]int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.toolCountFunc = f
}

// Start starts the gRPC server on a Unix socket.
func (s *Server) Start() error {
	// Ensure socket directory exists
	socketDir := filepath.Dir(s.socketPath)
	if err := os.MkdirAll(socketDir, 0o700); err != nil {
		return fmt.Errorf("failed to create socket directory: %w", err)
	}

	// Remove existing socket if present
	if err := os.Remove(s.socketPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove existing socket: %w", err)
	}

	// Listen on Unix socket
	listener, err := net.Listen("unix", s.socketPath)
	if err != nil {
		return fmt.Errorf("failed to listen on socket: %w", err)
	}

	// Create gRPC server
	s.grpcServer = grpc.NewServer()

	// Register all services with separate implementations
	RegisterMemoryServiceServer(s.grpcServer, &memoryServiceImpl{store: s.store})
	RegisterStateServiceServer(s.grpcServer, &stateServiceImpl{store: s.store})
	RegisterDecisionServiceServer(s.grpcServer, &decisionServiceImpl{store: s.store})
	RegisterMessageServiceServer(s.grpcServer, &messageServiceImpl{store: s.store})
	RegisterTaskServiceServer(s.grpcServer, &taskServiceImpl{store: s.store})
	RegisterCodeServiceServer(s.grpcServer, &codeServiceImpl{server: s, parser: code.NewParser(s.grammarLoader)})
	RegisterFindingsServiceServer(s.grpcServer, &findingsServiceImpl{server: s})
	RegisterHealthServiceServer(s.grpcServer, &healthServiceImpl{dbPath: s.dbPath})
	RegisterStatusServiceServer(s.grpcServer, &statusServiceImpl{server: s})

	// Start serving
	return s.grpcServer.Serve(listener)
}

// Stop gracefully stops the gRPC server.
func (s *Server) Stop() {
	if s.grpcServer != nil {
		s.grpcServer.GracefulStop()
	}
	// Clean up socket file
	os.Remove(s.socketPath)
}

// SocketPath returns the socket path.
func (s *Server) SocketPath() string {
	return s.socketPath
}

// =============================================================================
// Health Service Implementation
// =============================================================================

type healthServiceImpl struct {
	UnimplementedHealthServiceServer
	dbPath string
}

func (s *healthServiceImpl) Check(ctx context.Context, req *HealthCheckRequest) (*HealthCheckResponse, error) {
	return &HealthCheckResponse{
		Healthy: true,
		Version: version.Short(),
		DbPath:  s.dbPath,
	}, nil
}

// =============================================================================
// Memory Service Implementation
// =============================================================================

type memoryServiceImpl struct {
	UnimplementedMemoryServiceServer
	store store.MemoryStore
}

func (s *memoryServiceImpl) Add(ctx context.Context, req *MemoryAddRequest) (*MemoryAddResponse, error) {
	mem := &memory.Memory{
		ID:        generateID(),
		Content:   req.Content,
		Category:  memory.Category(req.Category),
		Tags:      req.Tags,
		CreatedAt: time.Now(),
	}

	if err := s.store.AddMemory(mem); err != nil {
		return nil, err
	}

	return &MemoryAddResponse{
		Memory: memoryToProto(mem),
	}, nil
}

func (s *memoryServiceImpl) Get(ctx context.Context, req *MemoryGetRequest) (*MemoryGetResponse, error) {
	mem, err := s.store.GetMemory(req.Id)
	if err != nil {
		return nil, err
	}

	return &MemoryGetResponse{
		Memory: memoryToProto(mem),
	}, nil
}

func (s *memoryServiceImpl) Search(ctx context.Context, req *MemorySearchRequest) (*MemorySearchResponse, error) {
	limit := int(req.Limit)
	if limit == 0 {
		limit = 10
	}

	memories, err := s.store.SearchMemories(req.Query, limit)
	if err != nil {
		return nil, err
	}

	protoMemories := make([]*Memory, len(memories))
	for i, m := range memories {
		protoMemories[i] = memoryToProto(m)
	}

	return &MemorySearchResponse{
		Memories: protoMemories,
	}, nil
}

func (s *memoryServiceImpl) List(ctx context.Context, req *MemoryListRequest) (*MemoryListResponse, error) {
	limit := int(req.Limit)
	if limit == 0 {
		limit = 50
	}

	opts := memory.SearchOptions{
		Category: memory.Category(req.Category),
		Limit:    limit,
	}

	memories, err := s.store.ListMemories(opts)
	if err != nil {
		return nil, err
	}

	protoMemories := make([]*Memory, len(memories))
	for i, m := range memories {
		protoMemories[i] = memoryToProto(m)
	}

	return &MemoryListResponse{
		Memories: protoMemories,
	}, nil
}

func (s *memoryServiceImpl) Delete(ctx context.Context, req *MemoryDeleteRequest) (*MemoryDeleteResponse, error) {
	if err := s.store.DeleteMemory(req.Id); err != nil {
		return nil, err
	}

	return &MemoryDeleteResponse{
		Success: true,
	}, nil
}

func (s *memoryServiceImpl) Clear(ctx context.Context, req *MemoryClearRequest) (*MemoryClearResponse, error) {
	count, err := s.store.ClearMemories()
	if err != nil {
		return nil, err
	}

	return &MemoryClearResponse{
		Count: int32(count),
	}, nil
}

func (s *memoryServiceImpl) Touch(ctx context.Context, req *MemoryTouchRequest) (*MemoryTouchResponse, error) {
	touched, err := s.store.TouchMemory(req.Ids)
	if err != nil {
		return nil, err
	}

	return &MemoryTouchResponse{
		Touched: int32(touched),
	}, nil
}

// =============================================================================
// State Service Implementation
// =============================================================================

type stateServiceImpl struct {
	UnimplementedStateServiceServer
	store store.StateStore
}

func (s *stateServiceImpl) Get(ctx context.Context, req *StateGetRequest) (*StateGetResponse, error) {
	key := req.Key
	if req.AgentId != "" {
		key = fmt.Sprintf("agent:%s:%s", req.AgentId, req.Key)
	}

	st, err := s.store.GetState(key)
	if err != nil {
		if err == store.ErrNotFound {
			return &StateGetResponse{Found: false}, nil
		}
		return nil, err
	}

	return &StateGetResponse{
		State: stateToProto(st),
		Found: true,
	}, nil
}

func (s *stateServiceImpl) Set(ctx context.Context, req *StateSetRequest) (*StateSetResponse, error) {
	key := req.Key
	if req.AgentId != "" {
		key = fmt.Sprintf("agent:%s:%s", req.AgentId, req.Key)
	}

	st := &memory.State{
		Key:   key,
		Value: req.Value,
		Agent: req.AgentId,
	}

	if err := s.store.SetState(st); err != nil {
		return nil, err
	}

	return &StateSetResponse{
		State: stateToProto(st),
	}, nil
}

func (s *stateServiceImpl) List(ctx context.Context, req *StateListRequest) (*StateListResponse, error) {
	states, err := s.store.ListState(req.AgentId)
	if err != nil {
		return nil, err
	}

	protoStates := make([]*State, len(states))
	for i, st := range states {
		protoStates[i] = stateToProto(st)
	}

	return &StateListResponse{
		States: protoStates,
	}, nil
}

func (s *stateServiceImpl) Delete(ctx context.Context, req *StateDeleteRequest) (*StateDeleteResponse, error) {
	if err := s.store.DeleteState(req.Key); err != nil {
		return nil, err
	}

	return &StateDeleteResponse{
		Success: true,
	}, nil
}

func (s *stateServiceImpl) Clear(ctx context.Context, req *StateClearRequest) (*StateClearResponse, error) {
	count, err := s.store.ClearState(req.AgentId)
	if err != nil {
		return nil, err
	}

	return &StateClearResponse{
		Count: int32(count),
	}, nil
}

func (s *stateServiceImpl) Cleanup(ctx context.Context, req *StateCleanupRequest) (*StateCleanupResponse, error) {
	// Parse duration from request (default to 1 hour)
	maxAge := 1 * time.Hour
	if req.MaxAge != "" {
		if d, err := time.ParseDuration(req.MaxAge); err == nil {
			maxAge = d
		}
	}

	count, err := s.store.CleanupStaleState(maxAge)
	if err != nil {
		return nil, err
	}

	return &StateCleanupResponse{
		Count: int32(count),
	}, nil
}

// =============================================================================
// Decision Service Implementation
// =============================================================================

type decisionServiceImpl struct {
	UnimplementedDecisionServiceServer
	store store.DecisionStore
}

func (s *decisionServiceImpl) Set(ctx context.Context, req *DecisionSetRequest) (*DecisionSetResponse, error) {
	dec := &memory.Decision{
		Topic:      req.Topic,
		Decision:   req.Decision,
		Rationale:  req.Rationale,
		Details:    req.Details,
		References: req.References,
		DecidedBy:  req.DecidedBy,
		CreatedAt:  time.Now(),
	}

	if err := s.store.SetDecision(dec); err != nil {
		return nil, err
	}

	return &DecisionSetResponse{
		Decision: decisionToProto(dec),
	}, nil
}

func (s *decisionServiceImpl) Get(ctx context.Context, req *DecisionGetRequest) (*DecisionGetResponse, error) {
	dec, err := s.store.GetDecision(req.Topic)
	if err != nil {
		if err == store.ErrNotFound {
			return &DecisionGetResponse{Found: false}, nil
		}
		return nil, err
	}

	return &DecisionGetResponse{
		Decision: decisionToProto(dec),
		Found:    true,
	}, nil
}

func (s *decisionServiceImpl) List(ctx context.Context, req *DecisionListRequest) (*DecisionListResponse, error) {
	decisions, err := s.store.ListDecisions()
	if err != nil {
		return nil, err
	}

	protoDecisions := make([]*Decision, len(decisions))
	for i, d := range decisions {
		protoDecisions[i] = decisionToProto(d)
	}

	return &DecisionListResponse{
		Decisions: protoDecisions,
	}, nil
}

func (s *decisionServiceImpl) History(ctx context.Context, req *DecisionHistoryRequest) (*DecisionHistoryResponse, error) {
	decisions, err := s.store.GetDecisionHistory(req.Topic)
	if err != nil {
		return nil, err
	}

	protoDecisions := make([]*Decision, len(decisions))
	for i, d := range decisions {
		protoDecisions[i] = decisionToProto(d)
	}

	return &DecisionHistoryResponse{
		Decisions: protoDecisions,
	}, nil
}

func (s *decisionServiceImpl) Delete(ctx context.Context, req *DecisionDeleteRequest) (*DecisionDeleteResponse, error) {
	count, err := s.store.DeleteDecision(req.Topic)
	if err != nil {
		return nil, err
	}

	return &DecisionDeleteResponse{
		Count: int32(count),
	}, nil
}

func (s *decisionServiceImpl) Clear(ctx context.Context, req *DecisionClearRequest) (*DecisionClearResponse, error) {
	count, err := s.store.ClearDecisions()
	if err != nil {
		return nil, err
	}

	return &DecisionClearResponse{
		Count: int32(count),
	}, nil
}

// =============================================================================
// Message Service Implementation
// =============================================================================

type messageServiceImpl struct {
	UnimplementedMessageServiceServer
	store store.MessageStore
}

func (s *messageServiceImpl) Send(ctx context.Context, req *MessageSendRequest) (*MessageSendResponse, error) {
	ttl := int(req.TtlSeconds)
	if ttl == 0 {
		ttl = 3600
	}

	msg := &memory.Message{
		From:      req.From,
		To:        req.To,
		Content:   req.Content,
		Type:      req.Type,
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(time.Duration(ttl) * time.Second),
	}

	if err := s.store.AddMessage(msg); err != nil {
		return nil, err
	}

	return &MessageSendResponse{
		Message: messageToProto(msg),
	}, nil
}

func (s *messageServiceImpl) List(ctx context.Context, req *MessageListRequest) (*MessageListResponse, error) {
	messages, err := s.store.GetMessages(req.AgentId)
	if err != nil {
		return nil, err
	}

	protoMessages := make([]*Message, len(messages))
	for i, m := range messages {
		protoMessages[i] = messageToProto(m)
	}

	return &MessageListResponse{
		Messages: protoMessages,
	}, nil
}

func (s *messageServiceImpl) Ack(ctx context.Context, req *MessageAckRequest) (*MessageAckResponse, error) {
	if err := s.store.AckMessage(req.MessageId, req.AgentId); err != nil {
		return nil, err
	}

	return &MessageAckResponse{
		Success: true,
	}, nil
}

func (s *messageServiceImpl) Prune(ctx context.Context, req *MessagePruneRequest) (*MessagePruneResponse, error) {
	count, err := s.store.PruneMessages()
	if err != nil {
		return nil, err
	}

	return &MessagePruneResponse{
		Count: int32(count),
	}, nil
}

// =============================================================================
// Task Service Implementation
// =============================================================================

type taskServiceImpl struct {
	UnimplementedTaskServiceServer
	store store.TaskStore
}

func (s *taskServiceImpl) Create(ctx context.Context, req *TaskCreateRequest) (*TaskCreateResponse, error) {
	task := &memory.Task{
		ID:          generateID(),
		Title:       req.Title,
		Description: req.Description,
		Status:      memory.TaskStatusPending,
		CreatedAt:   time.Now(),
	}

	if err := s.store.CreateTask(task); err != nil {
		return nil, err
	}

	return &TaskCreateResponse{
		Task: taskToProto(task),
	}, nil
}

func (s *taskServiceImpl) Get(ctx context.Context, req *TaskGetRequest) (*TaskGetResponse, error) {
	task, err := s.store.GetTask(req.Id)
	if err != nil {
		if err == store.ErrNotFound {
			return &TaskGetResponse{Found: false}, nil
		}
		return nil, err
	}

	return &TaskGetResponse{
		Task:  taskToProto(task),
		Found: true,
	}, nil
}

func (s *taskServiceImpl) List(ctx context.Context, req *TaskListRequest) (*TaskListResponse, error) {
	tasks, err := s.store.ListTasks(memory.TaskStatus(req.Status))
	if err != nil {
		return nil, err
	}

	protoTasks := make([]*Task, len(tasks))
	for i, t := range tasks {
		protoTasks[i] = taskToProto(t)
	}

	return &TaskListResponse{
		Tasks: protoTasks,
	}, nil
}

func (s *taskServiceImpl) Claim(ctx context.Context, req *TaskClaimRequest) (*TaskClaimResponse, error) {
	task, err := s.store.ClaimTask(req.TaskId, req.AgentId)
	if err != nil {
		return &TaskClaimResponse{
			Success: false,
			Error:   err.Error(),
		}, nil
	}

	// Update worktree if provided and persist
	if req.Worktree != "" {
		task.Worktree = req.Worktree
		if err := s.store.UpdateTask(task); err != nil {
			return &TaskClaimResponse{
				Success: false,
				Error:   "claimed but failed to set worktree: " + err.Error(),
			}, nil
		}
	}

	return &TaskClaimResponse{
		Task:    taskToProto(task),
		Success: true,
	}, nil
}

func (s *taskServiceImpl) Complete(ctx context.Context, req *TaskCompleteRequest) (*TaskCompleteResponse, error) {
	if err := s.store.CompleteTask(req.TaskId, req.Result); err != nil {
		return nil, fmt.Errorf("complete task: %w", err)
	}

	task, err := s.store.GetTask(req.TaskId)
	if err != nil {
		// Task was completed but we can't retrieve it - still a success
		return &TaskCompleteResponse{
			Success: true,
		}, nil
	}
	return &TaskCompleteResponse{
		Task:    taskToProto(task),
		Success: true,
	}, nil
}

func (s *taskServiceImpl) Update(ctx context.Context, req *TaskUpdateRequest) (*TaskUpdateResponse, error) {
	// Get existing task
	task, err := s.store.GetTask(req.TaskId)
	if err != nil {
		return nil, fmt.Errorf("get task for update: %w", err)
	}

	// Update fields
	if req.Status != "" {
		task.Status = memory.TaskStatus(req.Status)
	}
	if req.Result != "" {
		task.Result = req.Result
	}

	// Persist the changes
	if err := s.store.UpdateTask(task); err != nil {
		return nil, fmt.Errorf("update task: %w", err)
	}

	return &TaskUpdateResponse{
		Task:    taskToProto(task),
		Success: true,
	}, nil
}

func (s *taskServiceImpl) Delete(ctx context.Context, req *TaskDeleteRequest) (*TaskDeleteResponse, error) {
	if err := s.store.DeleteTask(req.Id); err != nil {
		return nil, fmt.Errorf("delete task: %w", err)
	}
	return &TaskDeleteResponse{Success: true}, nil
}

func (s *taskServiceImpl) Clear(ctx context.Context, req *TaskClearRequest) (*TaskClearResponse, error) {
	count, err := s.store.ClearTasks(memory.TaskStatus(req.Status))
	if err != nil {
		return nil, err
	}
	return &TaskClearResponse{Count: int32(count)}, nil
}

// =============================================================================
// Code Service Implementation
// =============================================================================

type codeServiceImpl struct {
	UnimplementedCodeServiceServer
	server *Server
	parser *code.Parser
}

func (s *codeServiceImpl) Search(ctx context.Context, req *CodeSearchRequest) (*CodeSearchResponse, error) {
	cs := s.server.GetCodeStore()
	if cs == nil {
		return nil, fmt.Errorf("code store not available")
	}

	limit := int(req.Limit)
	if limit == 0 {
		limit = 20
	}

	opts := code.SearchOptions{
		Kind:     req.Kind,
		Language: req.Language,
		FilePath: req.FilePath,
		Limit:    limit,
	}

	results, err := cs.SearchSymbols(req.Query, opts)
	if err != nil {
		return nil, err
	}

	protoSymbols := make([]*Symbol, len(results))
	for i, r := range results {
		protoSymbols[i] = symbolToProto(r.Symbol)
	}

	return &CodeSearchResponse{
		Symbols: protoSymbols,
	}, nil
}

func (s *codeServiceImpl) Symbols(ctx context.Context, req *CodeSymbolsRequest) (*CodeSymbolsResponse, error) {
	cs := s.server.GetCodeStore()
	if cs == nil {
		return nil, fmt.Errorf("code store not available")
	}

	symbols, err := cs.GetFileSymbols(req.FilePath)
	if err != nil {
		// If file not in index, try to parse it directly
		symbols, err = s.parser.ParseFile(req.FilePath)
		if err != nil {
			return nil, err
		}
	}

	protoSymbols := make([]*Symbol, len(symbols))
	for i, sym := range symbols {
		protoSymbols[i] = symbolToProto(sym)
	}

	return &CodeSymbolsResponse{
		Symbols: protoSymbols,
	}, nil
}

func (s *codeServiceImpl) Stats(ctx context.Context, req *CodeStatsRequest) (*CodeStatsResponse, error) {
	cs := s.server.GetCodeStore()
	if cs == nil {
		return nil, fmt.Errorf("code store not available")
	}

	stats, err := cs.Stats()
	if err != nil {
		return nil, err
	}

	return &CodeStatsResponse{
		Files:      int32(stats.Files),
		Symbols:    int32(stats.Symbols),
		References: int32(stats.References),
	}, nil
}

func (s *codeServiceImpl) Index(ctx context.Context, req *CodeIndexRequest) (*CodeIndexResponse, error) {
	cs := s.server.GetCodeStore()
	if cs == nil {
		return nil, fmt.Errorf("code store not available")
	}

	paths := req.Paths
	if len(paths) == 0 {
		paths = []string{"."}
	}

	var filesIndexed, symbolsIndexed, filesSkipped int32

	for _, root := range paths {
		err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil // Skip files with errors
			}

			// Skip directories
			if info.IsDir() {
				name := info.Name()
				if name == "node_modules" || name == ".git" || name == "vendor" ||
					name == "__pycache__" || name == ".venv" || name == "dist" ||
					name == "build" || name == ".aide" {
					return filepath.SkipDir
				}
				return nil
			}

			// Check if file is supported (extension or known filename)
			if !code.SupportedFile(path) {
				return nil
			}

			// Get relative path
			relPath := path
			if cwd, err := os.Getwd(); err == nil {
				if rel, err := filepath.Rel(cwd, path); err == nil {
					relPath = rel
				}
			}

			// Check if file needs reindexing
			if !req.Force {
				fileInfo, err := cs.GetFileInfo(relPath)
				if err == nil && fileInfo.ModTime.Equal(info.ModTime()) {
					filesSkipped++
					return nil
				}
			}

			// Parse file
			symbols, err := s.parser.ParseFile(path)
			if err != nil {
				return nil
			}

			// Clear existing symbols
			cs.ClearFile(relPath)

			// Store symbols
			var symbolIDs []string
			for _, sym := range symbols {
				sym.FilePath = relPath
				if err := cs.AddSymbol(sym); err != nil {
					continue
				}
				symbolIDs = append(symbolIDs, sym.ID)
				symbolsIndexed++
			}

			// Update file info
			cs.SetFileInfo(&code.FileInfo{
				Path:      relPath,
				ModTime:   info.ModTime(),
				SymbolIDs: symbolIDs,
			})

			filesIndexed++
			return nil
		})
		if err != nil {
			return nil, err
		}
	}

	return &CodeIndexResponse{
		FilesIndexed:   filesIndexed,
		SymbolsIndexed: symbolsIndexed,
		FilesSkipped:   filesSkipped,
	}, nil
}

func (s *codeServiceImpl) Clear(ctx context.Context, req *CodeClearRequest) (*CodeClearResponse, error) {
	cs := s.server.GetCodeStore()
	if cs == nil {
		return nil, fmt.Errorf("code store not available")
	}

	stats, _ := cs.Stats()
	if err := cs.Clear(); err != nil {
		return nil, err
	}

	var symbolsCleared, filesCleared int32
	if stats != nil {
		symbolsCleared = int32(stats.Symbols)
		filesCleared = int32(stats.Files)
	}

	return &CodeClearResponse{
		SymbolsCleared: symbolsCleared,
		FilesCleared:   filesCleared,
	}, nil
}

// =============================================================================
// Findings Service Implementation
// =============================================================================

type findingsServiceImpl struct {
	UnimplementedFindingsServiceServer
	server *Server
}

func (s *findingsServiceImpl) Add(ctx context.Context, req *FindingAddRequest) (*FindingAddResponse, error) {
	fs := s.server.GetFindingsStore()
	if fs == nil {
		return nil, fmt.Errorf("findings store not available")
	}

	f := &findings.Finding{
		Analyzer: req.Analyzer,
		Severity: req.Severity,
		Category: req.Category,
		FilePath: req.FilePath,
		Line:     int(req.Line),
		EndLine:  int(req.EndLine),
		Title:    req.Title,
		Detail:   req.Detail,
		Metadata: req.Metadata,
	}

	if err := fs.AddFinding(f); err != nil {
		return nil, err
	}

	return &FindingAddResponse{
		Finding: findingToProto(f),
	}, nil
}

func (s *findingsServiceImpl) Get(ctx context.Context, req *FindingGetRequest) (*FindingGetResponse, error) {
	fs := s.server.GetFindingsStore()
	if fs == nil {
		return nil, fmt.Errorf("findings store not available")
	}

	f, err := fs.GetFinding(req.Id)
	if err != nil {
		if err == store.ErrNotFound {
			return &FindingGetResponse{Found: false}, nil
		}
		return nil, err
	}

	return &FindingGetResponse{
		Finding: findingToProto(f),
		Found:   true,
	}, nil
}

func (s *findingsServiceImpl) Delete(ctx context.Context, req *FindingDeleteRequest) (*FindingDeleteResponse, error) {
	fs := s.server.GetFindingsStore()
	if fs == nil {
		return nil, fmt.Errorf("findings store not available")
	}

	if err := fs.DeleteFinding(req.Id); err != nil {
		return nil, err
	}

	return &FindingDeleteResponse{Success: true}, nil
}

func (s *findingsServiceImpl) Search(ctx context.Context, req *FindingSearchRequest) (*FindingSearchResponse, error) {
	fs := s.server.GetFindingsStore()
	if fs == nil {
		return nil, fmt.Errorf("findings store not available")
	}

	opts := findings.SearchOptions{
		Analyzer: req.Analyzer,
		Severity: req.Severity,
		FilePath: req.FilePath,
		Category: req.Category,
		Limit:    int(req.Limit),
	}

	results, err := fs.SearchFindings(req.Query, opts)
	if err != nil {
		return nil, err
	}

	protoFindings := make([]*Finding, len(results))
	for i, r := range results {
		protoFindings[i] = findingToProto(r.Finding)
	}

	return &FindingSearchResponse{Findings: protoFindings}, nil
}

func (s *findingsServiceImpl) List(ctx context.Context, req *FindingListRequest) (*FindingSearchResponse, error) {
	fs := s.server.GetFindingsStore()
	if fs == nil {
		return nil, fmt.Errorf("findings store not available")
	}

	opts := findings.SearchOptions{
		Analyzer: req.Analyzer,
		Severity: req.Severity,
		FilePath: req.FilePath,
		Category: req.Category,
		Limit:    int(req.Limit),
	}

	results, err := fs.ListFindings(opts)
	if err != nil {
		return nil, err
	}

	protoFindings := make([]*Finding, len(results))
	for i, f := range results {
		protoFindings[i] = findingToProto(f)
	}

	return &FindingSearchResponse{Findings: protoFindings}, nil
}

func (s *findingsServiceImpl) GetFileFindings(ctx context.Context, req *FindingFileRequest) (*FindingSearchResponse, error) {
	fs := s.server.GetFindingsStore()
	if fs == nil {
		return nil, fmt.Errorf("findings store not available")
	}

	results, err := fs.GetFileFindings(req.FilePath)
	if err != nil {
		return nil, err
	}

	protoFindings := make([]*Finding, len(results))
	for i, f := range results {
		protoFindings[i] = findingToProto(f)
	}

	return &FindingSearchResponse{Findings: protoFindings}, nil
}

func (s *findingsServiceImpl) ClearAnalyzer(ctx context.Context, req *FindingClearAnalyzerRequest) (*FindingClearAnalyzerResponse, error) {
	fs := s.server.GetFindingsStore()
	if fs == nil {
		return nil, fmt.Errorf("findings store not available")
	}

	count, err := fs.ClearAnalyzer(req.Analyzer)
	if err != nil {
		return nil, err
	}

	return &FindingClearAnalyzerResponse{Count: int32(count)}, nil
}

func (s *findingsServiceImpl) Stats(ctx context.Context, req *FindingStatsRequest) (*FindingStatsResponse, error) {
	fs := s.server.GetFindingsStore()
	if fs == nil {
		return nil, fmt.Errorf("findings store not available")
	}

	stats, err := fs.Stats()
	if err != nil {
		return nil, err
	}

	byAnalyzer := make(map[string]int32, len(stats.ByAnalyzer))
	for k, v := range stats.ByAnalyzer {
		byAnalyzer[k] = int32(v)
	}
	bySeverity := make(map[string]int32, len(stats.BySeverity))
	for k, v := range stats.BySeverity {
		bySeverity[k] = int32(v)
	}

	return &FindingStatsResponse{
		Total:      int32(stats.Total),
		ByAnalyzer: byAnalyzer,
		BySeverity: bySeverity,
	}, nil
}

func (s *findingsServiceImpl) Clear(ctx context.Context, req *FindingClearRequest) (*FindingClearResponse, error) {
	fs := s.server.GetFindingsStore()
	if fs == nil {
		return nil, fmt.Errorf("findings store not available")
	}

	if err := fs.Clear(); err != nil {
		return nil, err
	}

	return &FindingClearResponse{Success: true}, nil
}

// =============================================================================
// Status Service Implementation
// =============================================================================

type statusServiceImpl struct {
	UnimplementedStatusServiceServer
	server *Server
}

func (s *statusServiceImpl) GetStatus(ctx context.Context, req *StatusRequest) (*StatusResponse, error) {
	srv := s.server
	srv.mu.RLock()
	w := srv.watcher
	fr := srv.findingsRunner
	tools := srv.mcpTools
	countFunc := srv.toolCountFunc
	srv.mu.RUnlock()

	// Get tool execution counts
	var toolCounts map[string]int64
	if countFunc != nil {
		toolCounts = countFunc()
	}

	// Populate execution counts on tools
	if toolCounts != nil && len(tools) > 0 {
		for _, t := range tools {
			t.ExecutionCount = toolCounts[t.Name]
		}
	}

	resp := &StatusResponse{
		Version:       version.String(),
		Uptime:        formatHumanDuration(time.Since(srv.startTime)),
		ServerRunning: true,
		McpTools:      tools,
	}

	// Get stores via thread-safe getters
	cs := srv.GetCodeStore()
	fss := srv.GetFindingsStore()

	// Watcher status
	if w != nil {
		stats := w.Stats()
		watcherStatus := &StatusWatcher{
			Enabled:      stats.Enabled,
			Paths:        stats.Paths,
			DirsWatched:  int32(stats.DirsWatched),
			Debounce:     stats.Debounce.String(),
			PendingFiles: int32(stats.PendingFiles),
		}
		if cs != nil {
			watcherStatus.Subscribers = append(watcherStatus.Subscribers, "code-indexer")
		}
		if fr != nil {
			watcherStatus.Subscribers = append(watcherStatus.Subscribers, "findings")
		}
		resp.Watcher = watcherStatus
	}

	// Code indexer status
	if cs != nil {
		stats, err := cs.Stats()
		if err == nil && stats != nil {
			resp.CodeIndexer = &StatusCodeIndexer{
				Available:  true,
				Status:     "idle",
				Symbols:    int32(stats.Symbols),
				References: int32(stats.References),
				Files:      int32(stats.Files),
			}
		} else {
			resp.CodeIndexer = &StatusCodeIndexer{
				Available: true,
				Status:    "error",
			}
		}
	}

	// Findings status
	if fss != nil {
		stats, err := fss.Stats()
		if err == nil && stats != nil {
			findingsStatus := &StatusFindings{
				Available: true,
				Total:     int32(stats.Total),
			}
			byAnalyzer := make(map[string]int32, len(stats.ByAnalyzer))
			for k, v := range stats.ByAnalyzer {
				byAnalyzer[k] = int32(v)
			}
			findingsStatus.ByAnalyzer = byAnalyzer

			bySeverity := make(map[string]int32, len(stats.BySeverity))
			for k, v := range stats.BySeverity {
				bySeverity[k] = int32(v)
			}
			findingsStatus.BySeverity = bySeverity

			if fr != nil {
				runnerStatus := fr.GetStatus()
				analyzers := make(map[string]*StatusAnalyzer, len(runnerStatus))
				for name, as := range runnerStatus {
					lastRun := "never"
					if !as.LastRun.IsZero() {
						lastRun = as.LastRun.Format(time.RFC3339)
					}
					analyzers[name] = &StatusAnalyzer{
						Status:       as.Status,
						Scope:        as.Scope,
						LastRun:      lastRun,
						Findings:     int32(as.Findings),
						LastDuration: as.LastDuration.String(),
					}
				}
				findingsStatus.Analyzers = analyzers
			}

			resp.Findings = findingsStatus
		}
	}

	return resp, nil
}

// formatHumanDuration returns a human-readable duration string like "3m 21s" or "1h 5m".
func formatHumanDuration(d time.Duration) string {
	d = d.Round(time.Second)
	if d < time.Second {
		return "< 1s"
	}

	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	minutes := int(d.Minutes()) % 60
	seconds := int(d.Seconds()) % 60

	var parts []string
	if days > 0 {
		parts = append(parts, fmt.Sprintf("%dd", days))
	}
	if hours > 0 {
		parts = append(parts, fmt.Sprintf("%dh", hours))
	}
	if minutes > 0 {
		parts = append(parts, fmt.Sprintf("%dm", minutes))
	}
	if seconds > 0 && days == 0 { // skip seconds for long durations
		parts = append(parts, fmt.Sprintf("%ds", seconds))
	}
	if len(parts) == 0 {
		return "0s"
	}
	return strings.Join(parts, " ")
}

// =============================================================================
// Conversion helpers
// =============================================================================

func findingToProto(f *findings.Finding) *Finding {
	if f == nil {
		return nil
	}
	return &Finding{
		Id:        f.ID,
		Analyzer:  f.Analyzer,
		Severity:  f.Severity,
		Category:  f.Category,
		FilePath:  f.FilePath,
		Line:      int32(f.Line),
		EndLine:   int32(f.EndLine),
		Title:     f.Title,
		Detail:    f.Detail,
		Metadata:  f.Metadata,
		CreatedAt: timestamppb.New(f.CreatedAt),
	}
}

func symbolToProto(s *code.Symbol) *Symbol {
	if s == nil {
		return nil
	}
	return &Symbol{
		Id:         s.ID,
		Name:       s.Name,
		Kind:       s.Kind,
		Signature:  s.Signature,
		DocComment: s.DocComment,
		FilePath:   s.FilePath,
		StartLine:  int32(s.StartLine),
		EndLine:    int32(s.EndLine),
		Language:   s.Language,
		CreatedAt:  timestamppb.New(s.CreatedAt),
	}
}

func memoryToProto(m *memory.Memory) *Memory {
	if m == nil {
		return nil
	}
	pm := &Memory{
		Id:          m.ID,
		Category:    string(m.Category),
		Content:     m.Content,
		Tags:        m.Tags,
		Priority:    m.Priority,
		Plan:        m.Plan,
		Agent:       m.Agent,
		Namespace:   m.Namespace,
		AccessCount: m.AccessCount,
		CreatedAt:   timestamppb.New(m.CreatedAt),
		UpdatedAt:   timestamppb.New(m.UpdatedAt),
	}
	if !m.LastAccessed.IsZero() {
		pm.LastAccessed = timestamppb.New(m.LastAccessed)
	}
	return pm
}

func stateToProto(s *memory.State) *State {
	if s == nil {
		return nil
	}
	return &State{
		Key:       s.Key,
		Value:     s.Value,
		Agent:     s.Agent,
		UpdatedAt: timestamppb.New(s.UpdatedAt),
	}
}

func decisionToProto(d *memory.Decision) *Decision {
	if d == nil {
		return nil
	}
	return &Decision{
		Topic:      d.Topic,
		Decision:   d.Decision,
		Rationale:  d.Rationale,
		Details:    d.Details,
		References: d.References,
		DecidedBy:  d.DecidedBy,
		CreatedAt:  timestamppb.New(d.CreatedAt),
	}
}

func messageToProto(m *memory.Message) *Message {
	if m == nil {
		return nil
	}
	return &Message{
		Id:        m.ID,
		From:      m.From,
		To:        m.To,
		Content:   m.Content,
		Type:      m.Type,
		ReadBy:    m.ReadBy,
		CreatedAt: timestamppb.New(m.CreatedAt),
		ExpiresAt: timestamppb.New(m.ExpiresAt),
	}
}

func taskToProto(t *memory.Task) *Task {
	if t == nil {
		return nil
	}
	return &Task{
		Id:          t.ID,
		Title:       t.Title,
		Description: t.Description,
		Status:      string(t.Status),
		ClaimedBy:   t.ClaimedBy,
		Worktree:    t.Worktree,
		Result:      t.Result,
		CreatedAt:   timestamppb.New(t.CreatedAt),
		ClaimedAt:   timestamppb.New(t.ClaimedAt),
		CompletedAt: timestamppb.New(t.CompletedAt),
	}
}
