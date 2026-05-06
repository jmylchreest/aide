package store

import (
	"time"

	bolt "go.etcd.io/bbolt"
)

// indexerBoltOptions returns bbolt open options tuned for aide's write-heavy
// stores. Three departures from the bbolt defaults:
//
//   - NoFreelistSync skips one of the two fdatasync calls per Tx.Commit; the
//     freelist is rebuilt from page metadata at next open, so a crash extends
//     startup by milliseconds rather than corrupting data.
//   - FreelistMapType (vs the default array) is faster under fragmentation;
//     the only trade-off is non-deterministic page allocation order, which
//     does not matter for our access patterns.
//   - InitialMmapSize 256 MiB lets read transactions proceed without blocking
//     on a remap as the file grows. Address-space only, not disk allocation.
func indexerBoltOptions(timeout time.Duration) *bolt.Options {
	return &bolt.Options{
		Timeout:         timeout,
		NoFreelistSync:  true,
		FreelistType:    bolt.FreelistMapType,
		InitialMmapSize: 256 << 20,
	}
}
