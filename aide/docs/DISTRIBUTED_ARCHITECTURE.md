# Distributed AIDE Architecture

## Overview

AIDE currently operates as a local-first system with all data stored in BBolt (`.aide/memory/store.db`). This document outlines how the existing gRPC interface could be extended to support distributed deployments.

## Current Architecture

```
┌─────────────────────────────────────────────────────┐
│                   Claude Code                        │
├─────────────────────────────────────────────────────┤
│  Hooks (TypeScript)    │    MCP Server (aide mcp)   │
│  - session-start       │    - memory_add            │
│  - keyword-detector    │    - memory_search         │
│  - hud-updater         │    - decision_set          │
│  - etc.                │    - state_get/set         │
└────────────┬───────────┴────────────┬───────────────┘
             │                        │
             │    CLI / gRPC          │
             ▼                        ▼
┌─────────────────────────────────────────────────────┐
│                  aide binary                         │
│  ┌─────────────────────────────────────────────┐    │
│  │              Store Layer (pkg/store)         │    │
│  │  - Memory (with bleve full-text search)     │    │
│  │  - Decisions (append-only log)              │    │
│  │  - State (session/agent key-value)          │    │
│  │  - Messages (inter-agent queue)             │    │
│  │  - Tasks (swarm coordination)               │    │
│  └─────────────────────────────────────────────┘    │
│                        │                             │
│                        ▼                             │
│  ┌─────────────────────────────────────────────┐    │
│  │           BBolt Database (local)             │    │
│  │         .aide/memory/store.db                │    │
│  └─────────────────────────────────────────────┘    │
└─────────────────────────────────────────────────────┘
```

## Distributed Options

### Option 1: Centralized Server

A single aide-server instance serves multiple clients over gRPC.

```
┌──────────────┐  ┌──────────────┐  ┌──────────────┐
│  Machine A   │  │  Machine B   │  │  Machine C   │
│  Claude Code │  │  Claude Code │  │  Claude Code │
│     │        │  │     │        │  │     │        │
│  aide client │  │  aide client │  │  aide client │
└──────┬───────┘  └──────┬───────┘  └──────┬───────┘
       │                 │                 │
       └────────────┬────┴────────┬────────┘
                    │  gRPC/TLS   │
                    ▼             ▼
         ┌─────────────────────────────────┐
         │        aide-server              │
         │   (centralized instance)        │
         │                                 │
         │  ┌───────────────────────────┐  │
         │  │   PostgreSQL / SQLite     │  │
         │  │   + pgvector for search   │  │
         │  └───────────────────────────┘  │
         └─────────────────────────────────┘
```

**Pros:**
- Simple to implement
- Single source of truth
- Easy backup/restore

**Cons:**
- Single point of failure
- Latency for remote clients
- Requires network connectivity

**Implementation:**
1. Add `--remote=host:port` flag to aide CLI
2. When remote is set, forward all operations over gRPC
3. Server authenticates via API keys or mTLS
4. Server uses PostgreSQL with pgvector for scalable search

### Option 2: Raft Consensus Cluster

Multiple aide nodes form a Raft cluster for high availability.

```
                    ┌─────────────────┐
                    │   aide-node-1   │
                    │    (Leader)     │
                    └────────┬────────┘
                             │
              ┌──────────────┼──────────────┐
              │              │              │
              ▼              ▼              ▼
       ┌─────────────┐ ┌─────────────┐ ┌─────────────┐
       │ aide-node-2 │ │ aide-node-3 │ │ aide-node-4 │
       │ (Follower)  │ │ (Follower)  │ │ (Follower)  │
       └─────────────┘ └─────────────┘ └─────────────┘
              │              │              │
              ▼              ▼              ▼
       ┌─────────────┐ ┌─────────────┐ ┌─────────────┐
       │  Machine A  │ │  Machine B  │ │  Machine C  │
       │ Claude Code │ │ Claude Code │ │ Claude Code │
       └─────────────┘ └─────────────┘ └─────────────┘
```

**Pros:**
- High availability (survives node failures)
- Automatic leader election
- Strong consistency guarantees

**Cons:**
- Complex to operate
- Requires 3+ nodes for quorum
- Overkill for most use cases

**Implementation:**
1. Integrate hashicorp/raft or etcd/raft
2. Replicate all writes through Raft log
3. Reads can be served from any node (with stale option)
4. Use BoltDB as Raft's stable storage (already familiar)

### Option 3: Hybrid (Recommended)

Local-first with optional sync to central server.

```
┌─────────────────────────────────────────────────────┐
│                    Machine A                         │
│  ┌─────────────┐     ┌─────────────────────────┐    │
│  │ Claude Code │────▶│ aide (local)            │    │
│  └─────────────┘     │  .aide/memory/store.db  │    │
│                      └───────────┬─────────────┘    │
└──────────────────────────────────┼──────────────────┘
                                   │ sync (background)
                                   ▼
                    ┌─────────────────────────────────┐
                    │         aide-sync-server        │
                    │                                 │
                    │  - Receives memory/decision     │
                    │    updates from clients         │
                    │  - Merges with CRDT semantics   │
                    │  - Pushes updates to clients    │
                    │                                 │
                    └─────────────────────────────────┘
                                   ▲
                                   │ sync (background)
┌──────────────────────────────────┼──────────────────┐
│                    Machine B     │                   │
│  ┌─────────────┐     ┌───────────┴─────────────┐    │
│  │ Claude Code │────▶│ aide (local)            │    │
│  └─────────────┘     │  .aide/memory/store.db  │    │
│                      └─────────────────────────┘    │
└─────────────────────────────────────────────────────┘
```

**Pros:**
- Works offline (local-first)
- Low latency (reads/writes are local)
- Sync happens in background
- Graceful degradation

**Cons:**
- Eventual consistency (conflicts possible)
- Need CRDT or conflict resolution strategy

**Implementation:**
1. Each memory/decision gets a vector clock or hybrid logical clock
2. Sync protocol:
   - Client sends changes since last sync
   - Server merges and returns merged state
   - Client applies remote changes
3. Conflict resolution:
   - Memories: Last-write-wins (timestamps)
   - Decisions: Append-only, no conflicts
   - State: Last-write-wins per key
   - Messages: Merge (union of message sets)

## Configuration

```yaml
# ~/.aide/config.yaml
sync:
  enabled: true
  server: "https://aide.example.com:8443"
  api_key: "ak_..."
  interval: 60s  # sync every 60 seconds

  # Or for Raft cluster:
  # mode: raft
  # peers:
  #   - "node1.example.com:8443"
  #   - "node2.example.com:8443"
  #   - "node3.example.com:8443"
```

## Protocol Buffers Extension

```protobuf
// aide/proto/sync.proto

message SyncRequest {
  string client_id = 1;
  int64 last_sync_timestamp = 2;
  repeated MemoryChange memory_changes = 3;
  repeated DecisionChange decision_changes = 4;
  repeated StateChange state_changes = 5;
}

message SyncResponse {
  int64 server_timestamp = 1;
  repeated Memory new_memories = 2;
  repeated Decision new_decisions = 3;
  repeated StateEntry state_updates = 4;
  repeated string deleted_memory_ids = 5;
}

message MemoryChange {
  string id = 1;
  string content = 2;
  string category = 3;
  repeated string tags = 4;
  int64 timestamp = 5;
  bool deleted = 6;
}
```

## Security Considerations

1. **Authentication**: API keys or mTLS for client authentication
2. **Authorization**: Per-project access control
3. **Encryption**: TLS for transport, optional encryption at rest
4. **Audit**: Log all sync operations for compliance

## Migration Path

1. **Phase 1** (Current): Local-only operation
2. **Phase 2**: Add `--remote` flag for centralized server
3. **Phase 3**: Add background sync daemon (`aide sync start`)
4. **Phase 4**: Optional Raft mode for enterprise deployments

## TODO

- [ ] Define sync protocol in detail
- [ ] Implement `aide daemon --listen` for server mode
- [ ] Add `aide sync` subcommand for manual sync
- [ ] Design conflict resolution UI for edge cases
- [ ] Benchmark sync performance with large datasets
- [ ] Consider WebSocket for real-time sync
