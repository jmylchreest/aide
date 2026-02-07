package store

import (
	"encoding/json"
	"log"
	"time"

	"github.com/jmylchreest/aide/aide/pkg/memory"
	bolt "go.etcd.io/bbolt"
)

// AddTask creates a new task.
func (s *BoltStore) AddTask(t *memory.Task) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(BucketTasks)
		data, err := json.Marshal(t)
		if err != nil {
			return err
		}
		return b.Put([]byte(t.ID), data)
	})
}

// CreateTask is an alias for AddTask.
func (s *BoltStore) CreateTask(t *memory.Task) error {
	return s.AddTask(t)
}

// GetTask retrieves a task by ID.
func (s *BoltStore) GetTask(id string) (*memory.Task, error) {
	var t memory.Task
	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(BucketTasks)
		data := b.Get([]byte(id))
		if data == nil {
			return ErrNotFound
		}
		return json.Unmarshal(data, &t)
	})
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// ClaimTask atomically claims a task for an agent.
func (s *BoltStore) ClaimTask(taskID, agentID string) (*memory.Task, error) {
	var task memory.Task

	err := s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(BucketTasks)
		data := b.Get([]byte(taskID))
		if data == nil {
			return ErrNotFound
		}

		if err := json.Unmarshal(data, &task); err != nil {
			return err
		}

		if task.Status != memory.TaskStatusPending {
			return ErrAlreadyClaimed
		}

		task.Status = memory.TaskStatusClaimed
		task.ClaimedBy = agentID
		task.ClaimedAt = time.Now()

		data, err := json.Marshal(task)
		if err != nil {
			return err
		}
		return b.Put([]byte(taskID), data)
	})

	if err != nil {
		return nil, err
	}
	return &task, nil
}

// CompleteTask marks a task as done with a result.
func (s *BoltStore) CompleteTask(taskID, result string) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(BucketTasks)
		data := b.Get([]byte(taskID))
		if data == nil {
			return ErrNotFound
		}

		var task memory.Task
		if err := json.Unmarshal(data, &task); err != nil {
			return err
		}

		task.Status = memory.TaskStatusDone
		task.CompletedAt = time.Now()
		task.Result = result

		data, err := json.Marshal(task)
		if err != nil {
			return err
		}
		return b.Put([]byte(taskID), data)
	})
}

// UpdateTask updates an existing task.
func (s *BoltStore) UpdateTask(t *memory.Task) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(BucketTasks)
		// Verify task exists
		if data := b.Get([]byte(t.ID)); data == nil {
			return ErrNotFound
		}
		data, err := json.Marshal(t)
		if err != nil {
			return err
		}
		return b.Put([]byte(t.ID), data)
	})
}

// ListTasks returns tasks matching the given status.
func (s *BoltStore) ListTasks(status memory.TaskStatus) ([]*memory.Task, error) {
	var tasks []*memory.Task

	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(BucketTasks)
		return b.ForEach(func(k, v []byte) error {
			var t memory.Task
			if err := json.Unmarshal(v, &t); err != nil {
				log.Printf("store: skipping malformed task entry: %v", err)
				return nil
			}
			if status == "" || t.Status == status {
				tasks = append(tasks, &t)
			}
			return nil
		})
	})

	return tasks, err
}

// DeleteTask removes a task by ID.
func (s *BoltStore) DeleteTask(id string) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(BucketTasks)
		return b.Delete([]byte(id))
	})
}

// ClearTasks removes tasks matching the given status.
// If status is empty, removes all tasks.
func (s *BoltStore) ClearTasks(status memory.TaskStatus) (int, error) {
	var deleted int

	err := s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(BucketTasks)

		// Collect keys to delete (can't delete while iterating)
		var keysToDelete [][]byte
		err := b.ForEach(func(k, v []byte) error {
			if status == "" {
				keysToDelete = append(keysToDelete, k)
				return nil
			}

			var t memory.Task
			if err := json.Unmarshal(v, &t); err != nil {
				return nil // skip malformed
			}
			if t.Status == status {
				keysToDelete = append(keysToDelete, k)
			}
			return nil
		})
		if err != nil {
			return err
		}

		// Delete collected keys
		for _, k := range keysToDelete {
			if err := b.Delete(k); err != nil {
				return err
			}
			deleted++
		}
		return nil
	})

	return deleted, err
}
