export type InstanceStatus = "connected" | "disconnected" | "connecting";

export interface InstanceInfo {
  project_root: string;
  project_name: string;
  socket_path: string;
  status: InstanceStatus;
  version: string;
}

export interface MemoryItem {
  id: string;
  category: string;
  content: string;
  tags: string[];
  created_at: string;
}

export interface DecisionItem {
  topic: string;
  decision: string;
  rationale: string;
  decided_by: string;
  created_at: string;
}

export interface TaskItem {
  id: string;
  title: string;
  description?: string;
  status: string;
  claimed_by?: string;
  result?: string;
}

export interface MessageItem {
  id: number;
  from: string;
  to: string;
  content: string;
  type: string;
}

export interface StateItem {
  key: string;
  value: string;
  agent?: string;
}

export interface FindingItem {
  id: string;
  analyzer: string;
  severity: string;
  category: string;
  file_path: string;
  line: number;
  title: string;
  accepted: boolean;
}

export interface SurveyItem {
  id: string;
  analyzer: string;
  kind: string;
  name: string;
  file_path: string;
  title: string;
}

export interface SearchResult {
  instance: string;
  type: string;
  title: string;
  detail: string;
  link?: string;
}

export interface TokenEventItem {
  id: string;
  session_id: string;
  timestamp: string;
  event_type: string;
  tool: string;
  file_path: string;
  tokens: number;
  tokens_saved: number;
  /** 1-based start line within file_path; 0 = no range available. */
  start_line?: number;
  /** 1-based end line (inclusive); 0 = no range. */
  end_line?: number;
}

export interface ObserveEventItem {
  id: string;
  timestamp: string;
  kind: string;
  name: string;
  category?: string;
  subtype?: string;
  duration_ms?: number;
  tokens?: number;
  tokens_saved?: number;
  file_path?: string;
  parent?: string;
  session_id?: string;
  error?: string;
  attrs?: Record<string, string>;
}

export interface TokenStats {
  total_read: number;
  total_saved: number;
  total_written: number;
  total_delivered: number;
  event_count: number;
  by_tool: Record<string, number>;
  calls_by_tool: Record<string, number>;
  saved_by_tool: Record<string, number>;
  by_saving_type: Record<string, number>;
  by_delivery: Record<string, number>;
  sessions: number;
  read_count: number;
  code_tool_count: number;
}

export interface DetailedStatus {
  version: string;
  uptime: string;
  server_running: boolean;
  watcher?: {
    enabled: boolean;
    paths: string[];
    dirs_watched: number;
    debounce: string;
    pending: number;
    subscribers: string[];
  };
  code_indexer?: {
    available: boolean;
    status: string;
    symbols: number;
    references: number;
    files: number;
  };
  findings?: {
    available: boolean;
    total: number;
    by_analyzer: Record<string, number>;
    by_severity: Record<string, number>;
    analyzers: Record<string, {
      status: string;
      scope: string;
      last_run: string;
      findings: number;
      last_duration: string;
    }>;
  };
  survey?: {
    available: boolean;
    total: number;
    by_analyzer: Record<string, number>;
    by_kind: Record<string, number>;
  };
  stores?: Array<{
    name: string;
    path: string;
    size: number;
  }>;
  grammars?: Array<{
    name: string;
    version?: string;
    built_in: boolean;
  }>;
}

export interface InstinctProposedMemory {
  category: string;
  content: string;
  tags?: string[];
  priority?: number;
}

export interface InstinctEvidence {
  observe_event_ids?: string[];
  cross_session_ids?: string[];
  snapshot?: ObserveEventItem[];
}

export type InstinctStatus = "open" | "accepted" | "rejected" | "expired";

export interface InstinctProposalItem {
  id: string;
  shape: string;
  session_id?: string;
  proposed_at: string;
  summary: string;
  status: InstinctStatus;
  rejection_count?: number;
  rejection_reason?: string;
  accepted_memory_id?: string;
  last_reproposal_at?: string;
  expires_at?: string;
  evidence: InstinctEvidence;
  proposed_instinct: InstinctProposedMemory;
}

export interface SwarmAgentItem {
  agent: string;
  parent_session?: string;
  namespace?: string;
  status?: string;
  type?: string;
  started_at?: string;
  halt?: boolean;
  halt_reason?: string;
  paused?: boolean;
  deadline?: string;
}

export interface SwarmTaskUpdate {
  id: string;
  title: string;
  status: string;
  claimed_by?: string;
  parent_session_id?: string;
  worktree?: string;
  result?: string;
  created_at?: string;
  claimed_at?: string;
  completed_at?: string;
}

export interface SwarmMessageUpdate {
  id: number;
  from: string;
  to?: string;
  content: string;
  type?: string;
  priority?: string;
  parent_session_id?: string;
  created_at?: string;
}

export interface SwarmStateUpdate {
  key: string;
  value?: string;
  agent?: string;
  change: "set" | "delete";
  updated_at?: string;
}
