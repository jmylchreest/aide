package main

import (
	"strings"
	"testing"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/jmylchreest/aide/aide/pkg/memory"
)

// The category list a model sees is generated from memory.AllCategories.
// If the override silently returns nil the SDK falls back to the struct tag,
// which says nothing about which categories exist — so assert it applied.
func TestCategoryInputSchemaEnumeratesAllCategories(t *testing.T) {
	for _, tc := range []struct {
		name   string
		schema any
	}{
		{"memory_search", categoryInputSchema[MemorySearchInput]("Filter by category:")},
		{"memory_list", categoryInputSchema[MemoryListInput]("Filter by category:")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, ok := tc.schema.(*jsonschema.Schema)
			if !ok || s == nil {
				t.Fatal("schema override did not apply; the SDK would fall back to the struct tag")
			}
			prop := s.Properties["category"]
			if prop == nil {
				t.Fatal("no category property in the generated schema")
			}
			for _, info := range memory.AllCategories {
				if !strings.Contains(prop.Description, string(info.Category)) {
					t.Errorf("category %q missing from the description", info.Category)
				}
				if !strings.Contains(prop.Description, info.Description) {
					t.Errorf("category %q has no guidance in the description", info.Category)
				}
			}
		})
	}
}

func TestCategoryHelpOmitsReservedByDefault(t *testing.T) {
	if strings.Contains(memory.CategoryHelp(false), string(memory.CategoryInstinct)) {
		t.Error("instinct is reserved and should not be offered as a choice")
	}
	if !strings.Contains(memory.CategoryHelp(true), string(memory.CategoryInstinct)) {
		t.Error("instinct should appear when reserved categories are requested")
	}
}
