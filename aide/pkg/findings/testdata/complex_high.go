//go:build ignore

// Package fixture contains intentionally complex functions for testing.
// This file is NOT compiled — it is used as test data for the complexity analyzer.
package fixture

import "fmt"

// HighComplexity has cyclomatic complexity >= 20 (critical at threshold 10).
// Each decision point adds +1: if, for, case, &&, ||
func HighComplexity(a, b, c, d int, s string) string {
	result := ""

	// +1 if
	if a > 0 {
		result = "positive"
	}
	// +1 if
	if b > 0 {
		result += "-b"
	}
	// +1 if
	if c > 0 {
		result += "-c"
	}
	// +1 if
	if d > 0 {
		result += "-d"
	}

	// +1 for
	for i := 0; i < a; i++ {
		// +1 if
		if i%2 == 0 {
			result += "even"
		}
	}

	// switch: +1 per case (4 cases + default = 5)
	switch s {
	case "alpha":
		result = "a"
	case "beta":
		result = "b"
	case "gamma":
		result = "g"
	case "delta":
		result = "d"
	default:
		result = "unknown"
	}

	// +1 && , +1 ||
	if a > 0 && b > 0 {
		result += "ab"
	}
	if c > 0 || d > 0 {
		result += "cd"
	}

	// +1 for
	for j := 0; j < b; j++ {
		// +1 if
		if j > 5 {
			break
		}
	}

	// +1 if, +1 &&
	if a > 10 && c < 5 {
		result += "special"
	}

	// +1 if
	if len(result) > 50 {
		result = result[:50]
	}

	fmt.Println(result)
	return result
}

// ModerateComplexity has cyclomatic complexity between 10-19 (warning at threshold 10).
func ModerateComplexity(x int, name string) string {
	out := ""

	// +1 if
	if x > 0 {
		out = "pos"
	}
	// +1 if
	if x < 0 {
		out = "neg"
	}

	// +1 for
	for i := 0; i < x; i++ {
		// +1 if
		if i%3 == 0 {
			out += "fizz"
		}
	}

	// switch: +1, +1, +1
	switch name {
	case "one":
		out += "1"
	case "two":
		out += "2"
	default:
		out += "?"
	}

	// +1 &&, +1 ||
	if x > 5 && len(name) > 3 {
		out += "long"
	}
	if x == 0 || name == "" {
		out += "empty"
	}

	return out
}

// SimpleFunction has low complexity (below threshold).
func SimpleFunction(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
