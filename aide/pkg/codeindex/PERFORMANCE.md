# Checkout index benchmark summary

Use independent checkout stores with bulk writes and optional verified record
seeding. This preserves ordinary queries and simple cleanup, without parent-store
dependencies. Seeding verifies source content and parser compatibility; it still
builds a complete destination index, so storage is not deduplicated.

Spike measurements: Linux/amd64, NVMe/ext4, 323 tracked Go files with 32 changed;
one sample per method. Times cover index creation, excluding Git worktree creation.
These are historical comparisons, not production latency guarantees.

| Index creation approach | Time |
| --- | ---: |
| Per-file rebuild | 4.25 s |
| Bulk rebuild | 1.38 s |
| Bulk verified logical seed (chosen) | 0.64 s |
| Physical snapshot + bulk reconciliation | 0.27 s |

Snapshots were faster but required source quiescence and coordinated copying.
Shared-content/COW prototypes saved storage but added query filtering and version
GC. Independent stores best matched the priority of simplicity before creation
speed and storage savings. Profiling identified Bleve indexing/merging and
allocation costs as reasons to batch writes.

Production benchmarks remain in `index_test.go`. From `aide/`, run
`go test ./pkg/codeindex -run '^$' -bench BenchmarkCheckoutCreate -benchmem`.
Use scratch space on the filesystem being measured; keep raw results and pprof
artifacts outside Git.
