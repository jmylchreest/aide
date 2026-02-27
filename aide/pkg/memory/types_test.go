package memory

import (
	"encoding/json"
	"testing"
	"time"
)

func TestMemoryJSON(t *testing.T) {
	now := time.Now()
	m := Memory{
		ID:           "test-1",
		Category:     CategoryLearning,
		Content:      "Test content",
		Tags:         []string{"tag1", "tag2"},
		Priority:     0.8,
		Plan:         "test-plan",
		Agent:        "test-agent",
		AccessCount:  5,
		LastAccessed: now,
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	data, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var decoded Memory
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if decoded.ID != m.ID {
		t.Errorf("ID mismatch: got %s, want %s", decoded.ID, m.ID)
	}
	if decoded.Category != m.Category {
		t.Errorf("Category mismatch: got %s, want %s", decoded.Category, m.Category)
	}
	if decoded.Content != m.Content {
		t.Errorf("Content mismatch: got %s, want %s", decoded.Content, m.Content)
	}
	if decoded.AccessCount != m.AccessCount {
		t.Errorf("AccessCount mismatch: got %d, want %d", decoded.AccessCount, m.AccessCount)
	}
	if decoded.LastAccessed.IsZero() {
		t.Error("LastAccessed should not be zero after round-trip")
	}
}

func TestTaskStatusTransitions(t *testing.T) {
	tests := []struct {
		name   string
		status TaskStatus
		valid  bool
	}{
		{"pending", TaskStatusPending, true},
		{"claimed", TaskStatusClaimed, true},
		{"done", TaskStatusDone, true},
		{"blocked", TaskStatusBlocked, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			task := Task{
				ID:     "test",
				Status: tt.status,
			}

			data, err := json.Marshal(task)
			if err != nil {
				t.Fatalf("failed to marshal: %v", err)
			}

			var decoded Task
			if err := json.Unmarshal(data, &decoded); err != nil {
				t.Fatalf("failed to unmarshal: %v", err)
			}

			if decoded.Status != tt.status {
				t.Errorf("status mismatch: got %s, want %s", decoded.Status, tt.status)
			}
		})
	}
}

func TestCategoryConstants(t *testing.T) {
	categories := []Category{
		CategoryLearning,
		CategoryDecision,
		CategoryIssue,
		CategoryDiscovery,
		CategoryBlocker,
	}

	seen := make(map[Category]bool)
	for _, c := range categories {
		if seen[c] {
			t.Errorf("duplicate category: %s", c)
		}
		seen[c] = true

		if c == "" {
			t.Error("empty category constant")
		}
	}
}
