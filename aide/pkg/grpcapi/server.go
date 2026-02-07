// Package grpcapi provides the gRPC server implementation for aide.
package grpcapi

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"time"

	"github.com/jmylchreest/aide/aide/internal/version"
	"github.com/jmylchreest/aide/aide/pkg/code"
	"github.com/jmylchreest/aide/aide/pkg/memory"
	"github.com/jmylchreest/aide/aide/pkg/store"
	"github.com/oklog/ulid/v2"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// generateID creates a new ULID for use as an entity ID.
func generateID() string {
	return ulid.Make().String()
}

// DefaultSocketPath returns the default Unix socket path.
func DefaultSocketPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".aide", "aide.sock")
}

// Server manages the gRPC server and all service implementations.
type Server struct {
	store      store.Store
	codeStore  store.CodeIndexStore
	dbPath     string
	grpcServer *grpc.Server
	socketPath string
}

// NewServer creates a new gRPC server.
func NewServer(st store.Store, dbPath, socketPath string) *Server {
	return &Server{
		store:      st,
		dbPath:     dbPath,
		socketPath: socketPath,
	}
}

// SetCodeStore sets the code store for code indexing services.
func (s *Server) SetCodeStore(cs store.CodeIndexStore) {
	s.codeStore = cs
}

// Start starts the gRPC server on a Unix socket.
func (s *Server) Start() error {
	// Ensure socket directory exists
	socketDir := filepath.Dir(s.socketPath)
	if err := os.MkdirAll(socketDir, 0o755); err != nil {
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
	RegisterCodeServiceServer(s.grpcServer, &codeServiceImpl{store: s.codeStore, parser: code.NewParser()})
	RegisterHealthServiceServer(s.grpcServer, &healthServiceImpl{dbPath: s.dbPath})

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
		return &TaskCompleteResponse{
			Success: false,
		}, nil
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
		return &TaskUpdateResponse{Success: false}, nil
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
		return &TaskUpdateResponse{Success: false}, nil
	}

	return &TaskUpdateResponse{
		Task:    taskToProto(task),
		Success: true,
	}, nil
}

// =============================================================================
// Code Service Implementation
// =============================================================================

type codeServiceImpl struct {
	UnimplementedCodeServiceServer
	store  store.CodeIndexStore
	parser *code.Parser
}

func (s *codeServiceImpl) Search(ctx context.Context, req *CodeSearchRequest) (*CodeSearchResponse, error) {
	if s.store == nil {
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

	results, err := s.store.SearchSymbols(req.Query, opts)
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
	if s.store == nil {
		return nil, fmt.Errorf("code store not available")
	}

	symbols, err := s.store.GetFileSymbols(req.FilePath)
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
	if s.store == nil {
		return nil, fmt.Errorf("code store not available")
	}

	stats, err := s.store.Stats()
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
	if s.store == nil {
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

			// Check if file extension is supported
			ext := filepath.Ext(path)
			if !code.SupportedExtension(ext) {
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
				fileInfo, err := s.store.GetFileInfo(relPath)
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
			s.store.ClearFile(relPath)

			// Store symbols
			var symbolIDs []string
			for _, sym := range symbols {
				sym.FilePath = relPath
				if err := s.store.AddSymbol(sym); err != nil {
					continue
				}
				symbolIDs = append(symbolIDs, sym.ID)
				symbolsIndexed++
			}

			// Update file info
			s.store.SetFileInfo(&code.FileInfo{
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
	if s.store == nil {
		return nil, fmt.Errorf("code store not available")
	}

	stats, _ := s.store.Stats()
	if err := s.store.Clear(); err != nil {
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
// Conversion helpers
// =============================================================================

func symbolToProto(s *code.Symbol) *Symbol {
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
	return &Memory{
		Id:        m.ID,
		Category:  string(m.Category),
		Content:   m.Content,
		Tags:      m.Tags,
		Priority:  m.Priority,
		Plan:      m.Plan,
		Agent:     m.Agent,
		Namespace: m.Namespace,
		CreatedAt: timestamppb.New(m.CreatedAt),
		UpdatedAt: timestamppb.New(m.UpdatedAt),
	}
}

func stateToProto(s *memory.State) *State {
	return &State{
		Key:       s.Key,
		Value:     s.Value,
		Agent:     s.Agent,
		UpdatedAt: timestamppb.New(s.UpdatedAt),
	}
}

func decisionToProto(d *memory.Decision) *Decision {
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
