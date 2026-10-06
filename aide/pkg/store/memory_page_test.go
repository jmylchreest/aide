package store

import (
	"reflect"
	"testing"

	"github.com/jmylchreest/aide/aide/pkg/memory"
)

func TestMemoryPageLimitFollowsAllFilters(t *testing.T) {
	s, cleanup := setupTestDB(t)
	defer cleanup()
	rows := []*memory.Memory{
		{ID: "01", Category: memory.CategoryLearning, Plan: "target", Tags: []string{"keep", "forget"}},
		{ID: "02", Category: memory.CategorySession, Plan: "target", Tags: []string{"keep"}},
		{ID: "03", Category: memory.CategoryLearning, Plan: "other", Tags: []string{"keep"}},
		{ID: "04", Category: memory.CategoryLearning, Plan: "target", Tags: []string{"other"}},
		{ID: "05", Category: memory.CategoryLearning, Plan: "target", Tags: []string{"keep"}},
		{ID: "06", Category: memory.CategoryLearning, Plan: "target", Tags: []string{"keep"}},
		{ID: "07", Category: memory.CategoryLearning, Plan: "target", Tags: []string{"keep"}},
	}
	for _, row := range rows {
		row.Content = "synthetic page fixture"
		if err := s.AddMemory(row); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct {
		name  string
		limit int
		all   bool
		want  []string
	}{
		{"limited", 2, false, []string{"05", "06"}},
		{"zero unlimited", 0, false, []string{"05", "06", "07"}},
		{"negative unlimited", -1, false, []string{"05", "06", "07"}},
		{"include forgotten", 2, true, []string{"01", "05"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := s.ListMemories(memory.SearchOptions{Category: memory.CategoryLearning, Plan: "target", Tags: []string{"keep"}, Limit: tc.limit, IncludeAll: tc.all})
			if err != nil {
				t.Fatal(err)
			}
			ids := make([]string, len(got))
			for i, row := range got {
				ids[i] = row.ID
			}
			if !reflect.DeepEqual(ids, tc.want) {
				t.Fatalf("got %v, want %v", ids, tc.want)
			}
		})
	}
}
