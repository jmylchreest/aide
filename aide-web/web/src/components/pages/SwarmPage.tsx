import { useMemo, useState } from "react";
import { useParams } from "react-router-dom";
import { api } from "@/lib/api";
import { useApi } from "@/hooks/use-api";
import {
  useWatchSwarmTasks,
  useWatchSwarmMessages,
  useWatchSwarmState,
} from "@/hooks/useSwarmStreams";
import { formatTimestamp } from "@/lib/format";
import type { SwarmAgentItem } from "@/lib/types";
import {
  Network,
  Pause,
  Play,
  Square,
  AlertTriangle,
  RefreshCw,
} from "lucide-react";

type Pane = "tasks" | "messages" | "state";

const PANE_LABEL: Record<Pane, string> = {
  tasks: "Tasks",
  messages: "Messages",
  state: "State",
};

export function SwarmPage() {
  const { project } = useParams<{ project: string }>();
  const [pane, setPane] = useState<Pane>("tasks");
  // Selection is a single state: clicking a parent header selects the
  // whole swarm; clicking an agent narrows further. Either being empty
  // means "everything for this project".
  const [parentSession, setParentSession] = useState<string>("");
  const [selectedAgent, setSelectedAgent] = useState<string>("");
  const [includeStale, setIncludeStale] = useState(false);

  const {
    data: agents,
    loading,
    refresh: refreshAgents,
  } = useApi<SwarmAgentItem[]>(
    () =>
      project
        ? api.listSwarmAgents(project, undefined, includeStale)
        : Promise.resolve([]),
    [project, includeStale],
  );

  const { tasks, status: tasksStatus } = useWatchSwarmTasks({
    project,
    parentSession: parentSession || undefined,
    enabled: pane === "tasks",
  });
  const { messages, status: messagesStatus } = useWatchSwarmMessages({
    project,
    parentSession: parentSession || undefined,
    agent: selectedAgent || undefined,
    enabled: pane === "messages",
  });
  const { entries: stateEntries, status: stateStatus } = useWatchSwarmState({
    project,
    agent: selectedAgent || undefined,
    enabled: pane === "state",
  });

  // Tasks filter client-side by selected agent's claim, since WatchTasks
  // server filter only supports parent_session + status today.
  const visibleTasks = useMemo(
    () =>
      selectedAgent
        ? tasks.filter((t) => t.claimed_by === selectedAgent)
        : tasks,
    [tasks, selectedAgent],
  );

  // Group agents by parent_session for the tree view. Chronological order
  // (newest startedAt first) inside each group; sessions ordered by their
  // most-recent agent. Server returns the same order, this re-sort is just
  // defensive.
  const tree = useMemo(() => {
    const byParent: Record<string, SwarmAgentItem[]> = {};
    for (const a of agents ?? []) {
      const key = a.parent_session || "(orchestrator / solo)";
      (byParent[key] ??= []).push(a);
    }
    for (const k of Object.keys(byParent)) {
      byParent[k].sort((a, b) => {
        const ta = a.started_at ? Date.parse(a.started_at) : 0;
        const tb = b.started_at ? Date.parse(b.started_at) : 0;
        if (ta !== tb) return tb - ta;
        return a.agent.localeCompare(b.agent);
      });
    }
    return byParent;
  }, [agents]);

  const sessions = useMemo(() => {
    const keys = Object.keys(tree);
    keys.sort((a, b) => {
      const newest = (k: string) =>
        tree[k].reduce(
          (max, x) => Math.max(max, x.started_at ? Date.parse(x.started_at) : 0),
          0,
        );
      const ta = newest(a);
      const tb = newest(b);
      if (ta !== tb) return tb - ta;
      return a.localeCompare(b);
    });
    return keys;
  }, [tree]);

  const sendControl = async (
    action: "halt" | "pause" | "resume" | "deadline",
    agent: string,
    extras: { reason?: string; duration?: string } = {},
  ) => {
    if (!project || !agent) return;
    try {
      await api.agentControl(project, { agent, action, ...extras });
      refreshAgents();
    } catch (err) {
      // eslint-disable-next-line no-console
      console.error("agent control failed", err);
    }
  };

  return (
    <div className="flex gap-6 h-full">
      <div className="w-[280px] shrink-0 border-r border-aide-border pr-4">
        <div className="flex items-center justify-between mb-3">
          <h2 className="text-sm font-semibold flex items-center gap-2">
            <Network className="w-4 h-4" />
            Swarms
          </h2>
          <button
            className="text-xs text-aide-text-muted hover:text-aide-text"
            onClick={refreshAgents}
            title="Refresh agent list"
          >
            <RefreshCw className="w-3.5 h-3.5" />
          </button>
        </div>
        <div className="mb-2 flex items-center justify-between text-xs">
          <button
            onClick={() => {
              setParentSession("");
              setSelectedAgent("");
            }}
            className={`px-2 py-0.5 rounded ${
              !parentSession && !selectedAgent
                ? "bg-aide-accent/10 text-aide-accent font-semibold"
                : "text-aide-text-muted hover:text-aide-text"
            }`}
          >
            All
          </button>
          <label className="flex items-center gap-1 text-aide-text-muted">
            <input
              type="checkbox"
              checked={includeStale}
              onChange={(e) => setIncludeStale(e.target.checked)}
              className="w-3 h-3"
            />
            stale
          </label>
        </div>
        {loading && <p className="text-xs text-aide-text-muted">loading…</p>}
        {!loading && sessions.length === 0 && (
          <p className="text-xs text-aide-text-muted">
            No agents registered for this project.
          </p>
        )}
        <ul className="space-y-3 text-xs">
          {sessions.map((sid) => (
            <li key={sid}>
              <button
                className={`w-full text-left truncate px-1 py-0.5 rounded ${
                  parentSession === sid && !selectedAgent
                    ? "bg-aide-accent/10 text-aide-accent font-semibold"
                    : "text-aide-text-muted hover:text-aide-text"
                }`}
                title={sid}
                onClick={() => {
                  setParentSession(sid === "(orchestrator / solo)" ? "" : sid);
                  setSelectedAgent("");
                }}
              >
                {sid.length > 24 ? `${sid.slice(0, 8)}…${sid.slice(-8)}` : sid}
                <span className="ml-1 opacity-60">({tree[sid].length})</span>
              </button>
              <ul className="ml-3 mt-1 space-y-1">
                {tree[sid].map((a) => (
                  <li
                    key={a.agent}
                    className={`flex items-center justify-between px-2 py-1 rounded cursor-pointer ${
                      selectedAgent === a.agent
                        ? "bg-aide-accent/10 text-aide-accent"
                        : "hover:bg-aide-bg-elevated"
                    }`}
                    onClick={() => {
                      setSelectedAgent(a.agent);
                      if (a.parent_session) setParentSession(a.parent_session);
                    }}
                  >
                    <span className="truncate flex-1" title={a.agent}>
                      {a.agent.length > 16
                        ? `${a.agent.slice(0, 8)}…${a.agent.slice(-4)}`
                        : a.agent}
                    </span>
                    <span className="flex items-center gap-1">
                      {a.halt && (
                        <AlertTriangle
                          className="w-3 h-3 text-red-400"
                          title={a.halt_reason || "halted"}
                        />
                      )}
                      {a.paused && (
                        <Pause
                          className="w-3 h-3 text-amber-400"
                          title="paused"
                        />
                      )}
                      {a.status === "running" && !a.halt && !a.paused && (
                        <span className="w-2 h-2 rounded-full bg-emerald-400" />
                      )}
                    </span>
                  </li>
                ))}
              </ul>
            </li>
          ))}
        </ul>
      </div>

      <div className="flex-1 min-w-0">
        <div className="flex items-center justify-between mb-3">
          <div className="flex gap-1">
            {(Object.keys(PANE_LABEL) as Pane[]).map((p) => (
              <button
                key={p}
                onClick={() => setPane(p)}
                className={`px-3 py-1 text-xs rounded ${
                  pane === p
                    ? "bg-aide-accent/10 text-aide-accent font-semibold"
                    : "text-aide-text-muted hover:text-aide-text"
                }`}
              >
                {PANE_LABEL[p]}
              </button>
            ))}
          </div>
          {selectedAgent && (
            <div className="flex items-center gap-1 text-xs">
              <span className="text-aide-text-muted mr-2">
                {selectedAgent.slice(0, 8)}…
              </span>
              <button
                onClick={() =>
                  sendControl("halt", selectedAgent, {
                    reason: "halted from dashboard",
                  })
                }
                className="flex items-center gap-1 px-2 py-1 rounded bg-red-500/10 text-red-400 hover:bg-red-500/20"
              >
                <Square className="w-3 h-3" /> Halt
              </button>
              <button
                onClick={() => sendControl("pause", selectedAgent)}
                className="flex items-center gap-1 px-2 py-1 rounded bg-amber-500/10 text-amber-400 hover:bg-amber-500/20"
              >
                <Pause className="w-3 h-3" /> Pause
              </button>
              <button
                onClick={() => sendControl("resume", selectedAgent)}
                className="flex items-center gap-1 px-2 py-1 rounded bg-emerald-500/10 text-emerald-400 hover:bg-emerald-500/20"
              >
                <Play className="w-3 h-3" /> Resume
              </button>
            </div>
          )}
        </div>

        <div className="mb-2 text-xs text-aide-text-muted">
          Filter:{" "}
          {selectedAgent ? (
            <>agent <code>{selectedAgent.slice(0, 12)}…</code></>
          ) : parentSession ? (
            <>swarm <code>{parentSession.slice(0, 12)}…</code></>
          ) : (
            <>all (no filter)</>
          )}
        </div>

        {pane === "tasks" && (
          <TasksPane tasks={visibleTasks} status={tasksStatus} />
        )}
        {pane === "messages" && (
          <MessagesPane messages={messages} status={messagesStatus} />
        )}
        {pane === "state" && (
          <StatePane entries={stateEntries} status={stateStatus} />
        )}
      </div>
    </div>
  );
}

function TasksPane({
  tasks,
  status,
}: {
  tasks: import("@/lib/types").SwarmTaskUpdate[];
  status: string;
}) {
  return (
    <div>
      <p className="text-xs text-aide-text-muted mb-2">
        Stream: <code>{status}</code> · {tasks.length} updates
      </p>
      <ul className="space-y-1 text-xs">
        {tasks.map((t) => (
          <li
            key={t.id}
            className="flex items-center justify-between px-2 py-1 rounded hover:bg-aide-bg-elevated"
          >
            <span className="flex-1 truncate">
              <span className="font-mono text-aide-text-muted mr-2">
                {t.status}
              </span>
              {t.title}
            </span>
            {t.claimed_by && (
              <span className="ml-2 text-aide-text-muted">
                {t.claimed_by.slice(0, 8)}
              </span>
            )}
          </li>
        ))}
      </ul>
    </div>
  );
}

function MessagesPane({
  messages,
  status,
}: {
  messages: import("@/lib/types").SwarmMessageUpdate[];
  status: string;
}) {
  return (
    <div>
      <p className="text-xs text-aide-text-muted mb-2">
        Stream: <code>{status}</code> · {messages.length} messages
      </p>
      <ul className="space-y-1 text-xs">
        {messages.map((m) => (
          <li
            key={m.id}
            className="px-2 py-1 rounded hover:bg-aide-bg-elevated"
          >
            <div className="flex items-center justify-between text-aide-text-muted">
              <span>
                {m.from} → {m.to || "*"} {m.priority === "high" && "·high"}
              </span>
              <span>{formatTimestamp(m.created_at)}</span>
            </div>
            <div className="mt-0.5">{m.content}</div>
          </li>
        ))}
      </ul>
    </div>
  );
}

function StatePane({
  entries,
  status,
}: {
  entries: import("@/lib/types").SwarmStateUpdate[];
  status: string;
}) {
  return (
    <div>
      <p className="text-xs text-aide-text-muted mb-2">
        Stream: <code>{status}</code> · {entries.length} keys
      </p>
      <table className="w-full text-xs font-mono">
        <thead className="text-aide-text-muted">
          <tr className="border-b border-aide-border">
            <th className="text-left py-1">Key</th>
            <th className="text-left py-1">Value</th>
            <th className="text-left py-1">Updated</th>
          </tr>
        </thead>
        <tbody>
          {entries.map((e) => (
            <tr key={e.key} className="border-b border-aide-border/50">
              <td className="py-1">{e.key}</td>
              <td className="py-1 text-aide-text">{e.value}</td>
              <td className="py-1 text-aide-text-muted">
                {formatTimestamp(e.updated_at)}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
