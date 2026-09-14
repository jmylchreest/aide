# Checkout index performance

Use independent checkout stores, bounded bulk writes, and verified logical
seeding. This keeps one shared daemon and memory store without overlay reads,
tombstones, parent lifetime dependencies, or filesystem-specific reflinks.
Seeding copies records only when relative path, source hash, and parser/grammar
fingerprint match. It still writes a complete destination Bolt/Bleve index;
there is no storage deduplication between checkouts.

Measured 2026-09-14 on Linux/amd64, NVMe/ext4, with warmed filesystem cache.
The fixture has 128 Go files, 32 functions per file, and 12 changed files in
the destination. Figures are medians of three samples, three operations each;
CPU and allocation profiling were enabled. These are synthetic comparisons,
not production latency guarantees. Creation includes opening the destination
stores, reading/verifying files, parsing or seeding, and writing both indexes;
it excludes Git worktree creation and store close.

| Operation | Time | Allocated bytes/op |
| --- | ---: | ---: |
| Cold, per-file writes | 1.751 s | 962 MB |
| Cold, bulk writes | 319.8 ms | 170 MB |
| Bulk, 116 verified files seeded | 196.9 ms | 171 MB |
| Reconcile unchanged checkout | 3.56 ms | 2.52 MB |
| Symbol query | 1.12 ms | 0.275 MB |

Bulk writes were 5.5× faster than per-file writes. Seeding reduced bulk
creation time by another 38%, but did not reduce allocation volume: destination
search indexing still happens. The combined CPU profile attributes 13.4% of
samples directly to cgo calls; allocation leaders include Bleve/Vellum index
construction and buffering. The profile includes all benchmark modes and setup,
so it cannot attribute those costs exclusively to seeded creation.

Reproduce from `aide/`, setting `TMPDIR` to scratch space on the filesystem being
measured (the default `/tmp` may be tmpfs):

```sh
go test ./pkg/codeindex -run '^$' -bench BenchmarkCheckoutCreate \
  -benchtime=3x -count=3 -benchmem \
  -cpuprofile=/path/to/ignored/cpu.pprof \
  -memprofile=/path/to/ignored/heap.pprof -o /path/to/ignored/codeindex.test
go tool pprof -top /path/to/ignored/cpu.pprof
go tool pprof -top -alloc_space /path/to/ignored/heap.pprof
```

Raw measurements, profiles, and benchmark executables belong in ignored scratch
space. The daemon also retains its existing configurable pprof endpoint.
