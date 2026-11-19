package aideignore

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGitignoreReadFailureIsReportedAndNotCached(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{".git", ".gitignore"} {
		if err := os.Mkdir(filepath.Join(root, name), 0755); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := NewWithOptions(root, Options{Gitignore: true}); err == nil {
		t.Fatal("unreadable .gitignore was silently ignored")
	}
	if err := os.Remove(filepath.Join(root, ".gitignore")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".gitignore"), []byte("secret.txt\n"), 0600); err != nil {
		t.Fatal(err)
	}
	matcher, err := NewWithOptions(root, Options{Gitignore: true})
	if err != nil {
		t.Fatal(err)
	}
	if !matcher.ShouldIgnoreFile("secret.txt") {
		t.Fatal("cached partial ignore rules after read failure")
	}
}

func TestGitignoreScannerFailureDoesNotReturnPartialRules(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".git"), 0755); err != nil {
		t.Fatal(err)
	}
	data := "private/\n" + strings.Repeat("x", bufio.MaxScanTokenSize+1)
	if err := os.WriteFile(filepath.Join(root, ".gitignore"), []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	if matcher, err := NewWithOptions(root, Options{Gitignore: true}); err == nil || matcher != nil {
		t.Fatal("scanner failure returned partial ignore rules")
	}
}

func TestGitignorePreservesSignificantLeadingWhitespace(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".git"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".gitignore"), []byte(" secret.txt\n"), 0600); err != nil {
		t.Fatal(err)
	}
	matcher, err := NewWithOptions(root, Options{Gitignore: true})
	if err != nil {
		t.Fatal(err)
	}
	if !matcher.ShouldIgnoreFile(" secret.txt") || matcher.ShouldIgnoreFile("secret.txt") {
		t.Fatal("gitignore whitespace semantics changed")
	}
}

func TestGitignorePreservesNestedRepositoryExcludes(t *testing.T) {
	root := t.TempDir()
	for _, path := range []string{".git", "nested/.git/info"} {
		if err := os.MkdirAll(filepath.Join(root, path), 0755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "nested/.git/info/exclude"), []byte("private.txt\n"), 0600); err != nil {
		t.Fatal(err)
	}
	matcher, err := NewWithOptions(root, Options{Gitignore: true})
	if err != nil {
		t.Fatal(err)
	}
	if !matcher.ShouldIgnoreFile("nested/private.txt") || matcher.ShouldIgnoreFile("private.txt") {
		t.Fatal("nested repository exclusions lost their directory scope")
	}
}
