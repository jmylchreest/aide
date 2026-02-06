# AIDE Design Documents

This directory contains design documents for planned and in-progress AIDE features.

## Documents

| Document | Status | Description |
|----------|--------|-------------|
| [DISTRIBUTED_ARCHITECTURE.md](./DISTRIBUTED_ARCHITECTURE.md) | Planning | gRPC distributed mode, Raft consensus, sync protocols |
| [AUTOMATIC_MEMORY_CAPTURE.md](./AUTOMATIC_MEMORY_CAPTURE.md) | Planning | Auto-capture session turns like Supermemory |
| [SESSION_SUMMARIZATION.md](./SESSION_SUMMARIZATION.md) | Planning | Claude Haiku-powered session summaries |
| [CODEBASE_INDEXING.md](./CODEBASE_INDEXING.md) | Planning | Tree-sitter based symbol indexing |

## Implementation Priority

1. **Session Summarization** - Low effort, high value
2. **Automatic Memory Capture** - Medium effort, high value
3. **Codebase Indexing** - High effort, high value
4. **Distributed Architecture** - High effort, niche value

## Contributing

When adding new design docs:
1. Use clear markdown with diagrams where helpful
2. Include implementation steps
3. Consider alternatives and trade-offs
4. Add TODO items for tracking
