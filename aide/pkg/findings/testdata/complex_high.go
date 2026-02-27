//go:build ignore

// Package fixture contains intentionally complex functions for testing.
// This file is NOT compiled — it is used as test data for the complexity analyzer.
package fixture

import "fmt"

// HighComplexity has cyclomatic complexity >= 30 (critical at threshold 15).
// Each decision point adds +1: if, for, case, &&, ||
// Base complexity = 1, counted branch points bring total to ~34.
func HighComplexity(a, b, c, d, e int, s string) string {
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
	// +1 if
	if e > 0 {
		result += "-e"
	}

	// +1 for
	for i := 0; i < a; i++ {
		// +1 if
		if i%2 == 0 {
			result += "even"
		}
		// +1 if
		if i%3 == 0 {
			result += "triple"
		}
	}

	// switch: +1 per case (6 cases + default = 7)
	switch s {
	case "alpha":
		result = "a"
	case "beta":
		result = "b"
	case "gamma":
		result = "g"
	case "delta":
		result = "d"
	case "epsilon":
		result = "e"
	case "zeta":
		result = "z"
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

	// +1 for
	for k := 0; k < d; k++ {
		// +1 if, +1 &&
		if k > 2 && k < 8 {
			result += "mid"
		}
		// +1 if
		if k == e {
			result += "match"
		}
	}

	// +1 if, +1 ||, +1 &&
	if (a > 5 || b > 5) && c != d {
		result += "complex"
	}

	fmt.Println(result)
	return result
}

// ModerateComplexity has cyclomatic complexity ~18 (warning at threshold 15,
// below 30 so not critical).
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
	// +1 if
	if x == 0 {
		out = "zero"
	}

	// +1 for
	for i := 0; i < x; i++ {
		// +1 if
		if i%3 == 0 {
			out += "fizz"
		}
		// +1 if
		if i%5 == 0 {
			out += "buzz"
		}
	}

	// switch: +1, +1, +1, +1
	switch name {
	case "one":
		out += "1"
	case "two":
		out += "2"
	case "three":
		out += "3"
	default:
		out += "?"
	}

	// +1 if, +1 &&
	if x > 5 && len(name) > 3 {
		out += "long"
	}
	// +1 if, +1 ||
	if x == 0 || name == "" {
		out += "empty"
	}
	// +1 if, +1 &&
	if x > 10 && name != "skip" {
		out += "big"
	}
	// +1 if
	if len(out) > 20 {
		out = out[:20]
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
