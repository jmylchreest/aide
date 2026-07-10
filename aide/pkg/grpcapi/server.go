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
	"github.com/jmylchreest/aide/aide/pkg/aideignore"
	"github.com/jmylchreest/aide/aide/pkg/code"
	"github.com/jmylchreest/aide/aide/pkg/config"
	"github.com/jmylchreest/aide/aide/pkg/eventbus"
	"github.com/jmylchreest/aide/aide/pkg/findings"
	"github.com/jmylchreest/aide/aide/pkg/grammar"
	"github.com/jmylchreest/aide/aide/pkg/instinct"
	"github.com/jmylchreest/aide/aide/pkg/memory"
	"github.com/jmylchreest/aide/aide/pkg/observe"
	"github.com/jmylchreest/aide/aide/pkg/store"
	"github.com/jmylchreest/aide/aide/pkg/survey"
	"github.com/jmylchreest/aide/aide/pkg/surveyrun"
	"github.com/jmylchreest/aide/aide/pkg/watcher"
	"github.com/oklog/ulid/v2"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// SocketPathFromDB returns the Unix socket path derived from the database path.
// The socket is placed in the project's .aide directory (sibling to the memory dir).
func SocketPathFromDB(dbPath string) string {
	// dbPath is typically <project>/.aide/memory/memory.db
	// We want <project>/.aide/aide.sock
	aideDir := filepath.Dir(filepath.Dir(dbPath))
	return filepath.Join(aideDir, "aide.sock")
}

// projectRoot derives the project root from the database path.
// dbPath is <root>/.aide/memory/memory.db — three Dir() calls to reach <root>.
func projectRoot(dbPath string) string {
	return filepath.Dir(filepath.Dir(filepath.Dir(dbPath)))
}

// Server manages the gRPC server and all service implementations.
type Server struct {
	store         store.Store
	instinctStore store.InstinctProposalStore
	codeStore     store.CodeIndexStore
	findingsStore store.FindingsStore
	surveyStore   store.SurveyStore
	observeBus    *eventbus.Broadcaster[*observe.Event]
	instinctBus   *eventbus.Broadcaster[*instinct.Proposal]
	taskBus       *eventbus.Broadcaster[*memory.Task]
	messageBus    *eventbus.Broadcaster[*memory.Message]
	stateBus      *eventbus.Broadcaster[*StateChange]
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
	pprofURLFunc   func() string

	// codeReconciler, when non-nil, is invoked before project-wide code
	// analyzers run so the analyzer doesn't operate on stale index entries.
	// Returns (removed, refreshed, err). Set by the daemon when the file
	// watcher is enabled; nil otherwise.
	codeReconciler func() (int, int, error)
}

// NewServer creates a new gRPC server.
func NewServer(st store.Store, dbPath, socketPath string, loader grammar.Loader) *Server {
	return &Server{
		store:         st,
		observeBus:    eventbus.New[*observe.Event](256),
		instinctBus:   eventbus.New[*instinct.Proposal](64),
		taskBus:       eventbus.New[*memory.Task](64),
		messageBus:    eventbus.New[*memory.Message](128),
		stateBus:      eventbus.New[*StateChange](128),
		dbPath:        dbPath,
		socketPath:    socketPath,
		startTime:     time.Now(),
		grammarLoader: loader,
	}
}

// TaskBus returns the swarm task broadcaster for live streaming. It, with
// MessageBus and StateBus, is used by SwarmService.Watch* and by service
// handlers that publish on writes.
func (s *Server) TaskBus() *eventbus.Broadcaster[*memory.Task] { return s.taskBus }
func (s *Server) MessageBus() *eventbus.Broadcaster[*memory.Message] {
	return s.messageBus
}
func (s *Server) StateBus() *eventbus.Broadcaster[*StateChange] { return s.stateBus }

// SetInstinctStore attaches the instinct proposal store. Without it the
// InstinctService returns FailedPrecondition.
func (s *Server) SetInstinctStore(ps store.InstinctProposalStore) {
	s.storeMu.Lock()
	defer s.storeMu.Unlock()
	s.instinctStore = ps
}

// GetInstinctStore returns the current instinct proposal store.
func (s *Server) GetInstinctStore() store.InstinctProposalStore {
	s.storeMu.RLock()
	defer s.storeMu.RUnlock()
	return s.instinctStore
}

// InstinctBus exposes the proposal broadcaster.
func (s *Server) InstinctBus() *eventbus.Broadcaster[*instinct.Proposal] {
	return s.instinctBus
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

// ObserveBus returns the broadcaster used by WatchEvents. Always non-nil
// for a server constructed via NewServer.
func (s *Server) ObserveBus() *eventbus.Broadcaster[*observe.Event] {
	return s.observeBus
}

// SetSurveyStore sets the survey store for survey services.
func (s *Server) SetSurveyStore(ss store.SurveyStore) {
	s.storeMu.Lock()
	defer s.storeMu.Unlock()
	s.surveyStore = ss
}

// GetSurveyStore returns the current survey store (thread-safe).
func (s *Server) GetSurveyStore() store.SurveyStore {
	s.storeMu.RLock()
	defer s.storeMu.RUnlock()
	return s.surveyStore
}

// GetTombstoneStore returns the tombstone surface backed by the main store.
// Unlike findings/survey there is no separate setter: tombstones live in the
// same BoltDB as memories/decisions (CombinedStore and BoltStore both satisfy
// store.TombstoneStore), so we type-assert s.store. Returns nil if the store
// somehow does not implement the interface.
func (s *Server) GetTombstoneStore() store.TombstoneStore {
	ts, _ := s.store.(store.TombstoneStore)
	return ts
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

// SetCodeReconciler installs the reconcile callback used to refresh the code
// index before project-wide analyzers run. Pass nil to disable.
func (s *Server) SetCodeReconciler(fn func() (int, int, error)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.codeReconciler = fn
}

func (s *Server) reconcileCode() (int, int, error) {
	s.mu.RLock()
	fn := s.codeReconciler
	s.mu.RUnlock()
	if fn == nil {
		return 0, 0, nil
	}
	return fn()
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

// SetPprofURLFunc sets the function used to retrieve the pprof server URL.
func (s *Server) SetPprofURLFunc(f func() string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pprofURLFunc = f
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
	RegisterStateServiceServer(s.grpcServer, &stateServiceImpl{store: s.store, server: s})
	RegisterDecisionServiceServer(s.grpcServer, &decisionServiceImpl{store: s.store})
	RegisterMessageServiceServer(s.grpcServer, &messageServiceImpl{store: s.store, server: s})
	RegisterTaskServiceServer(s.grpcServer, &taskServiceImpl{store: s.store, server: s})
	RegisterCodeServiceServer(s.grpcServer, &codeServiceImpl{server: s, parser: code.NewParser(s.grammarLoader)})
	RegisterFindingsServiceServer(s.grpcServer, &findingsServiceImpl{server: s})
	RegisterSurveyServiceServer(s.grpcServer, &surveyServiceImpl{server: s})
	RegisterTombstoneServiceServer(s.grpcServer, &tombstoneServiceImpl{server: s})
	RegisterTokenServiceServer(s.grpcServer, &tokenServiceImpl{store: s.store})
	RegisterObserveServiceServer(s.grpcServer, &observeServiceImpl{store: s.store, bus: s.observeBus})
	RegisterInstinctServiceServer(s.grpcServer, &instinctServiceImpl{server: s})
	RegisterSwarmServiceServer(s.grpcServer, &swarmServiceImpl{server: s})
	RegisterHealthServiceServer(s.grpcServer, &healthServiceImpl{dbPath: s.dbPath, startTime: s.startTime})
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
	dbPath    string
	startTime time.Time
}

func (s *healthServiceImpl) Check(ctx context.Context, req *HealthCheckRequest) (*HealthCheckResponse, error) {
	info := version.GetInfo()
	return &HealthCheckResponse{
		Healthy:       true,
		Version:       info.Version,
		DbPath:        s.dbPath,
		Commit:        info.Commit,
		BuildDate:     info.Date,
		Pid:           int64(os.Getpid()),
		StartedAtUnix: s.startTime.Unix(),
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
		ID:       req.Id,
		Content:  req.Content,
		Category: memory.Category(req.Category),
		Tags:     req.Tags,
	}
	// Honour caller-supplied timestamps when present. BoltStore.AddMemory
	// fills zero values with sensible defaults, so leaving them unset keeps
	// the existing behaviour for non-identity-preserving callers.
	if req.CreatedAt != nil {
		mem.CreatedAt = req.CreatedAt.AsTime()
	}
	if req.UpdatedAt != nil {
		mem.UpdatedAt = req.UpdatedAt.AsTime()
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
	store  store.StateStore
	server *Server
}

func (s *stateServiceImpl) publish(st *memory.State, change string) {
	if s.server == nil {
		return
	}
	if bus := s.server.StateBus(); bus != nil {
		bus.Publish(&StateChange{State: stateToProto(st), Change: change})
	}
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
	if req.UpdatedAt != nil {
		st.UpdatedAt = req.UpdatedAt.AsTime()
	}

	if err := s.store.SetState(st); err != nil {
		return nil, err
	}

	s.publish(st, "set")

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

	s.publish(&memory.State{Key: req.Key}, "delete")

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
	}
	// Honour caller-supplied CreatedAt; BoltStore.SetDecision stamps time.Now()
	// when zero, matching the existing default.
	if req.CreatedAt != nil {
		dec.CreatedAt = req.CreatedAt.AsTime()
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
	store  store.MessageStore
	server *Server
}

func (s *messageServiceImpl) Send(ctx context.Context, req *MessageSendRequest) (*MessageSendResponse, error) {
	ttl := int(req.TtlSeconds)
	if ttl == 0 {
		ttl = 3600
	}

	msg := &memory.Message{
		From:            req.From,
		To:              req.To,
		Content:         req.Content,
		Type:            req.Type,
		Priority:        req.Priority,
		ParentSessionID: req.ParentSessionId,
		CreatedAt:       time.Now(),
		ExpiresAt:       time.Now().Add(time.Duration(ttl) * time.Second),
	}

	if err := s.store.AddMessage(msg); err != nil {
		return nil, err
	}

	if s.server != nil {
		if bus := s.server.MessageBus(); bus != nil {
			bus.Publish(msg)
		}
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

	if req.ParentSessionId != "" {
		filtered := messages[:0]
		for _, m := range messages {
			if m.ParentSessionID == req.ParentSessionId {
				filtered = append(filtered, m)
			}
		}
		messages = filtered
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
	store  store.TaskStore
	server *Server
}

func (s *taskServiceImpl) publishTask(t *memory.Task) {
	if s.server == nil || t == nil {
		return
	}
	if bus := s.server.TaskBus(); bus != nil {
		bus.Publish(t)
	}
}

func (s *taskServiceImpl) Create(ctx context.Context, req *TaskCreateRequest) (*TaskCreateResponse, error) {
	task := &memory.Task{
		Title:           req.Title,
		Description:     req.Description,
		ParentSessionID: req.ParentSessionId,
		Status:          memory.TaskStatusPending,
	}

	if err := s.store.CreateTask(task); err != nil {
		return nil, err
	}

	s.publishTask(task)

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

	if req.ParentSessionId != "" {
		filtered := tasks[:0]
		for _, t := range tasks {
			if t.ParentSessionID == req.ParentSessionId {
				filtered = append(filtered, t)
			}
		}
		tasks = filtered
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

	s.publishTask(task)

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
	s.publishTask(task)
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

	s.publishTask(task)

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
// Swarm Service Implementation
// =============================================================================

type swarmServiceImpl struct {
	UnimplementedSwarmServiceServer
	server *Server
}

func (s *swarmServiceImpl) WatchTasks(req *SwarmWatchTasksRequest, stream SwarmService_WatchTasksServer) error {
	bus := s.server.TaskBus()
	if bus == nil {
		return status.Error(codes.FailedPrecondition, "task broadcaster not configured")
	}
	ctx := stream.Context()

	matches := func(t *memory.Task) bool {
		if t == nil {
			return false
		}
		if req.ParentSessionId != "" && t.ParentSessionID != req.ParentSessionId {
			return false
		}
		if req.Status != "" && string(t.Status) != req.Status {
			return false
		}
		return true
	}

	sub, unsub := bus.Subscribe(ctx, matches)
	defer unsub()

	if tasks, err := s.server.store.ListTasks(memory.TaskStatus(req.Status)); err == nil {
		for _, t := range tasks {
			if !matches(t) {
				continue
			}
			if err := stream.Send(taskToProto(t)); err != nil {
				return err
			}
		}
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case t, ok := <-sub:
			if !ok {
				return nil
			}
			if err := stream.Send(taskToProto(t)); err != nil {
				return err
			}
		}
	}
}

func (s *swarmServiceImpl) WatchMessages(req *SwarmWatchMessagesRequest, stream SwarmService_WatchMessagesServer) error {
	bus := s.server.MessageBus()
	if bus == nil {
		return status.Error(codes.FailedPrecondition, "message broadcaster not configured")
	}
	ctx := stream.Context()

	matches := func(m *memory.Message) bool {
		if m == nil {
			return false
		}
		if req.ParentSessionId != "" && m.ParentSessionID != req.ParentSessionId {
			return false
		}
		if req.AgentId != "" && m.To != req.AgentId && m.From != req.AgentId {
			return false
		}
		if req.Priority != "" && !strings.EqualFold(m.Priority, req.Priority) {
			return false
		}
		return true
	}

	sub, unsub := bus.Subscribe(ctx, matches)
	defer unsub()

	// Backfill recent messages addressed to the agent (if any).
	if msgs, err := s.server.store.GetMessages(req.AgentId); err == nil {
		for _, m := range msgs {
			if !matches(m) {
				continue
			}
			if err := stream.Send(messageToProto(m)); err != nil {
				return err
			}
		}
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case m, ok := <-sub:
			if !ok {
				return nil
			}
			if err := stream.Send(messageToProto(m)); err != nil {
				return err
			}
		}
	}
}

func (s *swarmServiceImpl) WatchState(req *SwarmWatchStateRequest, stream SwarmService_WatchStateServer) error {
	bus := s.server.StateBus()
	if bus == nil {
		return status.Error(codes.FailedPrecondition, "state broadcaster not configured")
	}
	ctx := stream.Context()

	matches := func(c *StateChange) bool {
		if c == nil || c.State == nil {
			return false
		}
		if req.AgentId != "" && c.State.Agent != req.AgentId {
			return false
		}
		if req.KeyPrefix != "" && !strings.HasPrefix(c.State.Key, req.KeyPrefix) {
			return false
		}
		return true
	}

	sub, unsub := bus.Subscribe(ctx, matches)
	defer unsub()

	// Backfill: list current state for the agent so the client renders
	// initial values, then transition to live.
	if states, err := s.server.store.ListState(req.AgentId); err == nil {
		for _, st := range states {
			change := &StateChange{State: stateToProto(st), Change: "set"}
			if !matches(change) {
				continue
			}
			if err := stream.Send(change); err != nil {
				return err
			}
		}
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case c, ok := <-sub:
			if !ok {
				return nil
			}
			if err := stream.Send(c); err != nil {
				return err
			}
		}
	}
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

func (s *codeServiceImpl) GetFileInfo(ctx context.Context, req *CodeGetFileInfoRequest) (*CodeGetFileInfoResponse, error) {
	cs := s.server.GetCodeStore()
	if cs == nil {
		return nil, fmt.Errorf("code store not available")
	}
	fi, err := cs.GetFileInfo(req.Path)
	if err != nil || fi == nil {
		return &CodeGetFileInfoResponse{Found: false}, nil
	}
	return &CodeGetFileInfoResponse{
		Found:     true,
		ModTime:   timestamppb.New(fi.ModTime),
		SymbolIds: fi.SymbolIDs,
		Tokens:    int32(fi.Tokens),
		SizeBytes: fi.SizeBytes,
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

// indexParseWork is what the walker hands to a parser worker — the absolute
// path to read, the project-relative path to record, and the FileInfo (carries
// mtime + size) so the parser doesn't re-stat.
type indexParseWork struct {
	abs  string
	rel  string
	info os.FileInfo
}

// indexResult travels from a producer (walker for skipped files; parser
// worker for parsed files) to the single writer goroutine. The writer is the
// only thing allowed to touch the gRPC stream and the bbolt write tx, so all
// emit-and-persist work funnels through it in result-arrival order.
type indexResult struct {
	rel       string
	skipped   bool
	symbols   []*code.Symbol
	refs      []*code.Reference
	mtime     time.Time
	sizeBytes int64
}

// Index walks the requested paths and indexes every file the supported-files
// filter accepts. Tree-sitter parsing is the hot CPU cost (per pprof on the
// Linux kernel) and is pure & per-file, so we fan it out across N parser
// workers (N = AIDE_INDEX_WORKERS, defaulting to runtime.NumCPU()). The
// bbolt write tx is exclusive by design, so a single writer goroutine
// serialises IndexFileBatch and stream.Send.
//
// Pipeline shape:
//
//	walker ──► parseQueue ──► N parsers ──► resultQueue ──► writer ──► stream
//
// Cancellation: a derived ctx is cancelled by the writer on stream-send error
// so the walker and parsers stop pushing into now-orphaned channels; ctx is
// also tripped by stream.Context() (Ctrl-C / client disconnect / deadline).
//
// Progress event ordering changes vs the old single-threaded path: events
// arrive in completion order (small files first) rather than walk order.
// That tracks real progress more accurately and is the documented contract
// for the streaming RPC.
//
//nolint:gocyclo // pipeline orchestration: path validation, walker, parser fan-out, writer, and three cancellation paths all live here by design.
func (s *codeServiceImpl) Index(req *CodeIndexRequest, stream grpc.ServerStreamingServer[CodeIndexEvent]) error {
	cs := s.server.GetCodeStore()
	if cs == nil {
		return fmt.Errorf("code store not available")
	}

	ctx, cancel := context.WithCancel(stream.Context())
	defer cancel()

	paths := req.Paths
	if len(paths) == 0 {
		paths = []string{"."}
	}

	projRoot := projectRoot(s.server.dbPath)
	rootPrefix := projRoot + string(filepath.Separator)
	for _, p := range paths {
		abs, err := filepath.Abs(p)
		if err != nil {
			return fmt.Errorf("invalid path %q: %w", p, err)
		}
		if abs != projRoot && !strings.HasPrefix(abs, rootPrefix) {
			return fmt.Errorf("path %q is outside the project directory", p)
		}
	}

	ignore, err := aideignore.New(projRoot)
	if err != nil {
		ignore = aideignore.NewFromDefaults()
	}
	shouldSkip := ignore.WalkFunc(projRoot)

	workers := config.Get().IndexWorkerCount()
	parseQueue := make(chan indexParseWork, workers*2)
	resultQueue := make(chan indexResult, workers*2)

	// Parser workers: pull work off parseQueue, run tree-sitter in parallel,
	// push results to the writer.
	var parserWG sync.WaitGroup
	for i := 0; i < workers; i++ {
		parserWG.Add(1)
		go func() {
			defer parserWG.Done()
			for w := range parseQueue {
				if ctx.Err() != nil {
					return
				}
				symbols, err := s.parser.ParseFile(w.abs)
				if err != nil {
					continue
				}
				refs, _ := s.parser.ParseFileReferences(w.abs)
				select {
				case resultQueue <- indexResult{
					rel:       w.rel,
					symbols:   symbols,
					refs:      refs,
					mtime:     w.info.ModTime(),
					sizeBytes: w.info.Size(),
				}:
				case <-ctx.Done():
					return
				}
			}
		}()
	}

	// Close resultQueue once every parser has finished draining parseQueue.
	go func() {
		parserWG.Wait()
		close(resultQueue)
	}()

	// Writer: the only goroutine that calls stream.Send or IndexFileBatch.
	// Per-file bbolt commit + Bleve batch happen here; counters stay
	// goroutine-local so atomics aren't needed.
	var (
		filesIndexed, symbolsIndexed, filesSkipped int32
		writerErr                                  error
	)
	writerDone := make(chan struct{})
	go func() {
		defer close(writerDone)
		for r := range resultQueue {
			if ctx.Err() != nil {
				continue // drain queue but stop emitting
			}
			if r.skipped {
				filesSkipped++
				if err := sendProgress(stream, &CodeIndexProgress{
					Path:         r.rel,
					FilesDone:    filesIndexed,
					FilesSkipped: filesSkipped,
					Skipped:      true,
				}); err != nil {
					writerErr = err
					cancel()
					continue
				}
				continue
			}
			if err := cs.IndexFileBatch(r.rel, r.symbols, r.refs, r.mtime, r.sizeBytes); err != nil {
				continue
			}
			filesIndexed++
			fileSymbols := int32(len(r.symbols))
			symbolsIndexed += fileSymbols
			if err := sendProgress(stream, &CodeIndexProgress{
				Path:         r.rel,
				Symbols:      fileSymbols,
				FilesDone:    filesIndexed,
				FilesSkipped: filesSkipped,
			}); err != nil {
				writerErr = err
				cancel()
				continue
			}
		}
	}()

	// Walker: enumerate files, decide skip-vs-parse, push to either
	// resultQueue (skipped) or parseQueue (needs parsing). Runs in this
	// goroutine because filepath.Walk is fundamentally serial.
	var walkErr error
	for _, root := range paths {
		err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
			if cerr := ctx.Err(); cerr != nil {
				return cerr
			}
			if err != nil {
				return nil
			}
			if skip, skipDir := shouldSkip(path, info); skip {
				if skipDir {
					return filepath.SkipDir
				}
				return nil
			}
			if info.IsDir() {
				return nil
			}
			if !code.SupportedFile(path) {
				return nil
			}
			relPath := path
			if rel, err := filepath.Rel(projRoot, path); err == nil {
				relPath = rel
			}

			// Incremental mode: skip if mtime matches the indexed version.
			// Done in the walker (one cheap bbolt View) so the parsers
			// don't waste cycles on tree-sitter for unchanged files.
			if !req.Force {
				if existing, err := cs.GetFileInfo(relPath); err == nil && existing.ModTime.Equal(info.ModTime()) {
					select {
					case resultQueue <- indexResult{rel: relPath, skipped: true}:
					case <-ctx.Done():
						return ctx.Err()
					}
					return nil
				}
			}

			select {
			case parseQueue <- indexParseWork{abs: path, rel: relPath, info: info}:
			case <-ctx.Done():
				return ctx.Err()
			}
			return nil
		})
		if err != nil {
			if ctx.Err() == nil {
				walkErr = err
			}
			break
		}
	}
	close(parseQueue)

	<-writerDone

	if walkErr != nil {
		return walkErr
	}
	if writerErr != nil {
		return writerErr
	}
	if cerr := stream.Context().Err(); cerr != nil {
		return cerr
	}

	return stream.Send(&CodeIndexEvent{
		Event: &CodeIndexEvent_Summary{Summary: &CodeIndexResponse{
			FilesIndexed:   filesIndexed,
			SymbolsIndexed: symbolsIndexed,
			FilesSkipped:   filesSkipped,
		}},
	})
}

// sendProgress wraps a CodeIndexProgress in the event envelope and writes it
// to the stream. Returned errors abort the walk.
func sendProgress(stream grpc.ServerStreamingServer[CodeIndexEvent], p *CodeIndexProgress) error {
	return stream.Send(&CodeIndexEvent{Event: &CodeIndexEvent_Progress{Progress: p}})
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

func (s *codeServiceImpl) TopReferences(ctx context.Context, req *CodeTopReferencesRequest) (*CodeTopReferencesResponse, error) {
	cs := s.server.GetCodeStore()
	if cs == nil {
		return nil, fmt.Errorf("code store not available")
	}

	results, err := cs.TopReferencedSymbols(int(req.Limit), req.Kind)
	if err != nil {
		return nil, err
	}

	protoResults := make([]*SymbolRefCount, len(results))
	for i, r := range results {
		protoResults[i] = &SymbolRefCount{
			Symbol: r.Symbol,
			Count:  int32(r.Count),
			Kind:   r.Kind,
			File:   r.File,
		}
	}

	return &CodeTopReferencesResponse{
		Symbols: protoResults,
	}, nil
}

func (s *codeServiceImpl) SearchReferences(ctx context.Context, req *CodeSearchReferencesRequest) (*CodeSearchReferencesResponse, error) {
	cs := s.server.GetCodeStore()
	if cs == nil {
		return nil, fmt.Errorf("code store not available")
	}

	limit := int(req.Limit)
	if limit == 0 {
		limit = 50
	}

	opts := code.ReferenceSearchOptions{
		SymbolName: req.SymbolName,
		Kind:       req.Kind,
		FilePath:   req.FilePath,
		Limit:      limit,
	}

	results, err := cs.SearchReferences(opts)
	if err != nil {
		return nil, err
	}

	protoRefs := make([]*CodeReference, len(results))
	for i, r := range results {
		protoRefs[i] = referenceToProto(r)
	}

	return &CodeSearchReferencesResponse{
		References: protoRefs,
	}, nil
}

func (s *codeServiceImpl) GetFileReferences(ctx context.Context, req *CodeGetFileReferencesRequest) (*CodeSearchReferencesResponse, error) {
	cs := s.server.GetCodeStore()
	if cs == nil {
		return nil, fmt.Errorf("code store not available")
	}

	refs, err := cs.GetFileReferences(req.FilePath)
	if err != nil {
		return nil, err
	}

	protoRefs := make([]*CodeReference, len(refs))
	for i, r := range refs {
		protoRefs[i] = referenceToProto(r)
	}

	return &CodeSearchReferencesResponse{
		References: protoRefs,
	}, nil
}

func (s *codeServiceImpl) GetContainingSymbol(ctx context.Context, req *CodeGetContainingSymbolRequest) (*CodeGetContainingSymbolResponse, error) {
	cs := s.server.GetCodeStore()
	if cs == nil {
		return nil, fmt.Errorf("code store not available")
	}

	sym, err := cs.GetContainingSymbol(req.FilePath, int(req.Line))
	if err != nil {
		// ErrNotFound means no symbol contains this line — return found=false.
		return &CodeGetContainingSymbolResponse{Found: false}, nil
	}

	return &CodeGetContainingSymbolResponse{
		Symbol: symbolToProto(sym),
		Found:  true,
	}, nil
}

func (s *codeServiceImpl) ReadCheck(ctx context.Context, req *CodeReadCheckRequest) (*CodeReadCheckResponse, error) {
	cs := s.server.GetCodeStore()
	if cs == nil {
		return &CodeReadCheckResponse{}, nil
	}

	filePath := req.FilePath
	root := projectRoot(s.server.dbPath)

	// Resolve to absolute path for os.Stat
	absPath := filePath
	if !filepath.IsAbs(filePath) {
		absPath = filepath.Join(root, filePath)
	}

	// Resolve to relative path for store lookup
	relPath := filePath
	if filepath.IsAbs(filePath) {
		if rel, err := filepath.Rel(root, filePath); err == nil {
			relPath = rel
		}
	}

	fileInfo, err := cs.GetFileInfo(relPath)
	if err != nil {
		return &CodeReadCheckResponse{}, nil
	}

	stat, err := os.Stat(absPath)
	if err != nil {
		return &CodeReadCheckResponse{
			Indexed:         true,
			Symbols:         int32(len(fileInfo.SymbolIDs)),
			EstimatedTokens: int32(fileInfo.Tokens),
		}, nil
	}

	fresh := fileInfo.ModTime.Equal(stat.ModTime())
	symbolCount := int32(len(fileInfo.SymbolIDs))
	tokens := int32(fileInfo.Tokens)

	// If tokens weren't stored at index time, estimate from current size
	if tokens == 0 && stat.Size() > 0 {
		tokens = int32(code.EstimateTokensFromSize(relPath, stat.Size()))
	}

	return &CodeReadCheckResponse{
		Indexed:          true,
		Fresh:            fresh,
		Symbols:          symbolCount,
		OutlineAvailable: symbolCount > 0,
		EstimatedTokens:  tokens,
	}, nil
}

func (s *codeServiceImpl) RunDeadCodeAnalysis(ctx context.Context, req *CodeRunDeadCodeAnalysisRequest) (*CodeRunDeadCodeAnalysisResponse, error) {
	cs := s.server.GetCodeStore()
	if cs == nil {
		return nil, fmt.Errorf("code store not available")
	}
	fs := s.server.GetFindingsStore()
	if fs == nil {
		return nil, fmt.Errorf("findings store not available")
	}

	if removed, refreshed, err := s.server.reconcileCode(); err != nil {
		return nil, fmt.Errorf("reconcile code index: %w", err)
	} else if removed > 0 || refreshed > 0 {
		fmt.Printf("reconcile before deadcode: removed %d, refreshed %d\n", removed, refreshed)
	}

	stats, err := cs.Stats()
	if err != nil {
		return nil, fmt.Errorf("failed to read code stats: %w", err)
	}
	if stats.Symbols == 0 {
		return nil, fmt.Errorf("code index is empty — run 'aide code index' first")
	}

	cfg := findings.DeadCodeConfig{
		GetAllSymbols: func() ([]*code.Symbol, error) {
			return cs.ListAllSymbols(-1)
		},
		GetRefCount: func(name string) (int, error) {
			refs, err := cs.SearchReferences(code.ReferenceSearchOptions{
				SymbolName: name,
				Limit:      1,
			})
			if err != nil {
				return 0, err
			}
			return len(refs), nil
		},
		ProjectRoot:        projectRoot(s.server.dbPath),
		PackProvider:       grammar.DefaultPackRegistry().Get,
		IncludeExported:    req.IncludeExported,
		ConsumerExtensions: grammar.DefaultPackRegistry().ConsumerExtensions(),
	}

	ff, result, err := findings.AnalyzeDeadCode(cfg)
	if err != nil {
		return nil, err
	}

	if err := fs.ReplaceFindingsForAnalyzer(findings.AnalyzerDeadCode, ff); err != nil {
		return nil, fmt.Errorf("failed to store findings: %w", err)
	}

	return &CodeRunDeadCodeAnalysisResponse{
		SymbolsChecked: int32(result.SymbolsChecked),
		SymbolsSkipped: int32(result.SymbolsSkipped),
		FindingsCount:  int32(result.FindingsCount),
		DurationMs:     result.Duration.Milliseconds(),
	}, nil
}

// =============================================================================
// Token Service Implementation
// =============================================================================

type tokenServiceImpl struct {
	UnimplementedTokenServiceServer
	store store.Store
}

func (s *tokenServiceImpl) GetTokenStats(ctx context.Context, req *TokenStatsRequest) (*TokenStatsResponse, error) {
	var since, until time.Time
	if req.Since != nil {
		since = req.Since.AsTime()
	}
	if req.Until != nil {
		until = req.Until.AsTime()
	}
	stats, err := s.store.TokenStats(req.SessionId, since, until)
	if err != nil {
		return nil, err
	}

	byTool := make(map[string]int32, len(stats.ByTool))
	for k, v := range stats.ByTool {
		byTool[k] = int32(v)
	}
	bySaving := make(map[string]int32, len(stats.BySavingType))
	for k, v := range stats.BySavingType {
		bySaving[k] = int32(v)
	}
	byDelivery := make(map[string]int32, len(stats.ByDelivery))
	for k, v := range stats.ByDelivery {
		byDelivery[k] = int32(v)
	}
	callsByTool := make(map[string]int32, len(stats.CallsByTool))
	for k, v := range stats.CallsByTool {
		callsByTool[k] = int32(v)
	}
	savedByTool := make(map[string]int32, len(stats.SavedByTool))
	for k, v := range stats.SavedByTool {
		savedByTool[k] = int32(v)
	}

	return &TokenStatsResponse{
		TotalRead:      int32(stats.TotalRead),
		TotalSaved:     int32(stats.TotalSaved),
		TotalWritten:   int32(stats.TotalWritten),
		EventCount:     int32(stats.EventCount),
		ByTool:         byTool,
		CallsByTool:    callsByTool,
		SavedByTool:    savedByTool,
		BySavingType:   bySaving,
		Sessions:       int32(stats.Sessions),
		ReadCount:      int32(stats.ReadCount),
		CodeToolCount:  int32(stats.CodeToolCount),
		TotalDelivered: int32(stats.TotalDelivered),
		ByDelivery:     byDelivery,
	}, nil
}

func (s *tokenServiceImpl) ListTokenEvents(ctx context.Context, req *TokenEventListRequest) (*TokenEventListResponse, error) {
	// Honour the store contract: limit <= 0 means "all". Callers like
	// StoreAdapter.TokenStats deliberately pass 0 when they need a full
	// scan to aggregate over a time window (the proto doesn't carry
	// since/until yet, so client-side filter requires every event).
	// Cap at a safety upper bound so a malicious/buggy caller can't OOM us.
	limit := int(req.Limit)
	const maxLimit = 100000
	if limit <= 0 || limit > maxLimit {
		limit = maxLimit
	}

	events, err := s.store.ListTokenEvents(req.SessionId, limit, time.Time{}, time.Time{})
	if err != nil {
		return nil, err
	}

	protoEvents := make([]*TokenEventItem, len(events))
	for i, e := range events {
		protoEvents[i] = &TokenEventItem{
			Id:          e.ID,
			SessionId:   e.SessionID,
			Timestamp:   timestamppb.New(e.Timestamp),
			EventType:   e.EventType,
			Tool:        e.Tool,
			FilePath:    e.FilePath,
			Tokens:      int32(e.Tokens),
			TokensSaved: int32(e.TokensSaved),
		}
	}

	return &TokenEventListResponse{Events: protoEvents}, nil
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

	// Note: FindingStatsRequest proto has no include_accepted field,
	// so we default to hiding accepted findings (matching CLI/MCP behaviour).
	stats, err := fs.Stats(findings.SearchOptions{})
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

func (s *findingsServiceImpl) Accept(ctx context.Context, req *FindingAcceptRequest) (*FindingAcceptResponse, error) {
	fs := s.server.GetFindingsStore()
	if fs == nil {
		return nil, fmt.Errorf("findings store not available")
	}

	count, err := fs.AcceptFindings(req.Ids)
	if err != nil {
		return nil, err
	}

	return &FindingAcceptResponse{Count: int32(count)}, nil
}

func (s *findingsServiceImpl) AcceptByFilter(ctx context.Context, req *FindingAcceptByFilterRequest) (*FindingAcceptResponse, error) {
	fs := s.server.GetFindingsStore()
	if fs == nil {
		return nil, fmt.Errorf("findings store not available")
	}

	opts := findings.SearchOptions{
		Analyzer: req.Analyzer,
		Severity: req.Severity,
		FilePath: req.FilePath,
		Category: req.Category,
	}

	count, err := fs.AcceptFindingsByFilter(opts)
	if err != nil {
		return nil, err
	}

	return &FindingAcceptResponse{Count: int32(count)}, nil
}

// =============================================================================
// Survey Service Implementation
// =============================================================================

type surveyServiceImpl struct {
	UnimplementedSurveyServiceServer
	server *Server
}

// Run executes survey analyzers on the daemon, where the stores live.
// gRPC clients (MCP in client mode, CLI in daemon mode) delegate here
// because they cannot open the BoltDB stores directly.
func (s *surveyServiceImpl) Run(ctx context.Context, req *SurveyRunRequest) (*SurveyRunResponse, error) {
	surveyStore := s.server.surveyStore
	if surveyStore == nil {
		return nil, status.Error(codes.Unavailable, "survey store not available")
	}
	var analyzers []string
	if req.Analyzer != "" {
		analyzers = []string{req.Analyzer}
	}
	results := surveyrun.Run(projectRoot(s.server.dbPath), analyzers, surveyStore, s.server.GetCodeStore())

	resp := &SurveyRunResponse{}
	for _, r := range results {
		resp.Results = append(resp.Results, &SurveyRunResult{
			Analyzer: r.Analyzer,
			Entries:  int32(r.Entries),
			Error:    r.Err,
			Summary:  r.Summary,
		})
	}
	return resp, nil
}

func (s *surveyServiceImpl) Add(ctx context.Context, req *SurveyAddRequest) (*SurveyAddResponse, error) {
	ss := s.server.GetSurveyStore()
	if ss == nil {
		return nil, fmt.Errorf("survey store not available")
	}

	e := &survey.Entry{
		Analyzer: req.Analyzer,
		Kind:     req.Kind,
		Name:     req.Name,
		FilePath: req.FilePath,
		Title:    req.Title,
		Detail:   req.Detail,
		Metadata: req.Metadata,
	}

	if err := ss.AddEntry(e); err != nil {
		return nil, err
	}

	return &SurveyAddResponse{
		Entry: surveyEntryToProto(e),
	}, nil
}

func (s *surveyServiceImpl) Get(ctx context.Context, req *SurveyGetRequest) (*SurveyGetResponse, error) {
	ss := s.server.GetSurveyStore()
	if ss == nil {
		return nil, fmt.Errorf("survey store not available")
	}

	e, err := ss.GetEntry(req.Id)
	if err != nil {
		if err == store.ErrNotFound {
			return &SurveyGetResponse{Found: false}, nil
		}
		return nil, err
	}

	return &SurveyGetResponse{
		Entry: surveyEntryToProto(e),
		Found: true,
	}, nil
}

func (s *surveyServiceImpl) Delete(ctx context.Context, req *SurveyDeleteRequest) (*SurveyDeleteResponse, error) {
	ss := s.server.GetSurveyStore()
	if ss == nil {
		return nil, fmt.Errorf("survey store not available")
	}

	if err := ss.DeleteEntry(req.Id); err != nil {
		return nil, err
	}

	return &SurveyDeleteResponse{Success: true}, nil
}

func (s *surveyServiceImpl) Search(ctx context.Context, req *SurveySearchRequest) (*SurveySearchResponse, error) {
	ss := s.server.GetSurveyStore()
	if ss == nil {
		return nil, fmt.Errorf("survey store not available")
	}

	opts := survey.SearchOptions{
		Analyzer: req.Analyzer,
		Kind:     req.Kind,
		FilePath: req.FilePath,
		Limit:    int(req.Limit),
	}

	results, err := ss.SearchEntries(req.Query, opts)
	if err != nil {
		return nil, err
	}

	protoEntries := make([]*SurveyEntry, len(results))
	for i, r := range results {
		protoEntries[i] = surveyEntryToProto(r.Entry)
	}

	return &SurveySearchResponse{Entries: protoEntries}, nil
}

func (s *surveyServiceImpl) List(ctx context.Context, req *SurveyListRequest) (*SurveySearchResponse, error) {
	ss := s.server.GetSurveyStore()
	if ss == nil {
		return nil, fmt.Errorf("survey store not available")
	}

	opts := survey.SearchOptions{
		Analyzer: req.Analyzer,
		Kind:     req.Kind,
		FilePath: req.FilePath,
		Limit:    int(req.Limit),
	}

	results, err := ss.ListEntries(opts)
	if err != nil {
		return nil, err
	}

	protoEntries := make([]*SurveyEntry, len(results))
	for i, e := range results {
		protoEntries[i] = surveyEntryToProto(e)
	}

	return &SurveySearchResponse{Entries: protoEntries}, nil
}

func (s *surveyServiceImpl) GetFileEntries(ctx context.Context, req *SurveyFileRequest) (*SurveySearchResponse, error) {
	ss := s.server.GetSurveyStore()
	if ss == nil {
		return nil, fmt.Errorf("survey store not available")
	}

	results, err := ss.GetFileEntries(req.FilePath)
	if err != nil {
		return nil, err
	}

	protoEntries := make([]*SurveyEntry, len(results))
	for i, e := range results {
		protoEntries[i] = surveyEntryToProto(e)
	}

	return &SurveySearchResponse{Entries: protoEntries}, nil
}

func (s *surveyServiceImpl) ClearAnalyzer(ctx context.Context, req *SurveyClearAnalyzerRequest) (*SurveyClearAnalyzerResponse, error) {
	ss := s.server.GetSurveyStore()
	if ss == nil {
		return nil, fmt.Errorf("survey store not available")
	}

	count, err := ss.ClearAnalyzer(req.Analyzer)
	if err != nil {
		return nil, err
	}

	return &SurveyClearAnalyzerResponse{Count: int32(count)}, nil
}

func (s *surveyServiceImpl) Stats(ctx context.Context, req *SurveyStatsRequest) (*SurveyStatsResponse, error) {
	ss := s.server.GetSurveyStore()
	if ss == nil {
		return nil, fmt.Errorf("survey store not available")
	}

	stats, err := ss.Stats(survey.SearchOptions{})
	if err != nil {
		return nil, err
	}

	byAnalyzer := make(map[string]int32, len(stats.ByAnalyzer))
	for k, v := range stats.ByAnalyzer {
		byAnalyzer[k] = int32(v)
	}
	byKind := make(map[string]int32, len(stats.ByKind))
	for k, v := range stats.ByKind {
		byKind[k] = int32(v)
	}

	return &SurveyStatsResponse{
		Total:      int32(stats.Total),
		ByAnalyzer: byAnalyzer,
		ByKind:     byKind,
	}, nil
}

func (s *surveyServiceImpl) Clear(ctx context.Context, req *SurveyClearRequest) (*SurveyClearResponse, error) {
	ss := s.server.GetSurveyStore()
	if ss == nil {
		return nil, fmt.Errorf("survey store not available")
	}

	if err := ss.Clear(); err != nil {
		return nil, err
	}

	return &SurveyClearResponse{Success: true}, nil
}

// =============================================================================
// Tombstone Service Implementation
// =============================================================================

type tombstoneServiceImpl struct {
	UnimplementedTombstoneServiceServer
	server *Server
}

func (s *tombstoneServiceImpl) Add(ctx context.Context, req *TombstoneAddRequest) (*TombstoneAddResponse, error) {
	ts := s.server.GetTombstoneStore()
	if ts == nil {
		return nil, fmt.Errorf("tombstone store not available")
	}

	t := protoToTombstone(req.Tombstone)
	if t == nil {
		return nil, fmt.Errorf("tombstone is required")
	}

	if err := ts.AddTombstone(t); err != nil {
		return nil, err
	}

	// AddTombstone may stamp DeletedAt when zero; echo the stored value back.
	return &TombstoneAddResponse{Tombstone: tombstoneToProto(t)}, nil
}

func (s *tombstoneServiceImpl) Get(ctx context.Context, req *TombstoneGetRequest) (*TombstoneGetResponse, error) {
	ts := s.server.GetTombstoneStore()
	if ts == nil {
		return nil, fmt.Errorf("tombstone store not available")
	}

	t, err := ts.GetTombstone(req.Kind, req.Id)
	if err != nil {
		if err == store.ErrNotFound {
			return &TombstoneGetResponse{Found: false}, nil
		}
		return nil, err
	}

	return &TombstoneGetResponse{
		Tombstone: tombstoneToProto(t),
		Found:     true,
	}, nil
}

func (s *tombstoneServiceImpl) List(ctx context.Context, req *TombstoneListRequest) (*TombstoneListResponse, error) {
	ts := s.server.GetTombstoneStore()
	if ts == nil {
		return nil, fmt.Errorf("tombstone store not available")
	}

	tombstones, err := ts.ListTombstones()
	if err != nil {
		return nil, err
	}

	protoTombstones := make([]*Tombstone, len(tombstones))
	for i, t := range tombstones {
		protoTombstones[i] = tombstoneToProto(t)
	}
	return &TombstoneListResponse{Tombstones: protoTombstones}, nil
}

func (s *tombstoneServiceImpl) Delete(ctx context.Context, req *TombstoneDeleteRequest) (*TombstoneDeleteResponse, error) {
	ts := s.server.GetTombstoneStore()
	if ts == nil {
		return nil, fmt.Errorf("tombstone store not available")
	}

	if err := ts.DeleteTombstone(req.Kind, req.Id); err != nil {
		return nil, err
	}

	return &TombstoneDeleteResponse{Success: true}, nil
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
	pprofFunc := srv.pprofURLFunc
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

	// Get pprof URL if available
	var pprofURL string
	if pprofFunc != nil {
		pprofURL = pprofFunc()
	}

	resp := &StatusResponse{
		Version:       version.String(),
		Uptime:        formatHumanDuration(time.Since(srv.startTime)),
		ServerRunning: true,
		McpTools:      tools,
		PprofUrl:      pprofURL,
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

	// Findings status (exclude accepted findings for consistency)
	if fss != nil {
		stats, err := fss.Stats(findings.SearchOptions{})
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

			// Seed analyzers from stats.ByAnalyzer first so every analyser
			// with persisted findings appears in the panel — even brand-new
			// ones (deadcode, todos, ...) that haven't run on this daemon
			// instance yet. The runner overlay below then adds runtime
			// status (lastRun, duration) for analysers that have run.
			analyzers := make(map[string]*StatusAnalyzer, len(stats.ByAnalyzer))
			for name, n := range stats.ByAnalyzer {
				analyzers[name] = &StatusAnalyzer{
					Status:   "idle",
					LastRun:  "never",
					Findings: int32(n),
				}
			}
			if fr != nil {
				for name, as := range fr.GetStatus() {
					lastRun := "never"
					if !as.LastRun.IsZero() {
						lastRun = as.LastRun.Format(time.RFC3339)
					}
					existing, ok := analyzers[name]
					if !ok {
						existing = &StatusAnalyzer{}
						analyzers[name] = existing
					}
					existing.Status = as.Status
					existing.Scope = as.Scope
					existing.LastRun = lastRun
					existing.LastDuration = as.LastDuration.String()
					if as.Findings > 0 {
						existing.Findings = int32(as.Findings)
					}
				}
			}
			findingsStatus.Analyzers = analyzers

			resp.Findings = findingsStatus
		}
	}

	// Survey status
	if ss := srv.GetSurveyStore(); ss != nil {
		stats, err := ss.Stats(survey.SearchOptions{})
		if err == nil && stats != nil {
			surveyStatus := &StatusSurvey{
				Available: true,
				Total:     int32(stats.Total),
			}
			byAnalyzer := make(map[string]int32, len(stats.ByAnalyzer))
			for k, v := range stats.ByAnalyzer {
				byAnalyzer[k] = int32(v)
			}
			surveyStatus.ByAnalyzer = byAnalyzer

			byKind := make(map[string]int32, len(stats.ByKind))
			for k, v := range stats.ByKind {
				byKind[k] = int32(v)
			}
			surveyStatus.ByKind = byKind

			resp.Survey = surveyStatus
		}
	}

	// Store sizes
	resp.Stores = getStoreSizes(srv.dbPath)

	// Grammars
	if srv.grammarLoader != nil {
		for _, gi := range srv.grammarLoader.Installed() {
			resp.Grammars = append(resp.Grammars, &StatusGrammar{
				Name:    gi.Name,
				Version: gi.Version,
				BuiltIn: gi.BuiltIn,
			})
		}
	}

	return resp, nil
}

// getStoreSizes computes sizes for all known stores under .aide/memory/.
func getStoreSizes(dbPath string) []*StatusStore {
	if dbPath == "" {
		return nil
	}
	baseDir := filepath.Dir(dbPath) // .aide/memory/
	codeDir := filepath.Join(baseDir, "code")
	findingsDir := filepath.Join(baseDir, "findings")
	surveyDir := filepath.Join(baseDir, "survey")

	entries := []struct{ name, path string }{
		{"memory.db", dbPath},
		{"memory.bleve", store.GetSearchPath(dbPath)},
		{"code.db", filepath.Join(codeDir, "index.db")},
		{"code.bleve", filepath.Join(codeDir, "search.bleve")},
		{"findings.db", filepath.Join(findingsDir, "findings.db")},
		{"findings.bleve", findingsSearchPath(findingsDir)},
		{"survey.db", filepath.Join(surveyDir, "survey.db")},
		{"survey.bleve", filepath.Join(surveyDir, "search.bleve")},
	}

	// Derive relative path from project root for display
	projectRoot := projectRoot(dbPath)

	var stores []*StatusStore
	for _, e := range entries {
		size := dirOrFileSize(e.path)
		if size > 0 {
			rel, _ := filepath.Rel(projectRoot, e.path)
			stores = append(stores, &StatusStore{
				Name: e.name,
				Path: rel,
				Size: size,
			})
		}
	}
	return stores
}

// findingsSearchPath returns search.bleve if it exists, otherwise falls back to legacy findings.idx.
func findingsSearchPath(dir string) string {
	p := filepath.Join(dir, "search.bleve")
	if _, err := os.Stat(p); err == nil {
		return p
	}
	return filepath.Join(dir, "findings.idx")
}

// dirOrFileSize returns the total size of a file, or of all files in a directory tree.
func dirOrFileSize(path string) int64 {
	info, err := os.Stat(path)
	if err != nil {
		return 0
	}
	if !info.IsDir() {
		return info.Size()
	}
	var total int64
	_ = filepath.Walk(path, func(_ string, fi os.FileInfo, err error) error {
		if err == nil && !fi.IsDir() {
			total += fi.Size()
		}
		return nil
	})
	return total
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

func surveyEntryToProto(e *survey.Entry) *SurveyEntry {
	if e == nil {
		return nil
	}
	return &SurveyEntry{
		Id:        e.ID,
		Analyzer:  e.Analyzer,
		Kind:      e.Kind,
		Name:      e.Name,
		FilePath:  e.FilePath,
		Title:     e.Title,
		Detail:    e.Detail,
		Metadata:  e.Metadata,
		CreatedAt: timestamppb.New(e.CreatedAt),
	}
}

func tombstoneToProto(t *memory.Tombstone) *Tombstone {
	if t == nil {
		return nil
	}
	return &Tombstone{
		Id:        t.ID,
		Kind:      t.Kind,
		DeletedAt: timestamppb.New(t.DeletedAt),
	}
}

func protoToTombstone(pt *Tombstone) *memory.Tombstone {
	if pt == nil {
		return nil
	}
	var deletedAt time.Time
	if pt.DeletedAt != nil {
		deletedAt = pt.DeletedAt.AsTime()
	}
	return &memory.Tombstone{
		ID:        pt.Id,
		Kind:      pt.Kind,
		DeletedAt: deletedAt,
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

func referenceToProto(r *code.Reference) *CodeReference {
	if r == nil {
		return nil
	}
	return &CodeReference{
		Id:         r.ID,
		SymbolName: r.SymbolName,
		Kind:       r.Kind,
		FilePath:   r.FilePath,
		Line:       int32(r.Line),
		Column:     int32(r.Column),
		Context:    r.Context,
		Language:   r.Language,
		CreatedAt:  timestamppb.New(r.CreatedAt),
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
		Id:              m.ID,
		From:            m.From,
		To:              m.To,
		Content:         m.Content,
		Type:            m.Type,
		Priority:        m.Priority,
		ParentSessionId: m.ParentSessionID,
		ReadBy:          m.ReadBy,
		CreatedAt:       timestamppb.New(m.CreatedAt),
		ExpiresAt:       timestamppb.New(m.ExpiresAt),
	}
}

func taskToProto(t *memory.Task) *Task {
	if t == nil {
		return nil
	}
	return &Task{
		Id:              t.ID,
		Title:           t.Title,
		Description:     t.Description,
		Status:          string(t.Status),
		ClaimedBy:       t.ClaimedBy,
		Worktree:        t.Worktree,
		Result:          t.Result,
		ParentSessionId: t.ParentSessionID,
		CreatedAt:       timestamppb.New(t.CreatedAt),
		ClaimedAt:       timestamppb.New(t.ClaimedAt),
		CompletedAt:     timestamppb.New(t.CompletedAt),
	}
}

// =============================================================================
// Observe Service Implementation
// =============================================================================

type observeServiceImpl struct {
	UnimplementedObserveServiceServer
	store store.Store
	bus   *eventbus.Broadcaster[*observe.Event]
}

func (s *observeServiceImpl) RecordEvent(ctx context.Context, req *ObserveRecordRequest) (*ObserveRecordResponse, error) {
	e := &observe.Event{
		Kind:        observe.Kind(req.Kind),
		Name:        req.Name,
		Category:    req.Category,
		Subtype:     req.Subtype,
		DurationMs:  req.DurationMs,
		Tokens:      int(req.Tokens),
		TokensSaved: int(req.TokensSaved),
		FilePath:    req.FilePath,
		Parent:      req.Parent,
		SessionID:   req.SessionId,
		Error:       req.Error,
		Attrs:       req.Attrs,
	}
	if err := s.store.AddObserveEvent(e); err != nil {
		return nil, err
	}
	if s.bus != nil {
		s.bus.Publish(e)
	}
	return &ObserveRecordResponse{Id: e.ID}, nil
}

func (s *observeServiceImpl) ListEvents(ctx context.Context, req *ObserveListRequest) (*ObserveListResponse, error) {
	f := store.ObserveFilter{
		Kind:      observe.Kind(req.Kind),
		Name:      req.Name,
		Category:  req.Category,
		SessionID: req.SessionId,
		Limit:     int(req.Limit),
	}
	if req.SinceUnixMs > 0 {
		f.Since = time.UnixMilli(req.SinceUnixMs)
	}
	if req.UntilUnixMs > 0 {
		f.Until = time.UnixMilli(req.UntilUnixMs)
	}
	events, err := s.store.ListObserveEvents(f)
	if err != nil {
		return nil, err
	}
	out := make([]*ObserveEvent, 0, len(events))
	for _, e := range events {
		out = append(out, observeEventToProto(e))
	}
	return &ObserveListResponse{Events: out}, nil
}

func observeEventToProto(e *observe.Event) *ObserveEvent {
	return &ObserveEvent{
		Id:          e.ID,
		Timestamp:   timestamppb.New(e.Timestamp),
		Kind:        string(e.Kind),
		Name:        e.Name,
		Category:    e.Category,
		Subtype:     e.Subtype,
		DurationMs:  e.DurationMs,
		Tokens:      int32(e.Tokens),
		TokensSaved: int32(e.TokensSaved),
		FilePath:    e.FilePath,
		Parent:      e.Parent,
		SessionId:   e.SessionID,
		Error:       e.Error,
		Attrs:       e.Attrs,
	}
}

func (s *observeServiceImpl) WatchEvents(req *ObserveWatchRequest, stream ObserveService_WatchEventsServer) error {
	ctx := stream.Context()

	matches := func(e *observe.Event) bool {
		if req.Kind != "" && string(e.Kind) != req.Kind {
			return false
		}
		if req.Name != "" && e.Name != req.Name {
			return false
		}
		if req.Category != "" && e.Category != req.Category {
			return false
		}
		if req.SessionId != "" && e.SessionID != req.SessionId {
			return false
		}
		return true
	}

	// Subscribe before backfill so live writes during backfill aren't lost.
	if s.bus == nil {
		return status.Error(codes.FailedPrecondition, "observe broadcaster not configured")
	}
	sub, unsub := s.bus.Subscribe(ctx, matches)
	defer unsub()

	var maxBackfillID string
	if req.SinceId != "" {
		id, err := ulid.Parse(req.SinceId)
		if err != nil {
			return status.Errorf(codes.InvalidArgument, "invalid since_id: %v", err)
		}
		f := store.ObserveFilter{
			Kind:      observe.Kind(req.Kind),
			Name:      req.Name,
			Category:  req.Category,
			SessionID: req.SessionId,
			Since:     time.UnixMilli(int64(id.Time())),
		}
		events, err := s.store.ListObserveEvents(f)
		if err != nil {
			return err
		}
		// List returns newest-first; reverse to send chronologically.
		for i := len(events) - 1; i >= 0; i-- {
			e := events[i]
			if e.ID <= req.SinceId {
				continue
			}
			if err := stream.Send(observeEventToProto(e)); err != nil {
				return err
			}
			if e.ID > maxBackfillID {
				maxBackfillID = e.ID
			}
		}
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case e, ok := <-sub:
			if !ok {
				return nil
			}
			if maxBackfillID != "" && e.ID <= maxBackfillID {
				continue
			}
			if err := stream.Send(observeEventToProto(e)); err != nil {
				return err
			}
		}
	}
}

// =============================================================================
// Instinct Service Implementation
// =============================================================================

type instinctServiceImpl struct {
	UnimplementedInstinctServiceServer
	server *Server
}

func (s *instinctServiceImpl) requireStore() (store.InstinctProposalStore, error) {
	if ps := s.server.GetInstinctStore(); ps != nil {
		return ps, nil
	}
	return nil, status.Error(codes.FailedPrecondition, "instinct proposal store not configured")
}

func (s *instinctServiceImpl) List(ctx context.Context, req *InstinctListRequest) (*InstinctListResponse, error) {
	ps, err := s.requireStore()
	if err != nil {
		return nil, err
	}
	f := store.InstinctFilter{
		Status:    instinct.Status(req.Status),
		Shape:     req.Shape,
		SessionID: req.SessionId,
		Limit:     int(req.Limit),
	}
	props, err := ps.ListInstinctProposals(f)
	if err != nil {
		return nil, err
	}
	out := make([]*InstinctProposal, 0, len(props))
	for _, p := range props {
		out = append(out, instinctProposalToProto(p))
	}
	return &InstinctListResponse{Proposals: out}, nil
}

func (s *instinctServiceImpl) Get(ctx context.Context, req *InstinctGetRequest) (*InstinctGetResponse, error) {
	ps, err := s.requireStore()
	if err != nil {
		return nil, err
	}
	p, err := ps.GetInstinctProposal(req.Id)
	if err != nil {
		return nil, err
	}
	if p == nil {
		return &InstinctGetResponse{Found: false}, nil
	}
	return &InstinctGetResponse{Proposal: instinctProposalToProto(p), Found: true}, nil
}

func (s *instinctServiceImpl) Add(ctx context.Context, req *InstinctAddRequest) (*InstinctAddResponse, error) {
	ps, err := s.requireStore()
	if err != nil {
		return nil, err
	}
	if req.Proposal == nil {
		return nil, status.Error(codes.InvalidArgument, "proposal required")
	}
	p := protoToInstinctProposal(req.Proposal)
	if err := ps.AddInstinctProposal(p); err != nil {
		return nil, err
	}
	if bus := s.server.InstinctBus(); bus != nil {
		bus.Publish(p)
	}
	return &InstinctAddResponse{Proposal: instinctProposalToProto(p)}, nil
}

func (s *instinctServiceImpl) UpdateStatus(ctx context.Context, req *InstinctUpdateStatusRequest) (*InstinctUpdateStatusResponse, error) {
	ps, err := s.requireStore()
	if err != nil {
		return nil, err
	}
	p, err := ps.UpdateInstinctProposalStatus(req.Id, instinct.Status(req.Status), req.Reason, req.AcceptedMemoryId)
	if err != nil {
		return nil, err
	}
	if p == nil {
		return nil, status.Error(codes.NotFound, "proposal not found")
	}
	if bus := s.server.InstinctBus(); bus != nil {
		bus.Publish(p)
	}
	return &InstinctUpdateStatusResponse{Proposal: instinctProposalToProto(p)}, nil
}

func (s *instinctServiceImpl) Watch(req *InstinctWatchRequest, stream InstinctService_WatchServer) error {
	ps, err := s.requireStore()
	if err != nil {
		return err
	}
	ctx := stream.Context()
	bus := s.server.InstinctBus()
	if bus == nil {
		return status.Error(codes.FailedPrecondition, "instinct broadcaster not configured")
	}

	matches := func(p *instinct.Proposal) bool {
		if req.Status != "" && string(p.Status) != req.Status {
			return false
		}
		if req.Shape != "" && p.Shape != req.Shape {
			return false
		}
		if req.SessionId != "" && p.SessionID != req.SessionId {
			return false
		}
		return true
	}
	sub, unsub := bus.Subscribe(ctx, matches)
	defer unsub()

	var maxBackfillID string
	if req.SinceId != "" {
		if _, perr := ulid.Parse(req.SinceId); perr != nil {
			return status.Errorf(codes.InvalidArgument, "invalid since_id: %v", perr)
		}
		props, err := ps.ListInstinctProposals(store.InstinctFilter{
			Status:    instinct.Status(req.Status),
			Shape:     req.Shape,
			SessionID: req.SessionId,
		})
		if err != nil {
			return err
		}
		for i := len(props) - 1; i >= 0; i-- {
			p := props[i]
			if p.ID <= req.SinceId {
				continue
			}
			if err := stream.Send(instinctProposalToProto(p)); err != nil {
				return err
			}
			if p.ID > maxBackfillID {
				maxBackfillID = p.ID
			}
		}
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case p, ok := <-sub:
			if !ok {
				return nil
			}
			if maxBackfillID != "" && p.ID <= maxBackfillID {
				continue
			}
			if err := stream.Send(instinctProposalToProto(p)); err != nil {
				return err
			}
		}
	}
}

func instinctProposalToProto(p *instinct.Proposal) *InstinctProposal {
	if p == nil {
		return nil
	}
	out := &InstinctProposal{
		Id:               p.ID,
		Shape:            p.Shape,
		SessionId:        p.SessionID,
		Summary:          p.Summary,
		Status:           string(p.Status),
		RejectionCount:   int32(p.RejectionCount),
		RejectionReason:  p.RejectionReason,
		AcceptedMemoryId: p.AcceptedMemoryID,
	}
	if !p.ProposedAt.IsZero() {
		out.ProposedAt = timestamppb.New(p.ProposedAt)
	}
	if !p.LastReproposalAt.IsZero() {
		out.LastReproposalAt = timestamppb.New(p.LastReproposalAt)
	}
	if !p.ExpiresAt.IsZero() {
		out.ExpiresAt = timestamppb.New(p.ExpiresAt)
	}
	out.Evidence = &InstinctEvidence{
		ObserveEventIds: p.Evidence.ObserveEventIDs,
		CrossSessionIds: p.Evidence.CrossSessionIDs,
	}
	for _, e := range p.Evidence.Snapshot {
		out.Evidence.Snapshot = append(out.Evidence.Snapshot, observeEventToProto(e))
	}
	out.ProposedInstinct = &InstinctProposedMemory{
		Category: p.ProposedInstinct.Category,
		Content:  p.ProposedInstinct.Content,
		Tags:     p.ProposedInstinct.Tags,
		Priority: p.ProposedInstinct.Priority,
	}
	return out
}

func protoToInstinctProposal(p *InstinctProposal) *instinct.Proposal {
	if p == nil {
		return nil
	}
	out := &instinct.Proposal{
		ID:               p.Id,
		Shape:            p.Shape,
		SessionID:        p.SessionId,
		Summary:          p.Summary,
		Status:           instinct.Status(p.Status),
		RejectionCount:   int(p.RejectionCount),
		RejectionReason:  p.RejectionReason,
		AcceptedMemoryID: p.AcceptedMemoryId,
	}
	if p.ProposedAt != nil {
		out.ProposedAt = p.ProposedAt.AsTime()
	}
	if p.LastReproposalAt != nil {
		out.LastReproposalAt = p.LastReproposalAt.AsTime()
	}
	if p.ExpiresAt != nil {
		out.ExpiresAt = p.ExpiresAt.AsTime()
	}
	if p.Evidence != nil {
		out.Evidence.ObserveEventIDs = p.Evidence.ObserveEventIds
		out.Evidence.CrossSessionIDs = p.Evidence.CrossSessionIds
		for _, e := range p.Evidence.Snapshot {
			ev := &observe.Event{
				ID:          e.Id,
				Kind:        observe.Kind(e.Kind),
				Name:        e.Name,
				Category:    e.Category,
				Subtype:     e.Subtype,
				DurationMs:  e.DurationMs,
				Tokens:      int(e.Tokens),
				TokensSaved: int(e.TokensSaved),
				FilePath:    e.FilePath,
				Parent:      e.Parent,
				SessionID:   e.SessionId,
				Error:       e.Error,
				Attrs:       e.Attrs,
			}
			if e.Timestamp != nil {
				ev.Timestamp = e.Timestamp.AsTime()
			}
			out.Evidence.Snapshot = append(out.Evidence.Snapshot, ev)
		}
	}
	if p.ProposedInstinct != nil {
		out.ProposedInstinct.Category = p.ProposedInstinct.Category
		out.ProposedInstinct.Content = p.ProposedInstinct.Content
		out.ProposedInstinct.Tags = p.ProposedInstinct.Tags
		out.ProposedInstinct.Priority = p.ProposedInstinct.Priority
	}
	return out
}
