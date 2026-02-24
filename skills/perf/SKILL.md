---
name: perf
description: Performance analysis and optimization workflow
triggers:
  - "optimize"
  - "performance"
  - "slow"
  - "too slow"
  - "speed up"
  - "make faster"
---

# Performance Mode

**Recommended model tier:** smart (opus) - this skill requires complex reasoning

Systematic approach to identifying and fixing performance issues.

## Prerequisites

Before starting:

- Identify the specific operation or endpoint that is slow
- Understand what "fast enough" means (target latency, throughput)
- Ensure you can measure performance reproducibly

## Workflow

### Step 1: Establish Baseline Measurement

**Never optimize without data.** Measure current performance:

```bash
# Node.js - simple timing
time node script.js

# Node.js - CPU profiling
node --cpu-prof script.js
# Creates CPU.*.cpuprofile - analyze in Chrome DevTools

# Go - benchmarks
go test -bench=. -benchmem ./...

# API endpoint
curl -w "@curl-format.txt" -o /dev/null -s "http://localhost:3000/api/endpoint"
```

**Record baseline metrics:**

- Execution time (p50, p95, p99 if available)
- Memory usage
- Number of operations per second
- Number of I/O operations

### Step 2: Identify Hotspots

Find where time is being spent:

```bash
# Node.js profiling
node --cpu-prof app.js
# Then load .cpuprofile in Chrome DevTools > Performance

# Go profiling
go test -cpuprofile=cpu.prof -bench=.
go tool pprof -http=:8080 cpu.prof
```

```
# Get structural overview of suspect files (signatures + line ranges, not full content)
mcp__plugin_aide_aide__code_outline file="path/to/hotspot.ts"

# Find functions/classes in suspect area by name
mcp__plugin_aide_aide__code_search query="processData" kind="function"

# Find all callers of a hot function
mcp__plugin_aide_aide__code_references symbol="processData"

# Search for expensive patterns in code bodies (Grep is better here)
Grep for ".forEach(", ".map(", ".filter("    # Loop/iteration patterns
Grep for "SELECT", "find(", "query("         # Database queries
Grep for "fetch(", "axios", "http.Get"       # Network calls
Grep for "setTimeout", "setInterval"         # Timers
Grep for "JSON.parse", "JSON.stringify"      # Serialization
```

Note: `code_search` finds function/class/type _definitions_ by name.
For patterns inside function bodies (loops, queries, call chains), use Grep.

After identifying hotspot functions via profiling and search, use `Read` with offset/limit to read
specific functions (use line numbers from `code_outline`).

### Step 3: Analyze Performance Patterns

Look for these common issues:

| Issue                | Pattern                    | Solution              |
| -------------------- | -------------------------- | --------------------- |
| N+1 queries          | Loop containing DB call    | Batch/eager load      |
| Repeated computation | Same calculation in loop   | Memoize/cache         |
| Large allocations    | Creating objects in loop   | Reuse/pool objects    |
| Blocking I/O         | Sync file/network ops      | Make async/concurrent |
| Missing indexes      | Slow DB queries            | Add database indexes  |
| Unnecessary work     | Processing unused data     | Filter/skip early     |
| Serial execution     | Sequential independent ops | Parallelize           |

### Step 4: Apply Optimizations

**Priority order (highest impact first):**

1. **Algorithmic improvements** - O(n^2) -> O(n log n)
2. **Reduce I/O** - Batch requests, add caching
3. **Parallelize** - Concurrent operations
4. **Reduce allocations** - Reuse objects, pre-allocate
5. **Micro-optimizations** - Only as last resort

**Make one optimization at a time and measure after each.**

### Step 5: Measure After Each Change

```bash
# Same measurement as baseline
time node script.js
go test -bench=. -benchmem ./...
```

Compare:

- Did the metric improve?
- By how much (percentage)?
- Any negative side effects?

**If no improvement:** Revert and try different approach.

### Step 6: Verify No Regressions

```bash
# Run all tests
npm test
go test ./...

# Check for correctness
# Ensure output is still correct after optimization
```

## Failure Handling

| Situation                     | Action                                        |
| ----------------------------- | --------------------------------------------- |
| Cannot measure reliably       | Increase sample size, reduce variance sources |
| Optimization made it slower   | Revert, analyze why, profile more carefully   |
| Optimization broke tests      | Fix tests or revert if behavior changed       |
| Bottleneck is external        | Document, consider caching, async processing  |
| Memory improved but CPU worse | Evaluate trade-off for use case               |

## Common Optimizations

### JavaScript/TypeScript

```typescript
// BAD: N+1 queries
for (const user of users) {
  const posts = await db.getPosts(user.id);
}

// GOOD: Batch query
const userIds = users.map((u) => u.id);
const posts = await db.getPostsForUsers(userIds);

// BAD: Repeated work
const typeA = items.filter((x) => x.type === "a").map((x) => x.value);
const typeB = items.filter((x) => x.type === "b").map((x) => x.value);

// GOOD: Single pass
const grouped = { a: [], b: [] };
for (const x of items) {
  if (x.type in grouped) grouped[x.type].push(x.value);
}

// BAD: Serial async
const result1 = await fetch(url1);
const result2 = await fetch(url2);

// GOOD: Parallel async
const [result1, result2] = await Promise.all([fetch(url1), fetch(url2)]);
```

### Go

```go
// BAD: Allocation in loop
var result []T
for _, item := range items {
    result = append(result, process(item))
}

// GOOD: Pre-allocate
result := make([]T, 0, len(items))
for _, item := range items {
    result = append(result, process(item))
}

// BAD: String concatenation
s := ""
for _, item := range items {
    s += item
}

// GOOD: Builder
var b strings.Builder
for _, item := range items {
    b.WriteString(item)
}
```

### SQL

```sql
-- BAD: Missing index on frequently queried column
SELECT * FROM users WHERE email = 'x@y.com';

-- GOOD: Add index
CREATE INDEX idx_users_email ON users(email);

-- BAD: SELECT *
SELECT * FROM users;

-- GOOD: Select only needed columns
SELECT id, name, email FROM users;

-- BAD: Query in loop
-- for each user: SELECT * FROM posts WHERE user_id = ?

-- GOOD: Single batch query
SELECT * FROM posts WHERE user_id IN (?, ?, ?);
```

## MCP Tools

- `mcp__plugin_aide_aide__code_outline` - **Start here.** Get collapsed file skeleton to identify functions before reading
- `mcp__plugin_aide_aide__code_search` - Find function/class/type definitions by name
- `mcp__plugin_aide_aide__code_symbols` - List all symbol definitions in a file
- `mcp__plugin_aide_aide__code_references` - Find all callers of a hot function (exact name match)
- `mcp__plugin_aide_aide__memory_search` - Check past performance decisions
- **Grep** - Find code patterns in bodies: loops, queries, call chains, string literals

## Profiling Commands Reference

### Node.js

```bash
# CPU profiling
node --cpu-prof app.js
# Produces .cpuprofile file

# Memory profiling
node --heap-prof app.js
# Produces .heapprofile file

# Clinic.js for analysis
npx clinic doctor -- node app.js
npx clinic flame -- node app.js
```

### Go

```bash
# CPU profiling
go test -cpuprofile=cpu.prof -bench=.
go tool pprof -http=:8080 cpu.prof

# Memory profiling
go test -memprofile=mem.prof -bench=.
go tool pprof -http=:8080 mem.prof

# Execution trace
go test -trace=trace.out -bench=.
go tool trace trace.out
```

### Browser

- DevTools -> Performance -> Record
- DevTools -> Memory -> Heap snapshot
- Lighthouse for overall page performance

## Verification Criteria

Before completing:

- [ ] Baseline measurement recorded
- [ ] Improvement quantified (percentage)
- [ ] All tests still pass
- [ ] No correctness regressions
- [ ] Memory usage acceptable

## Output Format

```markdown
## Performance Analysis: [Operation/Endpoint Name]

### Baseline

- Execution time: 450ms (p50), 680ms (p95)
- Memory: 125MB peak
- Database queries: 150

### Hotspots Identified

1. `db.getUsers()` - 300ms (67% of total)
2. `processData()` - 100ms (22% of total)
3. `formatOutput()` - 50ms (11% of total)

### Optimizations Applied

1. Batched user queries - 300ms -> 50ms
2. Memoized processData for repeated calls - 100ms -> 5ms

### Results

- Execution time: 450ms -> 105ms (77% faster)
- Memory: 125MB -> 80MB (36% reduction)
- Database queries: 150 -> 3 (98% reduction)

### Verification

- All tests: PASS
- Output correctness: VERIFIED
```
