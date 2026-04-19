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
}

export interface TokenStats {
  total_read: number;
  total_saved: number;
  total_written: number;
  event_count: number;
  by_tool: Record<string, number>;
  by_saving_type: Record<string, number>;
  sessions: number;
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
