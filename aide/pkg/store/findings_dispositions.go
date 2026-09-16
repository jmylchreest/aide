package store

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/jmylchreest/aide/aide/pkg/checkout"
	"github.com/jmylchreest/aide/aide/pkg/findings"
	bolt "go.etcd.io/bbolt"
)

var bucketFindingDispositions = []byte("finding_dispositions")

func dispositionKey(checkoutID string, f *findings.Finding) []byte {
	evidence := *f
	evidence.ID = ""
	evidence.CreatedAt = time.Time{}
	evidence.Accepted = false
	data, _ := json.Marshal(evidence)
	return []byte(fmt.Sprintf("%s:%x", checkoutID, sha256.Sum256(data)))
}

func localBolt(st Store) *BoltStore {
	switch s := st.(type) {
	case *BoltStore:
		return s
	case *CombinedStore:
		return s.Bolt()
	}
	return nil
}

func (s *BoltStore) saveDisposition(c checkout.Info, f *findings.Finding) error {
	data, err := json.Marshal(struct {
		Checkout   checkout.Info     `json:"checkout"`
		Finding    *findings.Finding `json:"finding"`
		AcceptedAt time.Time         `json:"accepted_at"`
	}{c, f, time.Now()})
	if err != nil {
		return err
	}
	return s.db.Update(func(tx *bolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists(bucketFindingDispositions)
		if err != nil {
			return err
		}
		key := dispositionKey(c.ID, f)
		if b.Get(key) != nil {
			return nil
		}
		return b.Put(key, data)
	})
}

func (s *FindingsStoreImpl) applyDispositions(ff []*findings.Finding) error {
	if s.dispositions == nil {
		return nil
	}
	return s.dispositions.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketFindingDispositions)
		if b == nil {
			return nil
		}
		for _, f := range ff {
			if b.Get(dispositionKey(s.checkout.ID, f)) != nil {
				f.Accepted = true
			}
		}
		return nil
	})
}

// NewCheckoutFindingsStore keeps human acceptance in the shared memory database.
// Generated finding IDs can change on analysis; identical evidence in the same
// checkout keeps its disposition. Another checkout does not inherit acceptance.
func NewCheckoutFindingsStore(dir string, st Store, c checkout.Info) (*FindingsStoreImpl, error) {
	fs, err := NewFindingsStore(dir)
	if err != nil {
		return nil, err
	}
	fs.dispositions = localBolt(st)
	fs.checkout = c
	if st != nil && fs.dispositions == nil {
		fs.Close()
		return nil, fmt.Errorf("checkout findings require the shared database owner")
	}
	// Older cache versions may contain human acceptance not yet archived.
	all, err := fs.ListFindings(findings.SearchOptions{IncludeAccepted: true, Limit: -1})
	if err != nil {
		fs.Close()
		return nil, err
	}
	if fs.dispositions != nil {
		for _, f := range all {
			if f.Accepted {
				if err := fs.dispositions.saveDisposition(c, f); err != nil {
					fs.Close()
					return nil, err
				}
			}
		}
	}
	return fs, nil
}

// ArchiveLegacyFindings preserves every accepted legacy record before those
// generated stores are retired. Its origin is unknown: do not infer applicability.
func ArchiveLegacyFindings(dbPath string, st Store) error {
	return archiveFindingsFile(filepath.Join(filepath.Dir(dbPath), "findings", "findings.db"), st, checkout.Info{ID: "legacy", Root: ProjectRootFromDB(dbPath)})
}

func archiveFindingsFile(path string, st Store, c checkout.Info) error {
	shared := localBolt(st)
	if shared == nil {
		return fmt.Errorf("migration requires shared database owner")
	}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return err
	}
	db, err := bolt.Open(path, 0600, &bolt.Options{ReadOnly: true, Timeout: time.Second})
	if err != nil {
		return err
	}
	defer db.Close()
	return db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(BucketFindings)
		if b == nil {
			return nil
		}
		return b.ForEach(func(_, v []byte) error {
			var f findings.Finding
			if err := json.Unmarshal(v, &f); err != nil {
				return err
			}
			if !f.Accepted {
				return nil
			}
			return shared.saveDisposition(c, &f)
		})
	})
}
