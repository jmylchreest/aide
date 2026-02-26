//go:build ignore

// Package fixture contains intentionally duplicated functions for testing.
// This file is NOT compiled — it is used as test data for the clone analyzer.
package fixture

import "fmt"

// ProcessOrders is a function that will be duplicated in clone_b.go
// with different variable names but identical structure.
func ProcessOrders(orders []string, maxCount int) ([]string, error) {
	var results []string
	seen := make(map[string]bool)

	for idx, order := range orders {
		if idx >= maxCount {
			break
		}
		if order == "" {
			continue
		}
		trimmed := fmt.Sprintf("[%d] %s", idx, order)
		if seen[trimmed] {
			continue
		}
		seen[trimmed] = true
		results = append(results, trimmed)
	}

	if len(results) == 0 {
		return nil, fmt.Errorf("no valid orders found")
	}
	return results, nil
}

// ValidateInputs is another function duplicated in clone_b.go.
func ValidateInputs(items []string, limit int) ([]string, error) {
	var output []string
	unique := make(map[string]bool)

	for i, item := range items {
		if i >= limit {
			break
		}
		if item == "" {
			continue
		}
		formatted := fmt.Sprintf("[%d] %s", i, item)
		if unique[formatted] {
			continue
		}
		unique[formatted] = true
		output = append(output, formatted)
	}

	if len(output) == 0 {
		return nil, fmt.Errorf("no valid items found")
	}
	return output, nil
}
