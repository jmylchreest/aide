package store

import (
	"time"

	bolt "go.etcd.io/bbolt"
)

// indexerBoltOptions returns bbolt open options tuned for aide's write-heavy
// stores. Two departures from the bbolt defaults:
//
//   - FreelistMapType (vs the default array) is faster under fragmentation;
//     the only trade-off is non-deterministic page allocation order, which
//     does not matter for our access patterns. Has no open-time cost.
//   - InitialMmapSize 256 MiB lets read transactions proceed without blocking
//     on a remap as the file grows. Address-space only, not disk allocation.
//
// NoFreelistSync was deliberately *not* enabled. It looks attractive (drops
// one of bbolt's two fdatasyncs per commit) but its open-time cost is
// catastrophic on large databases: every fresh Open has to re-derive the
// freelist by walking every page in the file. For a multi-gigabyte code
// index that walk takes tens of minutes, during which the daemon's lazy
// init is blocked and every gRPC request returns "code store not available".
// The fast-commits-vs-fast-opens trade-off is the wrong choice for a daemon
// that restarts more than once a week.
func indexerBoltOptions(timeout time.Duration) *bolt.Options {
	return &bolt.Options{
		Timeout:         timeout,
		FreelistType:    bolt.FreelistMapType,
		InitialMmapSize: 256 << 20,
	}
}
