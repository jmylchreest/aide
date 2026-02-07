package main

import (
	"fmt"
	"os"
	"strings"
)

// fatal prints an error message and exits with code 1.
func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}

// truncate shortens a string to n characters with ellipsis.
func truncate(s string, n int) string {
	if n < 4 {
		return s
	}
	if len(s) <= n {
		return s
	}
	return s[:n-3] + "..."
}

// parseFlag extracts a flag value from args (e.g., "--key=value").
func parseFlag(args []string, prefix string) string {
	for _, arg := range args {
		if strings.HasPrefix(arg, prefix) {
			return strings.TrimPrefix(arg, prefix)
		}
	}
	return ""
}

// hasFlag checks if a flag is present in args.
func hasFlag(args []string, flag string) bool {
	for _, arg := range args {
		if arg == flag {
			return true
		}
	}
	return false
}

// titleCase capitalizes the first letter of each word.
func titleCase(s string) string {
	if s == "" {
		return s
	}
	words := strings.Fields(s)
	for i, word := range words {
		if len(word) > 0 {
			words[i] = strings.ToUpper(string(word[0])) + word[1:]
		}
	}
	return strings.Join(words, " ")
}
