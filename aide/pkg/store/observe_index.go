package store

import (
	"encoding/binary"
	"encoding/json"
	"time"

	"github.com/jmylchreest/aide/aide/pkg/observe"
	bolt "go.etcd.io/bbolt"
)

var bucketObserveByTime = []byte("observe_by_time")

// Signed seconds followed by nanoseconds preserve time.Time ordering, including
// pre-epoch and zero timestamps. IDs break ties exactly as ListObserveEvents does.
// ULID order alone is insufficient: events can arrive with backdated timestamps.
func observeTimeKey(at time.Time, id string) []byte {
	key := make([]byte, 12+len(id))
	binary.BigEndian.PutUint64(key, uint64(at.Unix())^(1<<63))
	binary.BigEndian.PutUint32(key[8:], uint32(at.Nanosecond()))
	copy(key[12:], id)
	return key
}

func migrateObserveTimeIndex(tx *bolt.Tx) error {
	idx, err := tx.CreateBucketIfNotExists(bucketObserveByTime)
	if err != nil {
		return err
	}
	rows := tx.Bucket(BucketObserveEvents)
	if rows == nil {
		return nil
	}
	return rows.ForEach(func(k, v []byte) error {
		var e observe.Event
		if json.Unmarshal(v, &e) != nil {
			return nil // Match the existing reader's malformed-record handling.
		}
		return idx.Put(observeTimeKey(e.Timestamp, e.ID), k)
	})
}

// putObserveEvent keeps replacement/backdating and the index in one transaction.
// The legacy token migration also uses this path, without changing normalization.
func putObserveEvent(tx *bolt.Tx, e *observe.Event, data []byte) error {
	rows := tx.Bucket(BucketObserveEvents)
	idx := tx.Bucket(bucketObserveByTime)
	id := []byte(e.ID)
	if idx != nil {
		if prior := rows.Get(id); prior != nil {
			var old observe.Event
			if json.Unmarshal(prior, &old) == nil {
				if err := idx.Delete(observeTimeKey(old.Timestamp, old.ID)); err != nil {
					return err
				}
			}
		}
		if err := idx.Put(observeTimeKey(e.Timestamp, e.ID), id); err != nil {
			return err
		}
	}
	return rows.Put(id, data)
}

func deleteObserveEvent(tx *bolt.Tx, id []byte) error {
	rows := tx.Bucket(BucketObserveEvents)
	if idx := tx.Bucket(bucketObserveByTime); idx != nil {
		var e observe.Event
		if json.Unmarshal(rows.Get(id), &e) == nil {
			if err := idx.Delete(observeTimeKey(e.Timestamp, e.ID)); err != nil {
				return err
			}
		}
	}
	return rows.Delete(id)
}
