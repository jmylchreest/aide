package store

import (
	"encoding/json"
	"log"
	"time"

	"github.com/jmylchreest/aide/aide/pkg/memory"
	bolt "go.etcd.io/bbolt"
)

// DefaultMessageTTL is the default time-to-live for messages (1 hour).
const DefaultMessageTTL = 1 * time.Hour

// AddMessage stores a new message with optional TTL.
func (s *BoltStore) AddMessage(m *memory.Message) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(BucketMessages)

		// Auto-increment ID.
		id, err := b.NextSequence()
		if err != nil {
			return err
		}
		m.ID = id

		// Set CreatedAt if not set.
		if m.CreatedAt.IsZero() {
			m.CreatedAt = time.Now()
		}

		// Set default TTL if not specified.
		if m.ExpiresAt.IsZero() {
			m.ExpiresAt = m.CreatedAt.Add(DefaultMessageTTL)
		}

		data, err := json.Marshal(m)
		if err != nil {
			return err
		}
		return b.Put(itob(id), data)
	})
}

// GetMessages retrieves unread messages for an agent (prunes expired first).
func (s *BoltStore) GetMessages(agentID string) ([]*memory.Message, error) {
	// Prune expired messages first.
	_, _ = s.PruneMessages()

	var messages []*memory.Message

	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(BucketMessages)
		return b.ForEach(func(k, v []byte) error {
			var m memory.Message
			if err := json.Unmarshal(v, &m); err != nil {
				log.Printf("store: skipping malformed message entry: %v", err)
				return nil
			}
			// Include if broadcast (empty To) or addressed to this agent.
			if m.To == "" || m.To == agentID {
				messages = append(messages, &m)
			}
			return nil
		})
	})

	return messages, err
}

// AckMessage marks a message as read by an agent.
func (s *BoltStore) AckMessage(messageID uint64, agentID string) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(BucketMessages)
		data := b.Get(itob(messageID))
		if data == nil {
			return ErrNotFound
		}

		var m memory.Message
		if err := json.Unmarshal(data, &m); err != nil {
			return err
		}

		// Add to ReadBy if not already there.
		for _, r := range m.ReadBy {
			if r == agentID {
				return nil // Already acked.
			}
		}
		m.ReadBy = append(m.ReadBy, agentID)

		data, err := json.Marshal(m)
		if err != nil {
			return err
		}
		return b.Put(itob(messageID), data)
	})
}

// PruneMessages removes expired messages.
func (s *BoltStore) PruneMessages() (int, error) {
	var pruned int
	var toDelete [][]byte
	now := time.Now()

	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(BucketMessages)
		return b.ForEach(func(k, v []byte) error {
			var m memory.Message
			if err := json.Unmarshal(v, &m); err != nil {
				log.Printf("store: skipping malformed message entry: %v", err)
				return nil
			}
			if !m.ExpiresAt.IsZero() && m.ExpiresAt.Before(now) {
				toDelete = append(toDelete, k)
			}
			return nil
		})
	})
	if err != nil {
		return 0, err
	}

	if len(toDelete) > 0 {
		err = s.db.Update(func(tx *bolt.Tx) error {
			b := tx.Bucket(BucketMessages)
			for _, k := range toDelete {
				if err := b.Delete(k); err != nil {
					return err
				}
				pruned++
			}
			return nil
		})
	}

	return pruned, err
}

// ClearMessages removes all messages for an agent (or all if empty).
func (s *BoltStore) ClearMessages(agentID string) (int, error) {
	var cleared int
	var toDelete [][]byte

	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(BucketMessages)
		return b.ForEach(func(k, v []byte) error {
			if agentID == "" {
				toDelete = append(toDelete, k)
				return nil
			}
			var m memory.Message
			if err := json.Unmarshal(v, &m); err != nil {
				log.Printf("store: skipping malformed message entry: %v", err)
				return nil
			}
			if m.To == agentID || m.From == agentID {
				toDelete = append(toDelete, k)
			}
			return nil
		})
	})
	if err != nil {
		return 0, err
	}

	if len(toDelete) > 0 {
		err = s.db.Update(func(tx *bolt.Tx) error {
			b := tx.Bucket(BucketMessages)
			for _, k := range toDelete {
				if err := b.Delete(k); err != nil {
					return err
				}
				cleared++
			}
			return nil
		})
	}

	return cleared, err
}
