//go:build ignore

// Package fixture contains intentionally duplicated functions for testing.
// This file is NOT compiled — it is used as test data for the clone analyzer.
package fixture

import "fmt"

// HandleRequests mirrors ProcessOrders from clone_a.go
// with different variable names but identical control flow.
func HandleRequests(requests []string, maxItems int) ([]string, error) {
	var processed []string
	visited := make(map[string]bool)

	for pos, request := range requests {
		if pos >= maxItems {
			break
		}
		if request == "" {
			continue
		}
		label := fmt.Sprintf("[%d] %s", pos, request)
		if visited[label] {
			continue
		}
		visited[label] = true
		processed = append(processed, label)
	}

	if len(processed) == 0 {
		return nil, fmt.Errorf("no valid requests found")
	}
	return processed, nil
}

// CheckEntries mirrors ValidateInputs from clone_a.go
// with different variable names but identical control flow.
func CheckEntries(entries []string, cap int) ([]string, error) {
	var filtered []string
	known := make(map[string]bool)

	for n, entry := range entries {
		if n >= cap {
			break
		}
		if entry == "" {
			continue
		}
		tag := fmt.Sprintf("[%d] %s", n, entry)
		if known[tag] {
			continue
		}
		known[tag] = true
		filtered = append(filtered, tag)
	}

	if len(filtered) == 0 {
		return nil, fmt.Errorf("no valid entries found")
	}
	return filtered, nil
}
