package store

import (
	"encoding/json"
	"time"

	"github.com/jmylchreest/aide/aide/pkg/instinct"
	"github.com/oklog/ulid/v2"
	bolt "go.etcd.io/bbolt"
)

// InstinctFilter narrows ListInstinctProposals results. Zero-value fields
// match anything.
type InstinctFilter struct {
	Status    instinct.Status
	Shape     string
	SessionID string
	Since     time.Time
	Until     time.Time
	Limit     int
}

func (s *BoltStore) AddInstinctProposal(p *instinct.Proposal) error {
	if p.ID == "" {
		p.ID = ulid.Make().String()
	}
	if p.ProposedAt.IsZero() {
		p.ProposedAt = time.Now()
	}
	if p.Status == "" {
		p.Status = instinct.StatusOpen
	}
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(BucketInstinctProposals)
		data, err := json.Marshal(p)
		if err != nil {
			return err
		}
		return b.Put([]byte(p.ID), data)
	})
}

func (s *BoltStore) GetInstinctProposal(id string) (*instinct.Proposal, error) {
	var p *instinct.Proposal
	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(BucketInstinctProposals)
		data := b.Get([]byte(id))
		if data == nil {
			return nil
		}
		p = new(instinct.Proposal)
		return json.Unmarshal(data, p)
	})
	return p, err
}

// ListInstinctProposals returns proposals newest-first. A non-positive Limit
// returns all matches.
func (s *BoltStore) ListInstinctProposals(f InstinctFilter) ([]*instinct.Proposal, error) {
	var out []*instinct.Proposal
	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(BucketInstinctProposals)
		c := b.Cursor()
		for k, v := c.Last(); k != nil; k, v = c.Prev() {
			var p instinct.Proposal
			if err := json.Unmarshal(v, &p); err != nil {
				continue
			}
			if !f.Since.IsZero() && p.ProposedAt.Before(f.Since) {
				break
			}
			if !f.Until.IsZero() && p.ProposedAt.After(f.Until) {
				continue
			}
			if f.Status != "" && p.Status != f.Status {
				continue
			}
			if f.Shape != "" && p.Shape != f.Shape {
				continue
			}
			if f.SessionID != "" && p.SessionID != f.SessionID {
				continue
			}
			out = append(out, &p)
			if f.Limit > 0 && len(out) >= f.Limit {
				break
			}
		}
		return nil
	})
	return out, err
}

// UpdateInstinctProposalStatus changes a proposal's status and sets the
// fields that go with the transition. Returns the updated record.
func (s *BoltStore) UpdateInstinctProposalStatus(id string, status instinct.Status, reason string, acceptedMemoryID string) (*instinct.Proposal, error) {
	var updated *instinct.Proposal
	err := s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(BucketInstinctProposals)
		data := b.Get([]byte(id))
		if data == nil {
			return nil
		}
		var p instinct.Proposal
		if err := json.Unmarshal(data, &p); err != nil {
			return err
		}
		p.Status = status
		switch status {
		case instinct.StatusAccepted:
			p.AcceptedMemoryID = acceptedMemoryID
		case instinct.StatusRejected:
			p.RejectionCount++
			p.RejectionReason = reason
			p.LastReproposalAt = time.Now()
		}
		out, err := json.Marshal(&p)
		if err != nil {
			return err
		}
		updated = &p
		return b.Put([]byte(id), out)
	})
	return updated, err
}

// CleanupInstinctProposals expires open proposals past their ExpiresAt and
// deletes rejected proposals older than rejectedTTL. Returns (expired, deleted).
func (s *BoltStore) CleanupInstinctProposals(rejectedTTL time.Duration) (int, int, error) {
	now := time.Now()
	rejectCutoff := now.Add(-rejectedTTL)
	type op struct {
		key    []byte
		action string
		p      instinct.Proposal
	}
	var ops []op
	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(BucketInstinctProposals)
		return b.ForEach(func(k, v []byte) error {
			var p instinct.Proposal
			if err := json.Unmarshal(v, &p); err != nil {
				return nil
			}
			switch {
			case p.Status == instinct.StatusOpen && !p.ExpiresAt.IsZero() && now.After(p.ExpiresAt):
				ops = append(ops, op{key: append([]byte{}, k...), action: "expire", p: p})
			case p.Status == instinct.StatusRejected && !p.LastReproposalAt.IsZero() && p.LastReproposalAt.Before(rejectCutoff):
				ops = append(ops, op{key: append([]byte{}, k...), action: "delete"})
			}
			return nil
		})
	})
	if err != nil {
		return 0, 0, err
	}
	expired, deleted := 0, 0
	err = s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(BucketInstinctProposals)
		for _, o := range ops {
			switch o.action {
			case "expire":
				o.p.Status = instinct.StatusExpired
				out, mErr := json.Marshal(&o.p)
				if mErr != nil {
					continue
				}
				if pErr := b.Put(o.key, out); pErr == nil {
					expired++
				}
			case "delete":
				if dErr := b.Delete(o.key); dErr == nil {
					deleted++
				}
			}
		}
		return nil
	})
	return expired, deleted, err
}
