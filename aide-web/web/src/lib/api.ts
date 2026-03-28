import type {
  InstanceInfo,
  MemoryItem,
  DecisionItem,
  TaskItem,
  MessageItem,
  StateItem,
  FindingItem,
  SurveyItem,
  SearchResult,
  DetailedStatus,
} from "./types";

const BASE = "/api";

async function get<T>(path: string, params?: Record<string, string>): Promise<T> {
  const url = new URL(path, window.location.origin);
  if (params) {
    Object.entries(params).forEach(([k, v]) => {
      if (v) url.searchParams.set(k, v);
    });
  }
  const res = await fetch(url.toString());
  if (!res.ok) {
    throw new Error(`API ${res.status}: ${res.statusText}`);
  }
  return res.json();
}

async function post(path: string, body: unknown): Promise<void> {
  const res = await fetch(new URL(path, window.location.origin).toString(), {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
  if (!res.ok) {
    throw new Error(`API ${res.status}: ${res.statusText}`);
  }
}

async function postJson<T>(path: string, body?: unknown): Promise<T> {
  const res = await fetch(new URL(path, window.location.origin).toString(), {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: body !== undefined ? JSON.stringify(body) : undefined,
  });
  if (!res.ok) {
    throw new Error(`API ${res.status}: ${res.statusText}`);
  }
  return res.json();
}

async function del(path: string): Promise<void> {
  const res = await fetch(new URL(path, window.location.origin).toString(), {
    method: "DELETE",
  });
  if (!res.ok) {
    throw new Error(`API ${res.status}: ${res.statusText}`);
  }
}

export const api = {
  getVersion: () =>
    get<{ version: string }>(`${BASE}/version`).then((r) => r.version),

  listInstances: () =>
    get<{ instances: InstanceInfo[] }>(`${BASE}/instances`).then(
      (r) => r.instances ?? []
    ),

  getStatus: (project: string) =>
    get<{ status: string; version?: string }>(
      `${BASE}/instances/${encodeURIComponent(project)}/status`
    ),

  getDetailedStatus: (project: string) =>
    get<DetailedStatus>(
      `${BASE}/instances/${encodeURIComponent(project)}/status/detailed`
    ),

  listMemories: (project: string, category?: string) =>
    get<{ memories: MemoryItem[] }>(
      `${BASE}/instances/${encodeURIComponent(project)}/memories`,
      category ? { category } : undefined
    ).then((r) => r.memories ?? []),

  getMemory: (project: string, id: string) =>
    get<{ memory: MemoryItem }>(
      `${BASE}/instances/${encodeURIComponent(project)}/memories/${encodeURIComponent(id)}`
    ).then((r) => r.memory),

  createMemory: (project: string, data: { category: string; content: string; tags?: string[] }) =>
    post(`${BASE}/instances/${encodeURIComponent(project)}/memories`, data),

  deleteMemory: (project: string, id: string) =>
    del(`${BASE}/instances/${encodeURIComponent(project)}/memories/${encodeURIComponent(id)}`),

  listDecisions: (project: string) =>
    get<{ decisions: DecisionItem[] }>(
      `${BASE}/instances/${encodeURIComponent(project)}/decisions`
    ).then((r) => r.decisions ?? []),

  createDecision: (project: string, data: { topic: string; decision: string; rationale?: string; decided_by?: string }) =>
    post(`${BASE}/instances/${encodeURIComponent(project)}/decisions`, data),

  deleteDecision: (project: string, topic: string) =>
    del(`${BASE}/instances/${encodeURIComponent(project)}/decisions/${encodeURIComponent(topic)}`),

  listTasks: (project: string, status?: string) =>
    get<{ tasks: TaskItem[] }>(
      `${BASE}/instances/${encodeURIComponent(project)}/tasks`,
      status ? { status } : undefined
    ).then((r) => r.tasks ?? []),

  createTask: (project: string, data: { title: string; description?: string }) =>
    post(`${BASE}/instances/${encodeURIComponent(project)}/tasks`, data),

  deleteTask: (project: string, id: string) =>
    del(`${BASE}/instances/${encodeURIComponent(project)}/tasks/${encodeURIComponent(id)}`),

  listMessages: (project: string, agent?: string) =>
    get<{ messages: MessageItem[] }>(
      `${BASE}/instances/${encodeURIComponent(project)}/messages`,
      agent ? { agent } : undefined
    ).then((r) => r.messages ?? []),

  listState: (project: string, agent?: string) =>
    get<{ states: StateItem[] }>(
      `${BASE}/instances/${encodeURIComponent(project)}/state`,
      agent ? { agent } : undefined
    ).then((r) => r.states ?? []),

  deleteState: (project: string, key: string) =>
    del(`${BASE}/instances/${encodeURIComponent(project)}/state/${encodeURIComponent(key)}`),

  acceptFindings: (project: string, ids: string[]) =>
    post(`${BASE}/instances/${encodeURIComponent(project)}/findings/accept`, { ids }),

  listFindings: (project: string, analyzer?: string, severity?: string) =>
    get<{ findings: FindingItem[] }>(
      `${BASE}/instances/${encodeURIComponent(project)}/findings`,
      { ...(analyzer && { analyzer }), ...(severity && { severity }) }
    ).then((r) => r.findings ?? []),

  listSurvey: (project: string, analyzer?: string, kind?: string) =>
    get<{ entries: SurveyItem[] }>(
      `${BASE}/instances/${encodeURIComponent(project)}/survey`,
      { ...(analyzer && { analyzer }), ...(kind && { kind }) }
    ).then((r) => r.entries ?? []),

  search: (query: string) =>
    get<{ results: SearchResult[] }>(`${BASE}/search`, { q: query }).then(
      (r) => r.results ?? []
    ),

  readFile: (project: string, path: string) =>
    get<{ path: string; content: string; language: string; lines: number }>(
      `${BASE}/instances/${encodeURIComponent(project)}/code/file`,
      { path }
    ),

  runCodeIndex: (project: string) =>
    postJson<{ files_indexed: number; symbols_indexed: number; files_skipped: number }>(
      `${BASE}/instances/${encodeURIComponent(project)}/code/index`
    ),
};
